package handler

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/kursadbilgin/dispatch-engine/internal/domain"
	"github.com/kursadbilgin/dispatch-engine/internal/repository"
	"github.com/kursadbilgin/dispatch-engine/internal/service"
)

const (
	defaultPage     = 1
	defaultPageSize = 50
	maxPageSize     = 100
)

type NotificationService interface {
	Create(ctx context.Context, n *domain.Notification) (*domain.Notification, error)
	CreateBatch(ctx context.Context, notifications []domain.Notification) (*domain.Batch, []domain.Notification, error)
	GetByID(ctx context.Context, id string) (*domain.Notification, error)
	GetBatchSummary(ctx context.Context, batchID string) (*service.BatchSummary, error)
	Cancel(ctx context.Context, id string) error
	List(ctx context.Context, params repository.ListParams) ([]domain.Notification, int64, error)
}

type NotificationHandler struct {
	service NotificationService
}

func NewNotificationHandler(service NotificationService) (*NotificationHandler, error) {
	if service == nil {
		return nil, fmt.Errorf("notification service is required")
	}
	return &NotificationHandler{service: service}, nil
}

func RegisterNotificationRoutes(router fiber.Router, service NotificationService) error {
	h, err := NewNotificationHandler(service)
	if err != nil {
		return err
	}

	v1 := router.Group("/v1")
	v1.Post("/notifications", h.CreateNotification)
	v1.Post("/notifications/batch", h.CreateBatch)
	v1.Get("/notifications/:id", h.GetNotification)
	v1.Post("/notifications/:id/cancel", h.CancelNotification)
	v1.Get("/notifications", h.ListNotifications)
	v1.Get("/batches/:batchId", h.GetBatchSummary)

	return nil
}

type createNotificationRequest struct {
	CorrelationID  string  `json:"correlationId"`
	IdempotencyKey *string `json:"idempotencyKey"`
	Channel        string  `json:"channel"`
	Priority       string  `json:"priority"`
	Recipient      string  `json:"recipient"`
	Content        string  `json:"content"`
	MaxRetries     *int    `json:"maxRetries,omitempty"`
}

type createBatchRequest struct {
	Notifications []createNotificationRequest `json:"notifications"`
}

type notificationResponse struct {
	ID                string     `json:"id"`
	CorrelationID     string     `json:"correlationId"`
	IdempotencyKey    *string    `json:"idempotencyKey,omitempty"`
	BatchID           *string    `json:"batchId,omitempty"`
	Channel           string     `json:"channel"`
	Priority          string     `json:"priority"`
	Recipient         string     `json:"recipient"`
	Content           string     `json:"content"`
	Status            string     `json:"status"`
	ProviderMessageID *string    `json:"providerMessageId,omitempty"`
	AttemptCount      int        `json:"attemptCount"`
	MaxRetries        int        `json:"maxRetries"`
	NextRetryAt       *time.Time `json:"nextRetryAt,omitempty"`
	CreatedAt         time.Time  `json:"createdAt,omitempty"`
	UpdatedAt         time.Time  `json:"updatedAt,omitempty"`
}

type createBatchResponse struct {
	BatchID       string                 `json:"batchId"`
	Status        string                 `json:"status"`
	TotalCount    int                    `json:"totalCount"`
	Notifications []notificationResponse `json:"notifications"`
	Warning       string                 `json:"warning,omitempty"`
}

type listNotificationsResponse struct {
	Data []notificationResponse `json:"data"`
	Meta listMeta               `json:"meta"`
}

type listMeta struct {
	Page     int   `json:"page"`
	PageSize int   `json:"pageSize"`
	Total    int64 `json:"total"`
}

type batchSummaryResponse struct {
	BatchID    string                 `json:"batchId"`
	TotalCount int                    `json:"totalCount"`
	Status     string                 `json:"status"`
	Counts     []batchStatusCountItem `json:"counts"`
}

type batchStatusCountItem struct {
	Status string `json:"status"`
	Count  int    `json:"count"`
}

