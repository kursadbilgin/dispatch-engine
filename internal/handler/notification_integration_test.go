package handler

import (
	"bytes"
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/kursadbilgin/dispatch-engine/internal/domain"
	"github.com/kursadbilgin/dispatch-engine/internal/repository"
	"github.com/kursadbilgin/dispatch-engine/internal/service"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

func TestNotificationIntegration_CreateNotification(t *testing.T) {
	t.Parallel()

	svc := &stubNotificationService{
		createFn: func(ctx context.Context, n *domain.Notification) (*domain.Notification, error) {
			if err := n.Validate(); err != nil {
				return nil, err
			}
			n.ID = "n-created"
			n.Status = domain.StatusQueued
			if strings.TrimSpace(n.CorrelationID) == "" {
				n.CorrelationID = "corr-from-service"
			}
			return n, nil
		},
	}

	app := newNotificationTestApp(t, svc)

	validBody := `{"channel":"sms","priority":"normal","recipient":"+905551112233","content":"hello"}`
	resp, body := performRequest(t, app, http.MethodPost, "/v1/notifications", validBody)
	if resp.StatusCode != fiber.StatusAccepted {
		t.Fatalf("status = %d, want 202, body=%s", resp.StatusCode, string(body))
	}
	var accepted map[string]any
	if err := json.Unmarshal(body, &accepted); err != nil {
		t.Fatalf("json unmarshal error = %v", err)
	}
	if accepted["id"] != "n-created" {
		t.Fatalf("id = %v, want n-created", accepted["id"])
	}
	if accepted["status"] != domain.StatusQueued.String() {
		t.Fatalf("status = %v, want %s", accepted["status"], domain.StatusQueued.String())
	}

	missingRecipientBody := `{"channel":"sms","priority":"normal","recipient":"","content":"hello"}`
	resp, _ = performRequest(t, app, http.MethodPost, "/v1/notifications", missingRecipientBody)
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for missing recipient", resp.StatusCode)
	}

	tooLongSMSBody := fmt.Sprintf(
		`{"channel":"sms","priority":"normal","recipient":"+905551112233","content":"%s"}`,
		strings.Repeat("a", domain.MaxSMSContent+1),
	)
	resp, _ = performRequest(t, app, http.MethodPost, "/v1/notifications", tooLongSMSBody)
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for SMS overflow", resp.StatusCode)
	}
}

func TestNotificationIntegration_CreateNotificationScheduledAt(t *testing.T) {
	t.Parallel()

	expectedScheduledAt, _ := time.Parse(time.RFC3339, "2026-03-01T10:00:00Z")
	svc := &stubNotificationService{
		createFn: func(ctx context.Context, n *domain.Notification) (*domain.Notification, error) {
			if n.ScheduledAt == nil {
				t.Fatal("ScheduledAt should be parsed from request")
			}
			if !n.ScheduledAt.Equal(expectedScheduledAt) {
				t.Fatalf("ScheduledAt = %v, want %v", n.ScheduledAt, expectedScheduledAt)
			}
			n.ID = "n-scheduled"
			n.Status = domain.StatusAccepted
			return n, nil
		},
	}

	app := newNotificationTestApp(t, svc)

	validBody := `{"channel":"sms","priority":"normal","recipient":"+905551112233","content":"hello","scheduledAt":"2026-03-01T10:00:00Z"}`
	resp, body := performRequest(t, app, http.MethodPost, "/v1/notifications", validBody)
	if resp.StatusCode != fiber.StatusAccepted {
		t.Fatalf("status = %d, want 202, body=%s", resp.StatusCode, string(body))
	}

	var parsed map[string]any
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("json unmarshal error = %v", err)
	}
	if parsed["scheduledAt"] != "2026-03-01T10:00:00Z" {
		t.Fatalf("scheduledAt = %v, want 2026-03-01T10:00:00Z", parsed["scheduledAt"])
	}
	if parsed["status"] != domain.StatusAccepted.String() {
		t.Fatalf("status = %v, want %s", parsed["status"], domain.StatusAccepted.String())
	}

	invalidBody := `{"channel":"sms","priority":"normal","recipient":"+905551112233","content":"hello","scheduledAt":"invalid-date"}`
	resp, _ = performRequest(t, app, http.MethodPost, "/v1/notifications", invalidBody)
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for invalid scheduledAt", resp.StatusCode)
	}
}

