package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/kursadbilgin/dispatch-engine/internal/domain"
	"github.com/kursadbilgin/dispatch-engine/internal/queue"
	"github.com/kursadbilgin/dispatch-engine/internal/repository"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

const (
	defaultMaxRetries = 5
	maxBatchSize      = 1000
)

type NotificationService struct {
	notifications repository.NotificationRepository
	batches       repository.BatchRepository
	publisher     queue.Publisher
	logger        *zap.Logger
}

type BatchSummary struct {
	BatchID    string
	TotalCount int
	Status     domain.BatchStatus
	Counts     []StatusCount
}

type StatusCount struct {
	Status domain.Status
	Count  int
}

func NewNotificationService(
	notifications repository.NotificationRepository,
	batches repository.BatchRepository,
	publisher queue.Publisher,
	logger *zap.Logger,
) (*NotificationService, error) {
	if logger == nil {
		logger = zap.NewNop()
	}

	return &NotificationService{
		notifications: notifications,
		batches:       batches,
		publisher:     publisher,
		logger:        logger,
	}, nil
}

func (s *NotificationService) Create(ctx context.Context, notification *domain.Notification) (*domain.Notification, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	if err := prepareNotificationForCreate(notification, nil); err != nil {
		return nil, err
	}

	if err := s.notifications.Create(ctx, notification); err != nil {
		existing, resolved, resolveErr := s.resolveIdempotencyConflict(ctx, err, notification.IdempotencyKey)
		if resolveErr != nil {
			return nil, resolveErr
		}
		if resolved {
			return existing, nil
		}
		return nil, err
	}

	now := time.Now().UTC()
	if !shouldEnqueueImmediately(notification.ScheduledAt, now) {
		return notification, nil
	}

	msg := queue.NotificationMessage{
		NotificationID: notification.ID,
		CorrelationID:  notification.CorrelationID,
		Channel:        notification.Channel,
		Priority:       notification.Priority,
	}
	if err := s.publisher.Publish(ctx, queue.QueueName(notification.Channel), msg); err != nil {
		s.logger.Error("failed to publish notification",
			zap.String("notificationId", notification.ID),
			zap.String("channel", string(notification.Channel)),
			zap.Error(err),
		)
		if updateErr := s.notifications.UpdateStatus(ctx, notification.ID, domain.StatusFailed); updateErr != nil {
			s.logger.Error("failed to mark notification as failed after publish error",
				zap.String("notificationId", notification.ID),
				zap.Error(updateErr),
			)
			return nil, fmt.Errorf("failed to publish notification: %w (failed to mark as failed: %v)", err, updateErr)
		}
		notification.Status = domain.StatusFailed
		return nil, fmt.Errorf("failed to publish notification: %w", err)
	}

	if err := s.notifications.UpdateStatus(ctx, notification.ID, domain.StatusQueued); err != nil {
		return nil, fmt.Errorf("failed to update notification status to queued: %w", err)
	}
	notification.Status = domain.StatusQueued

	return notification, nil
}

func (s *NotificationService) CreateBatch(
	ctx context.Context,
	notifications []domain.Notification,
) (*domain.Batch, []domain.Notification, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	if len(notifications) == 0 {
		return nil, nil, fmt.Errorf("%w: batch must include at least one notification", domain.ErrValidation)
	}
	if len(notifications) > maxBatchSize {
		return nil, nil, fmt.Errorf("%w: batch size exceeds %d", domain.ErrValidation, maxBatchSize)
	}

	batchID := uuid.NewString()

	created := make([]domain.Notification, len(notifications))
	createdPtrs := make([]*domain.Notification, len(notifications))
	for i := range notifications {
		created[i] = notifications[i]
		if err := prepareNotificationForCreate(&created[i], &batchID); err != nil {
			return nil, nil, err
		}
		createdPtrs[i] = &created[i]
	}

	batch := &domain.Batch{
		ID:         batchID,
		TotalCount: len(notifications),
		Status:     domain.BatchStatusProcessing,
	}
	if err := s.batches.Create(ctx, batch); err != nil {
		return nil, nil, err
	}

	if err := s.notifications.CreateBatch(ctx, createdPtrs); err != nil {
		_ = s.batches.UpdateStatus(ctx, batch.ID, domain.BatchStatusPartialFailure)
		return nil, nil, err
	}

	failed := 0
	now := time.Now().UTC()
	for i := range createdPtrs {
		current := createdPtrs[i]
		if !shouldEnqueueImmediately(current.ScheduledAt, now) {
			continue
		}

		msg := queue.NotificationMessage{
			NotificationID: current.ID,
			CorrelationID:  current.CorrelationID,
			Channel:        current.Channel,
			Priority:       current.Priority,
		}

		if err := s.publisher.Publish(ctx, queue.QueueName(current.Channel), msg); err != nil {
			s.logger.Error("batch: failed to publish notification",
				zap.String("notificationId", current.ID),
				zap.String("channel", string(current.Channel)),
				zap.Error(err),
			)
			failed++
			_ = s.notifications.UpdateStatus(ctx, current.ID, domain.StatusFailed)
			current.Status = domain.StatusFailed
			continue
		}
		if err := s.notifications.UpdateStatus(ctx, current.ID, domain.StatusQueued); err != nil {
			failed++
			continue
		}
		current.Status = domain.StatusQueued
	}

	batch.Status = domain.BatchStatusCompleted
	if failed > 0 {
		batch.Status = domain.BatchStatusPartialFailure
	}
	if err := s.batches.UpdateStatus(ctx, batch.ID, batch.Status); err != nil {
		return nil, nil, err
	}

	if failed > 0 {
		s.logger.Warn("batch completed with partial failure",
			zap.String("batchId", batch.ID),
			zap.Int("failed", failed),
			zap.Int("total", len(created)),
		)
		return batch, created, fmt.Errorf("batch queued with partial failure: %d/%d failed", failed, len(created))
	}

	return batch, created, nil
}