func (h *NotificationHandler) CreateNotification(c *fiber.Ctx) error {
	var req createNotificationRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}

	notification, err := requestToDomainNotification(req, requestCorrelationID(c))
	if err != nil {
		return toHTTPError(err)
	}

	created, err := h.service.Create(c.Context(), &notification)
	if err != nil {
		return toHTTPError(err)
	}

	return c.Status(fiber.StatusAccepted).JSON(toNotificationResponse(created))
}

func (h *NotificationHandler) CreateBatch(c *fiber.Ctx) error {
	var req createBatchRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}

	if len(req.Notifications) == 0 {
		return toHTTPError(fmt.Errorf("%w: notifications is required", domain.ErrValidation))
	}

	notifications := make([]domain.Notification, 0, len(req.Notifications))
	for _, item := range req.Notifications {
		n, err := requestToDomainNotification(item, requestCorrelationID(c))
		if err != nil {
			return toHTTPError(err)
		}
		notifications = append(notifications, n)
	}

	batch, createdNotifications, err := h.service.CreateBatch(c.Context(), notifications)
	if err != nil {
		if errors.Is(err, domain.ErrValidation) {
			return toHTTPError(err)
		}
		if batch == nil {
			return err
		}

		return c.Status(fiber.StatusAccepted).JSON(createBatchResponse{
			BatchID:       batch.ID,
			Status:        batch.Status.String(),
			TotalCount:    batch.TotalCount,
			Notifications: toNotificationResponses(createdNotifications),
			Warning:       err.Error(),
		})
	}

	return c.Status(fiber.StatusAccepted).JSON(createBatchResponse{
		BatchID:       batch.ID,
		Status:        batch.Status.String(),
		TotalCount:    batch.TotalCount,
		Notifications: toNotificationResponses(createdNotifications),
	})
}

func (h *NotificationHandler) GetNotification(c *fiber.Ctx) error {
	id := strings.TrimSpace(c.Params("id"))
	notification, err := h.service.GetByID(c.Context(), id)
	if err != nil {
		return toHTTPError(err)
	}

	return c.Status(fiber.StatusOK).JSON(toNotificationResponse(notification))
}

func (h *NotificationHandler) CancelNotification(c *fiber.Ctx) error {
	id := strings.TrimSpace(c.Params("id"))
	if err := h.service.Cancel(c.Context(), id); err != nil {
		return toHTTPError(err)
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"notificationId": id,
		"status":         domain.StatusCanceled.String(),
	})
}

func (h *NotificationHandler) ListNotifications(c *fiber.Ctx) error {
	params, err := parseListParams(c)
	if err != nil {
		return toHTTPError(err)
	}

	notifications, total, err := h.service.List(c.Context(), params)
	if err != nil {
		return toHTTPError(err)
	}

	return c.Status(fiber.StatusOK).JSON(listNotificationsResponse{
		Data: toNotificationResponses(notifications),
		Meta: listMeta{
			Page:     params.Page,
			PageSize: params.PageSize,
			Total:    total,
		},
	})
}

func (h *NotificationHandler) GetBatchSummary(c *fiber.Ctx) error {
	batchID := strings.TrimSpace(c.Params("batchId"))
	summary, err := h.service.GetBatchSummary(c.Context(), batchID)
	if err != nil {
		return toHTTPError(err)
	}

	items := make([]batchStatusCountItem, 0, len(summary.Counts))
	for _, count := range summary.Counts {
		items = append(items, batchStatusCountItem{
			Status: count.Status.String(),
			Count:  count.Count,
		})
	}

	return c.Status(fiber.StatusOK).JSON(batchSummaryResponse{
		BatchID:    summary.BatchID,
		TotalCount: summary.TotalCount,
		Status:     summary.Status.String(),
		Counts:     items,
	})
}