func TestNotificationIntegration_CreateBatch(t *testing.T) {
	t.Parallel()

	svc := &stubNotificationService{
		createBatchFn: func(ctx context.Context, notifications []domain.Notification) (*domain.Batch, []domain.Notification, error) {
			if len(notifications) > 1000 {
				return nil, nil, fmt.Errorf("%w: batch size exceeds 1000", domain.ErrValidation)
			}

			created := make([]domain.Notification, len(notifications))
			copy(created, notifications)
			for i := range created {
				if err := created[i].Validate(); err != nil {
					return nil, nil, err
				}
				created[i].ID = fmt.Sprintf("n-%d", i+1)
				created[i].Status = domain.StatusQueued
			}

			return &domain.Batch{
				ID:         "batch-1",
				TotalCount: len(created),
				Status:     domain.BatchStatusCompleted,
			}, created, nil
		},
	}

	app := newNotificationTestApp(t, svc)

	overLimitItems := make([]string, 0, 1001)
	for i := 0; i < 1001; i++ {
		overLimitItems = append(overLimitItems, `{"channel":"sms","priority":"normal","recipient":"+905551112233","content":"hello"}`)
	}
	overLimitBody := `{"notifications":[` + strings.Join(overLimitItems, ",") + `]}`
	resp, _ := performRequest(t, app, http.MethodPost, "/v1/notifications/batch", overLimitBody)
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for batch size > 1000", resp.StatusCode)
	}

	validBody := `{"notifications":[{"channel":"sms","priority":"high","recipient":"+905551112233","content":"hello sms"},{"channel":"email","priority":"normal","recipient":"user@example.com","content":"hello email"}]}`
	resp, body := performRequest(t, app, http.MethodPost, "/v1/notifications/batch", validBody)
	if resp.StatusCode != fiber.StatusAccepted {
		t.Fatalf("status = %d, want 202, body=%s", resp.StatusCode, string(body))
	}

	var parsed map[string]any
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("json unmarshal error = %v", err)
	}
	if parsed["batchId"] != "batch-1" {
		t.Fatalf("batchId = %v, want batch-1", parsed["batchId"])
	}
	if parsed["totalCount"] != float64(2) {
		t.Fatalf("totalCount = %v, want 2", parsed["totalCount"])
	}
}

func TestNotificationIntegration_GetNotification(t *testing.T) {
	t.Parallel()

	svc := &stubNotificationService{
		getByIDFn: func(ctx context.Context, id string) (*domain.Notification, error) {
			if id == "n-found" {
				return &domain.Notification{
					ID:            "n-found",
					CorrelationID: "corr-1",
					Channel:       domain.ChannelSMS,
					Priority:      domain.PriorityNormal,
					Recipient:     "+905551112233",
					Content:       "hello",
					Status:        domain.StatusQueued,
					MaxRetries:    5,
				}, nil
			}
			return nil, domain.ErrNotFound
		},
	}

	app := newNotificationTestApp(t, svc)

	resp, _ := performRequest(t, app, http.MethodGet, "/v1/notifications/n-found", "")
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	resp, _ = performRequest(t, app, http.MethodGet, "/v1/notifications/not-exists", "")
	if resp.StatusCode != fiber.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}

func TestNotificationIntegration_CancelNotification(t *testing.T) {
	t.Parallel()

	svc := &stubNotificationService{
		cancelFn: func(ctx context.Context, id string) error {
			if id == "n-cancelable" {
				return nil
			}
			return domain.ErrConflict
		},
	}

	app := newNotificationTestApp(t, svc)

	resp, _ := performRequest(t, app, http.MethodPost, "/v1/notifications/n-cancelable/cancel", "")
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	resp, _ = performRequest(t, app, http.MethodPost, "/v1/notifications/n-locked/cancel", "")
	if resp.StatusCode != fiber.StatusConflict {
		t.Fatalf("status = %d, want 409", resp.StatusCode)
	}
}