func (s *NotificationService) GetByID(ctx context.Context, id string) (*domain.Notification, error) {
	if strings.TrimSpace(id) == "" {
		return nil, fmt.Errorf("%w: notification id is required", domain.ErrValidation)
	}
	return s.notifications.GetByID(ctx, strings.TrimSpace(id))
}

func (s *NotificationService) GetBatchSummary(ctx context.Context, batchID string) (*BatchSummary, error) {
	if strings.TrimSpace(batchID) == "" {
		return nil, fmt.Errorf("%w: batch id is required", domain.ErrValidation)
	}

	batch, err := s.batches.GetByID(ctx, strings.TrimSpace(batchID))
	if err != nil {
		return nil, err
	}

	statuses, err := s.notifications.GetBatchSummary(ctx, batchID)
	if err != nil {
		return nil, err
	}

	counts := make([]StatusCount, 0, len(statuses))
	for _, summary := range statuses {
		counts = append(counts, StatusCount{
			Status: summary.Status,
			Count:  summary.Count,
		})
	}

	return &BatchSummary{
		BatchID:    batch.ID,
		TotalCount: batch.TotalCount,
		Status:     batch.Status,
		Counts:     counts,
	}, nil
}

func (s *NotificationService) Cancel(ctx context.Context, id string) error {
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("%w: notification id is required", domain.ErrValidation)
	}
	return s.notifications.Cancel(ctx, strings.TrimSpace(id))
}

func (s *NotificationService) List(
	ctx context.Context,
	params repository.ListParams,
) ([]domain.Notification, int64, error) {
	return s.notifications.List(ctx, params)
}

func prepareNotificationForCreate(n *domain.Notification, batchID *string) error {
	if n == nil {
		return fmt.Errorf("%w: notification is required", domain.ErrValidation)
	}

	n.Recipient = strings.TrimSpace(n.Recipient)
	n.Content = strings.TrimSpace(n.Content)
	n.CorrelationID = strings.TrimSpace(n.CorrelationID)
	if n.CorrelationID == "" {
		n.CorrelationID = uuid.NewString()
	}

	n.ID = strings.TrimSpace(n.ID)
	if n.ID == "" {
		n.ID = uuid.NewString()
	}

	n.IdempotencyKey = normalizeOptionalString(n.IdempotencyKey)
	if batchID != nil {
		n.BatchID = batchID
	}

	n.Status = domain.StatusAccepted
	n.AttemptCount = 0
	if n.MaxRetries <= 0 {
		n.MaxRetries = defaultMaxRetries
	}
	n.ProviderMessageID = nil
	n.NextRetryAt = nil

	if err := n.Validate(); err != nil {
		return err
	}

	return nil
}

func normalizeOptionalString(v *string) *string {
	if v == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*v)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func shouldEnqueueImmediately(scheduledAt *time.Time, now time.Time) bool {
	if scheduledAt == nil {
		return true
	}
	return !scheduledAt.After(now)
}

func (s *NotificationService) resolveIdempotencyConflict(
	ctx context.Context,
	createErr error,
	idempotencyKey *string,
) (*domain.Notification, bool, error) {
	if idempotencyKey == nil || strings.TrimSpace(*idempotencyKey) == "" {
		return nil, false, nil
	}
	if !isUniqueViolationError(createErr) {
		return nil, false, nil
	}

	existing, err := s.notifications.GetByIdempotencyKey(ctx, strings.TrimSpace(*idempotencyKey))
	if err != nil {
		return nil, false, fmt.Errorf("failed to load existing notification after idempotency conflict: %w", err)
	}
	s.logger.Info("idempotency conflict resolved",
		zap.String("existingId", existing.ID),
		zap.String("idempotencyKey", *idempotencyKey),
	)
	return existing, true, nil
}

func isUniqueViolationError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}

	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "duplicate key") || strings.Contains(msg, "unique constraint")
}