func parseListParams(c *fiber.Ctx) (repository.ListParams, error) {
	params := repository.ListParams{
		Page:     c.QueryInt("page", defaultPage),
		PageSize: c.QueryInt("pageSize", defaultPageSize),
	}

	if params.Page < 1 {
		return repository.ListParams{}, fmt.Errorf("%w: page must be >= 1", domain.ErrValidation)
	}
	if params.PageSize < 1 || params.PageSize > maxPageSize {
		return repository.ListParams{}, fmt.Errorf("%w: pageSize must be between 1 and %d", domain.ErrValidation, maxPageSize)
	}

	if rawStatus := strings.TrimSpace(c.Query("status")); rawStatus != "" {
		status, err := domain.ParseStatusFromString(rawStatus)
		if err != nil {
			return repository.ListParams{}, err
		}
		params.Status = &status
	}

	if rawChannel := strings.TrimSpace(c.Query("channel")); rawChannel != "" {
		channel, err := domain.ParseChannelFromString(rawChannel)
		if err != nil {
			return repository.ListParams{}, err
		}
		params.Channel = &channel
	}

	from, err := parseRFC3339Query(c.Query("from"), "from")
	if err != nil {
		return repository.ListParams{}, err
	}
	to, err := parseRFC3339Query(c.Query("to"), "to")
	if err != nil {
		return repository.ListParams{}, err
	}
	params.From = from
	params.To = to

	return params, nil
}

func parseRFC3339Query(value string, field string) (*time.Time, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil, nil
	}

	t, err := time.Parse(time.RFC3339, trimmed)
	if err != nil {
		return nil, fmt.Errorf("%w: %s must be RFC3339", domain.ErrValidation, field)
	}
	return &t, nil
}

func requestToDomainNotification(req createNotificationRequest, fallbackCorrelationID string) (domain.Notification, error) {
	channel, err := domain.ParseChannelFromString(req.Channel)
	if err != nil {
		return domain.Notification{}, err
	}

	priority, err := domain.ParsePriorityFromString(req.Priority)
	if err != nil {
		return domain.Notification{}, err
	}

	n := domain.Notification{
		CorrelationID:  strings.TrimSpace(req.CorrelationID),
		IdempotencyKey: req.IdempotencyKey,
		Channel:        channel,
		Priority:       priority,
		Recipient:      strings.TrimSpace(req.Recipient),
		Content:        strings.TrimSpace(req.Content),
	}

	if n.CorrelationID == "" {
		n.CorrelationID = strings.TrimSpace(fallbackCorrelationID)
	}
	if req.MaxRetries != nil {
		n.MaxRetries = *req.MaxRetries
	}

	return n, nil
}

func requestCorrelationID(c *fiber.Ctx) string {
	if value := strings.TrimSpace(c.Get(fiber.HeaderXRequestID)); value != "" {
		return value
	}
	if value, ok := c.Locals("requestid").(string); ok {
		return strings.TrimSpace(value)
	}
	return ""
}

func toNotificationResponses(notifications []domain.Notification) []notificationResponse {
	responses := make([]notificationResponse, 0, len(notifications))
	for _, notification := range notifications {
		n := notification
		responses = append(responses, toNotificationResponse(&n))
	}
	return responses
}

func toNotificationResponse(n *domain.Notification) notificationResponse {
	if n == nil {
		return notificationResponse{}
	}

	return notificationResponse{
		ID:                n.ID,
		CorrelationID:     n.CorrelationID,
		IdempotencyKey:    n.IdempotencyKey,
		BatchID:           n.BatchID,
		Channel:           n.Channel.String(),
		Priority:          n.Priority.String(),
		Recipient:         n.Recipient,
		Content:           n.Content,
		Status:            n.Status.String(),
		ProviderMessageID: n.ProviderMessageID,
		AttemptCount:      n.AttemptCount,
		MaxRetries:        n.MaxRetries,
		NextRetryAt:       n.NextRetryAt,
		CreatedAt:         n.CreatedAt,
		UpdatedAt:         n.UpdatedAt,
	}
}

func toHTTPError(err error) error {
	switch {
	case errors.Is(err, domain.ErrValidation):
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	case errors.Is(err, domain.ErrNotFound):
		return fiber.NewError(fiber.StatusNotFound, err.Error())
	case errors.Is(err, domain.ErrConflict):
		return fiber.NewError(fiber.StatusConflict, err.Error())
	default:
		return err
	}
}