func TestNotificationIntegration_ListNotificationsPaginationAndFilters(t *testing.T) {
	t.Parallel()

	fromExpected, _ := time.Parse(time.RFC3339, "2026-01-01T00:00:00Z")
	toExpected, _ := time.Parse(time.RFC3339, "2026-01-31T23:59:59Z")

	svc := &stubNotificationService{
		listFn: func(ctx context.Context, params repository.ListParams) ([]domain.Notification, int64, error) {
			if params.Page != 2 {
				t.Fatalf("page = %d, want 2", params.Page)
			}
			if params.PageSize != 10 {
				t.Fatalf("pageSize = %d, want 10", params.PageSize)
			}
			if params.Status == nil || *params.Status != domain.StatusQueued {
				t.Fatalf("status filter = %v, want QUEUED", params.Status)
			}
			if params.Channel == nil || *params.Channel != domain.ChannelSMS {
				t.Fatalf("channel filter = %v, want SMS", params.Channel)
			}
			if params.From == nil || !params.From.Equal(fromExpected) {
				t.Fatalf("from = %v, want %v", params.From, fromExpected)
			}
			if params.To == nil || !params.To.Equal(toExpected) {
				t.Fatalf("to = %v, want %v", params.To, toExpected)
			}

			return []domain.Notification{
				{
					ID:            "n-list-1",
					CorrelationID: "corr-list",
					Channel:       domain.ChannelSMS,
					Priority:      domain.PriorityNormal,
					Recipient:     "+905551112233",
					Content:       "hello",
					Status:        domain.StatusQueued,
					MaxRetries:    5,
				},
			}, 1, nil
		},
	}

	app := newNotificationTestApp(t, svc)

	path := "/v1/notifications?page=2&pageSize=10&status=queued&channel=sms&from=2026-01-01T00:00:00Z&to=2026-01-31T23:59:59Z"
	resp, body := performRequest(t, app, http.MethodGet, path, "")
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", resp.StatusCode, string(body))
	}

	var parsed struct {
		Data []map[string]any `json:"data"`
		Meta struct {
			Page     int   `json:"page"`
			PageSize int   `json:"pageSize"`
			Total    int64 `json:"total"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("json unmarshal error = %v", err)
	}

	if parsed.Meta.Page != 2 || parsed.Meta.PageSize != 10 || parsed.Meta.Total != 1 {
		t.Fatalf("meta = %+v, want page=2,pageSize=10,total=1", parsed.Meta)
	}
	if len(parsed.Data) != 1 {
		t.Fatalf("data len = %d, want 1", len(parsed.Data))
	}

	resp, _ = performRequest(
		t,
		app,
		http.MethodGet,
		"/v1/notifications?from=2026-02-01T00:00:00Z&to=2026-01-01T00:00:00Z",
		"",
	)
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for invalid date range", resp.StatusCode)
	}
}

func TestNotificationIntegration_GetBatchSummary(t *testing.T) {
	t.Parallel()

	svc := &stubNotificationService{
		getBatchSummaryFn: func(ctx context.Context, batchID string) (*service.BatchSummary, error) {
			if batchID != "batch-42" {
				return nil, domain.ErrNotFound
			}
			return &service.BatchSummary{
				BatchID:    "batch-42",
				TotalCount: 3,
				Status:     domain.BatchStatusPartialFailure,
				Counts: []service.StatusCount{
					{Status: domain.StatusSent, Count: 2},
					{Status: domain.StatusFailed, Count: 1},
				},
			}, nil
		},
	}

	app := newNotificationTestApp(t, svc)

	resp, body := performRequest(t, app, http.MethodGet, "/v1/batches/batch-42", "")
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", resp.StatusCode, string(body))
	}

	var parsed map[string]any
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("json unmarshal error = %v", err)
	}
	if parsed["batchId"] != "batch-42" {
		t.Fatalf("batchId = %v, want batch-42", parsed["batchId"])
	}
	if parsed["status"] != domain.BatchStatusPartialFailure.String() {
		t.Fatalf("status = %v, want %s", parsed["status"], domain.BatchStatusPartialFailure.String())
	}
}

func TestHealthIntegration_LivezAndReadyz(t *testing.T) {
	t.Parallel()

	t.Run("livez returns 200", func(t *testing.T) {
		t.Parallel()

		app := fiber.New(fiber.Config{ErrorHandler: ErrorHandler(zap.NewNop())})
		RegisterHealthRoutes(app, sql.OpenDB(stubConnector{}), newStubRedisClient(nil))

		resp, body := performRequest(t, app, http.MethodGet, "/livez", "")
		if resp.StatusCode != fiber.StatusOK {
			t.Fatalf("status = %d, want 200, body=%s", resp.StatusCode, string(body))
		}
	})

	t.Run("readyz returns 200 when dependencies healthy", func(t *testing.T) {
		t.Parallel()

		sqlDB := sql.OpenDB(stubConnector{})
		t.Cleanup(func() { _ = sqlDB.Close() })

		rdb := newStubRedisClient(nil)
		t.Cleanup(func() { _ = rdb.Close() })

		app := fiber.New(fiber.Config{ErrorHandler: ErrorHandler(zap.NewNop())})
		RegisterHealthRoutes(app, sqlDB, rdb)

		resp, body := performRequest(t, app, http.MethodGet, "/readyz", "")
		if resp.StatusCode != fiber.StatusOK {
			t.Fatalf("status = %d, want 200, body=%s", resp.StatusCode, string(body))
		}
	})

	t.Run("readyz returns 503 when dependencies down", func(t *testing.T) {
		t.Parallel()

		sqlDB := sql.OpenDB(stubConnector{pingErr: errors.New("postgres down")})
		t.Cleanup(func() { _ = sqlDB.Close() })

		rdb := newStubRedisClient(errors.New("redis down"))
		t.Cleanup(func() { _ = rdb.Close() })

		app := fiber.New(fiber.Config{ErrorHandler: ErrorHandler(zap.NewNop())})
		RegisterHealthRoutes(app, sqlDB, rdb)

		resp, body := performRequest(t, app, http.MethodGet, "/readyz", "")
		if resp.StatusCode != fiber.StatusServiceUnavailable {
			t.Fatalf("status = %d, want 503, body=%s", resp.StatusCode, string(body))
		}
	})
}

type stubNotificationService struct {
	createFn          func(ctx context.Context, n *domain.Notification) (*domain.Notification, error)
	createBatchFn     func(ctx context.Context, notifications []domain.Notification) (*domain.Batch, []domain.Notification, error)
	getByIDFn         func(ctx context.Context, id string) (*domain.Notification, error)
	getBatchSummaryFn func(ctx context.Context, batchID string) (*service.BatchSummary, error)
	cancelFn          func(ctx context.Context, id string) error
	listFn            func(ctx context.Context, params repository.ListParams) ([]domain.Notification, int64, error)
}

func (s *stubNotificationService) Create(ctx context.Context, n *domain.Notification) (*domain.Notification, error) {
	if s.createFn != nil {
		return s.createFn(ctx, n)
	}
	return nil, errors.New("not implemented")
}

func (s *stubNotificationService) CreateBatch(
	ctx context.Context,
	notifications []domain.Notification,
) (*domain.Batch, []domain.Notification, error) {
	if s.createBatchFn != nil {
		return s.createBatchFn(ctx, notifications)
	}
	return nil, nil, errors.New("not implemented")
}

func (s *stubNotificationService) GetByID(ctx context.Context, id string) (*domain.Notification, error) {
	if s.getByIDFn != nil {
		return s.getByIDFn(ctx, id)
	}
	return nil, domain.ErrNotFound
}

func (s *stubNotificationService) GetBatchSummary(ctx context.Context, batchID string) (*service.BatchSummary, error) {
	if s.getBatchSummaryFn != nil {
		return s.getBatchSummaryFn(ctx, batchID)
	}
	return nil, domain.ErrNotFound
}

func (s *stubNotificationService) Cancel(ctx context.Context, id string) error {
	if s.cancelFn != nil {
		return s.cancelFn(ctx, id)
	}
	return nil
}

func (s *stubNotificationService) List(
	ctx context.Context,
	params repository.ListParams,
) ([]domain.Notification, int64, error) {
	if s.listFn != nil {
		return s.listFn(ctx, params)
	}
	return nil, 0, nil
}

func newNotificationTestApp(t *testing.T, svc NotificationService) *fiber.App {
	t.Helper()

	app := fiber.New(fiber.Config{
		ErrorHandler: ErrorHandler(zap.NewNop()),
	})

	if err := RegisterNotificationRoutes(app, svc); err != nil {
		t.Fatalf("RegisterNotificationRoutes() error = %v", err)
	}

	return app
}

func performRequest(t *testing.T, app *fiber.App, method string, path string, body string) (*http.Response, []byte) {
	t.Helper()

	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	req.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test() error = %v", err)
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read response body: %v", err)
	}
	_ = resp.Body.Close()

	return resp, respBody
}

type stubConnector struct {
	pingErr error
}

func (c stubConnector) Connect(context.Context) (driver.Conn, error) {
	return stubConn(c), nil
}

func (c stubConnector) Driver() driver.Driver {
	return stubDriver(c)
}

type stubDriver struct {
	pingErr error
}

func (d stubDriver) Open(string) (driver.Conn, error) {
	return stubConn(d), nil
}

type stubConn struct {
	pingErr error
}

func (c stubConn) Prepare(string) (driver.Stmt, error) { return nil, errors.New("not implemented") }
func (c stubConn) Close() error                        { return nil }
func (c stubConn) Begin() (driver.Tx, error)           { return nil, errors.New("not implemented") }
func (c stubConn) Ping(context.Context) error          { return c.pingErr }

type stubRedisHook struct {
	pingErr error
}

func (h stubRedisHook) DialHook(next redis.DialHook) redis.DialHook {
	return next
}

func (h stubRedisHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		if strings.EqualFold(cmd.Name(), "ping") {
			if h.pingErr != nil {
				cmd.SetErr(h.pingErr)
				return h.pingErr
			}
			cmd.SetErr(nil)
			return nil
		}
		cmd.SetErr(nil)
		return nil
	}
}

func (h stubRedisHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return func(ctx context.Context, cmds []redis.Cmder) error {
		for _, cmd := range cmds {
			cmd.SetErr(nil)
		}
		return nil
	}
}

func newStubRedisClient(pingErr error) *redis.Client {
	rdb := redis.NewClient(&redis.Options{
		Addr:         "127.0.0.1:6379",
		DialTimeout:  time.Millisecond,
		ReadTimeout:  time.Millisecond,
		WriteTimeout: time.Millisecond,
	})
	rdb.AddHook(stubRedisHook{pingErr: pingErr})
	return rdb
}
