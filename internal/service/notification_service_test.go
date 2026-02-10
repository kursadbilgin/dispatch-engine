package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kursadbilgin/dispatch-engine/internal/domain"
	"github.com/kursadbilgin/dispatch-engine/internal/queue"
	"github.com/kursadbilgin/dispatch-engine/internal/repository"
)

func TestNotificationServiceCreateHappyPath(t *testing.T) {
	t.Parallel()

	updatedToQueued := false
	repo := &fakeNotificationRepo{
		createFn: func(ctx context.Context, n *domain.Notification) error {
			if n.Status != domain.StatusAccepted {
				t.Fatalf("status = %s, want ACCEPTED", n.Status)
			}
			if strings.TrimSpace(n.CorrelationID) == "" {
				t.Fatal("correlation id should be generated")
			}
			n.CreatedAt = time.Now().UTC()
			n.UpdatedAt = n.CreatedAt
			return nil
		},
		updateStatusFn: func(ctx context.Context, id string, status domain.Status) error {
			if status != domain.StatusQueued {
				t.Fatalf("status update = %s, want QUEUED", status)
			}
			updatedToQueued = true
			return nil
		},
	}

	publishCalled := false
	publisher := &fakePublisher{
		publishFn: func(ctx context.Context, queueName string, msg queue.NotificationMessage) error {
			if queueName != "sms" {
				t.Fatalf("queue name = %s, want sms", queueName)
			}
			if msg.NotificationID == "" {
				t.Fatal("notification id should be set on publish")
			}
			publishCalled = true
			return nil
		},
	}

	svc, err := NewNotificationService(repo, &fakeBatchRepo{}, publisher, nil)
	if err != nil {
		t.Fatalf("NewNotificationService() error = %v", err)
	}

	result, err := svc.Create(context.Background(), &domain.Notification{
		Channel:   domain.ChannelSMS,
		Priority:  domain.PriorityNormal,
		Recipient: "+905551112233",
		Content:   "hello world",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if result.Status != domain.StatusQueued {
		t.Fatalf("result status = %s, want QUEUED", result.Status)
	}
	if !publishCalled {
		t.Fatal("expected publish to be called")
	}
	if !updatedToQueued {
		t.Fatal("expected UpdateStatus to be called")
	}
}

func TestNotificationServiceCreatePublishFailureMarksFailed(t *testing.T) {
	t.Parallel()

	markedFailed := false
	repo := &fakeNotificationRepo{
		createFn: func(ctx context.Context, n *domain.Notification) error {
			n.CreatedAt = time.Now().UTC()
			n.UpdatedAt = n.CreatedAt
			return nil
		},
		updateStatusFn: func(ctx context.Context, id string, status domain.Status) error {
			if status != domain.StatusFailed {
				t.Fatalf("status update = %s, want FAILED", status)
			}
			markedFailed = true
			return nil
		},
	}

	publisher := &fakePublisher{
		publishFn: func(ctx context.Context, queueName string, msg queue.NotificationMessage) error {
			return errors.New("broker unavailable")
		},
	}

	svc, err := NewNotificationService(repo, &fakeBatchRepo{}, publisher, nil)
	if err != nil {
		t.Fatalf("NewNotificationService() error = %v", err)
	}

	_, err = svc.Create(context.Background(), &domain.Notification{
		Channel:   domain.ChannelSMS,
		Priority:  domain.PriorityNormal,
		Recipient: "+905551112233",
		Content:   "hello world",
	})
	if err == nil {
		t.Fatal("Create() expected error, got nil")
	}
	if !markedFailed {
		t.Fatal("Create() should mark notification as FAILED when publish fails")
	}
}

func TestNotificationServiceCreateScheduledSkipsPublish(t *testing.T) {
	t.Parallel()

	updatedStatus := false
	repo := &fakeNotificationRepo{
		createFn: func(ctx context.Context, n *domain.Notification) error {
			n.CreatedAt = time.Now().UTC()
			n.UpdatedAt = n.CreatedAt
			return nil
		},
		updateStatusFn: func(ctx context.Context, id string, status domain.Status) error {
			updatedStatus = true
			return nil
		},
	}

	published := false
	publisher := &fakePublisher{
		publishFn: func(ctx context.Context, queueName string, msg queue.NotificationMessage) error {
			published = true
			return nil
		},
	}

	svc, err := NewNotificationService(repo, &fakeBatchRepo{}, publisher, nil)
	if err != nil {
		t.Fatalf("NewNotificationService() error = %v", err)
	}

	scheduledAt := time.Now().UTC().Add(2 * time.Minute)
	result, err := svc.Create(context.Background(), &domain.Notification{
		Channel:     domain.ChannelSMS,
		Priority:    domain.PriorityNormal,
		Recipient:   "+905551112233",
		Content:     "hello world",
		ScheduledAt: &scheduledAt,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if result.Status != domain.StatusAccepted {
		t.Fatalf("result status = %s, want ACCEPTED", result.Status)
	}
	if published {
		t.Fatal("publish should not be called for future scheduled notification")
	}
	if updatedStatus {
		t.Fatal("status should not be updated to QUEUED for future scheduled notification")
	}
}

func TestNotificationServiceCreateIdempotencyConflictReturnsExisting(t *testing.T) {
	t.Parallel()

	key := "same-idempotency-key"
	existing := &domain.Notification{
		ID:            "existing-id",
		CorrelationID: "existing-correlation",
		Channel:       domain.ChannelSMS,
		Priority:      domain.PriorityNormal,
		Recipient:     "+905551112233",
		Content:       "already queued",
		Status:        domain.StatusQueued,
	}

	repo := &fakeNotificationRepo{
		createFn: func(ctx context.Context, n *domain.Notification) error {
			return errors.New("duplicate key value violates unique constraint idx_notifications_idempotency_key")
		},
		getByIdempotencyKeyFn: func(ctx context.Context, idempotencyKey string) (*domain.Notification, error) {
			if idempotencyKey != key {
				t.Fatalf("idempotency key = %q, want %q", idempotencyKey, key)
			}
			return existing, nil
		},
	}

	publisher := &fakePublisher{
		publishFn: func(ctx context.Context, queueName string, msg queue.NotificationMessage) error {
			t.Fatal("publish should not be called on idempotency conflict")
			return nil
		},
	}

	svc, err := NewNotificationService(repo, &fakeBatchRepo{}, publisher, nil)
	if err != nil {
		t.Fatalf("NewNotificationService() error = %v", err)
	}

	result, err := svc.Create(context.Background(), &domain.Notification{
		IdempotencyKey: &key,
		Channel:        domain.ChannelSMS,
		Priority:       domain.PriorityNormal,
		Recipient:      "+905551112233",
		Content:        "hello",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if result.ID != existing.ID {
		t.Fatalf("result id = %s, want %s", result.ID, existing.ID)
	}
}

func TestNotificationServiceCreateBatchValidationBeforeBatchCreate(t *testing.T) {
	t.Parallel()

	batchCreateCalled := false
	svc, err := NewNotificationService(
		&fakeNotificationRepo{},
		&fakeBatchRepo{
			createFn: func(ctx context.Context, b *domain.Batch) error {
				batchCreateCalled = true
				return nil
			},
		},
		&fakePublisher{},
		nil,
	)
	if err != nil {
		t.Fatalf("NewNotificationService() error = %v", err)
	}

	_, _, err = svc.CreateBatch(context.Background(), []domain.Notification{
		{
			Channel:   domain.ChannelSMS,
			Priority:  domain.PriorityNormal,
			Recipient: "",
			Content:   "invalid",
		},
	})
	if !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("CreateBatch() error = %v, want ErrValidation", err)
	}
	if batchCreateCalled {
		t.Fatal("batch should not be created when payload validation fails")
	}
}

func TestNotificationServiceCreateBatchMarksBatchPartialFailureOnPersistError(t *testing.T) {
	t.Parallel()

	var updatedStatus domain.BatchStatus
	svc, err := NewNotificationService(
		&fakeNotificationRepo{
			createBatchFn: func(ctx context.Context, notifications []*domain.Notification) error {
				return errors.New("insert failed")
			},
		},
		&fakeBatchRepo{
			updateStatusFn: func(ctx context.Context, id string, status domain.BatchStatus) error {
				updatedStatus = status
				return nil
			},
		},
		&fakePublisher{},
		nil,
	)
	if err != nil {
		t.Fatalf("NewNotificationService() error = %v", err)
	}

	_, _, err = svc.CreateBatch(context.Background(), []domain.Notification{
		{
			Channel:   domain.ChannelSMS,
			Priority:  domain.PriorityNormal,
			Recipient: "+905551112233",
			Content:   "hello",
		},
	})
	if err == nil {
		t.Fatal("CreateBatch() expected error, got nil")
	}
	if updatedStatus != domain.BatchStatusPartialFailure {
		t.Fatalf("batch status update = %s, want PARTIAL_FAILURE", updatedStatus)
	}
}

func TestNotificationServiceCreateBatchExceedsLimit(t *testing.T) {
	t.Parallel()

	svc, err := NewNotificationService(&fakeNotificationRepo{}, &fakeBatchRepo{}, &fakePublisher{}, nil)
	if err != nil {
		t.Fatalf("NewNotificationService() error = %v", err)
	}

	notifications := make([]domain.Notification, maxBatchSize+1)
	for i := range notifications {
		notifications[i] = domain.Notification{
			Channel:   domain.ChannelSMS,
			Priority:  domain.PriorityNormal,
			Recipient: "+905551112233",
			Content:   "hello",
		}
	}

	_, _, err = svc.CreateBatch(context.Background(), notifications)
	if !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("CreateBatch() error = %v, want ErrValidation", err)
	}
}

func TestNotificationServiceCreateBatchPartialFailure(t *testing.T) {
	t.Parallel()

	queuedCount := 0
	failedCount := 0
	repo := &fakeNotificationRepo{
		createBatchFn: func(ctx context.Context, notifications []*domain.Notification) error {
			if len(notifications) != 2 {
				t.Fatalf("create batch len = %d, want 2", len(notifications))
			}
			return nil
		},
		updateStatusFn: func(ctx context.Context, id string, status domain.Status) error {
			switch status {
			case domain.StatusQueued:
				queuedCount++
			case domain.StatusFailed:
				failedCount++
			default:
				t.Fatalf("status = %s, want QUEUED or FAILED", status)
			}
			return nil
		},
	}

	var updatedBatchStatus domain.BatchStatus
	batchRepo := &fakeBatchRepo{
		updateStatusFn: func(ctx context.Context, id string, status domain.BatchStatus) error {
			updatedBatchStatus = status
			return nil
		},
	}

	publishCalls := 0
	publisher := &fakePublisher{
		publishFn: func(ctx context.Context, queueName string, msg queue.NotificationMessage) error {
			publishCalls++
			if publishCalls == 2 {
				return errors.New("broker temporary down")
			}
			return nil
		},
	}

	svc, err := NewNotificationService(repo, batchRepo, publisher, nil)
	if err != nil {
		t.Fatalf("NewNotificationService() error = %v", err)
	}

	batch, created, err := svc.CreateBatch(context.Background(), []domain.Notification{
		{
			Channel:   domain.ChannelSMS,
			Priority:  domain.PriorityNormal,
			Recipient: "+905551112233",
			Content:   "hello",
		},
		{
			Channel:   domain.ChannelEmail,
			Priority:  domain.PriorityHigh,
			Recipient: "user@example.com",
			Content:   "hello email",
		},
	})

	if err == nil {
		t.Fatal("CreateBatch() expected partial failure error, got nil")
	}
	if batch == nil {
		t.Fatal("batch should not be nil")
	}
	if len(created) != 2 {
		t.Fatalf("created len = %d, want 2", len(created))
	}
	if batch.Status != domain.BatchStatusPartialFailure {
		t.Fatalf("batch status = %s, want PARTIAL_FAILURE", batch.Status)
	}
	if updatedBatchStatus != domain.BatchStatusPartialFailure {
		t.Fatalf("updated batch status = %s, want PARTIAL_FAILURE", updatedBatchStatus)
	}
	if publishCalls != 2 {
		t.Fatalf("publish calls = %d, want 2", publishCalls)
	}
	if queuedCount != 1 {
		t.Fatalf("queued count = %d, want 1", queuedCount)
	}
	if failedCount != 1 {
		t.Fatalf("failed count = %d, want 1", failedCount)
	}
}

func TestNotificationServiceCreateBatchSkipsFutureScheduledEnqueue(t *testing.T) {
	t.Parallel()

	queuedCount := 0
	repo := &fakeNotificationRepo{
		createBatchFn: func(ctx context.Context, notifications []*domain.Notification) error {
			return nil
		},
		updateStatusFn: func(ctx context.Context, id string, status domain.Status) error {
			if status != domain.StatusQueued {
				t.Fatalf("status = %s, want QUEUED", status)
			}
			queuedCount++
			return nil
		},
	}

	var updatedBatchStatus domain.BatchStatus
	batchRepo := &fakeBatchRepo{
		updateStatusFn: func(ctx context.Context, id string, status domain.BatchStatus) error {
			updatedBatchStatus = status
			return nil
		},
	}

	publishCalls := 0
	publisher := &fakePublisher{
		publishFn: func(ctx context.Context, queueName string, msg queue.NotificationMessage) error {
			publishCalls++
			return nil
		},
	}

	svc, err := NewNotificationService(repo, batchRepo, publisher, nil)
	if err != nil {
		t.Fatalf("NewNotificationService() error = %v", err)
	}

	scheduledAt := time.Now().UTC().Add(3 * time.Minute)
	batch, created, err := svc.CreateBatch(context.Background(), []domain.Notification{
		{
			Channel:     domain.ChannelSMS,
			Priority:    domain.PriorityNormal,
			Recipient:   "+905551112233",
			Content:     "scheduled sms",
			ScheduledAt: &scheduledAt,
		},
		{
			Channel:   domain.ChannelEmail,
			Priority:  domain.PriorityHigh,
			Recipient: "user@example.com",
			Content:   "send now email",
		},
	})
	if err != nil {
		t.Fatalf("CreateBatch() error = %v", err)
	}
	if batch == nil {
		t.Fatal("batch should not be nil")
	}
	if len(created) != 2 {
		t.Fatalf("created len = %d, want 2", len(created))
	}
	if publishCalls != 1 {
		t.Fatalf("publish calls = %d, want 1", publishCalls)
	}
	if queuedCount != 1 {
		t.Fatalf("queued count = %d, want 1", queuedCount)
	}
	if created[0].Status != domain.StatusAccepted {
		t.Fatalf("scheduled notification status = %s, want ACCEPTED", created[0].Status)
	}
	if created[1].Status != domain.StatusQueued {
		t.Fatalf("immediate notification status = %s, want QUEUED", created[1].Status)
	}
	if batch.Status != domain.BatchStatusCompleted {
		t.Fatalf("batch status = %s, want COMPLETED", batch.Status)
	}
	if updatedBatchStatus != domain.BatchStatusCompleted {
		t.Fatalf("updated batch status = %s, want COMPLETED", updatedBatchStatus)
	}
}

func TestNotificationServiceCancelConflict(t *testing.T) {
	t.Parallel()

	svc, err := NewNotificationService(
		&fakeNotificationRepo{
			cancelFn: func(ctx context.Context, id string) error {
				return domain.ErrConflict
			},
		},
		&fakeBatchRepo{},
		&fakePublisher{},
		nil,
	)
	if err != nil {
		t.Fatalf("NewNotificationService() error = %v", err)
	}

	err = svc.Cancel(context.Background(), "notification-id")
	if !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("Cancel() error = %v, want ErrConflict", err)
	}
}

type fakeNotificationRepo struct {
	createFn              func(ctx context.Context, n *domain.Notification) error
	createBatchFn         func(ctx context.Context, notifications []*domain.Notification) error
	getByIDFn             func(ctx context.Context, id string) (*domain.Notification, error)
	getByIdempotencyKeyFn func(ctx context.Context, idempotencyKey string) (*domain.Notification, error)
	listFn                func(ctx context.Context, params repository.ListParams) ([]domain.Notification, int64, error)
	updateStatusFn        func(ctx context.Context, id string, status domain.Status) error
	markQueuedIfAccepted  func(ctx context.Context, id string) (bool, error)
	updateStatusWithRetry func(ctx context.Context, id string, status domain.Status, nextRetryAt time.Time) error
	cancelFn              func(ctx context.Context, id string) error
	lockForSendingFn      func(ctx context.Context, id string) (*domain.Notification, error)
	getDueForScheduleFn   func(ctx context.Context, limit int) ([]domain.Notification, error)
	getDueForRetryFn      func(ctx context.Context, limit int) ([]domain.Notification, error)
	clearNextRetryAtFn    func(ctx context.Context, id string) error
	setProviderMessageID  func(ctx context.Context, id string, providerMsgID string) error
	getBatchSummaryFn     func(ctx context.Context, batchID string) ([]repository.BatchSummary, error)
}

func (f *fakeNotificationRepo) Create(ctx context.Context, n *domain.Notification) error {
	if f.createFn != nil {
		return f.createFn(ctx, n)
	}
	return nil
}

func (f *fakeNotificationRepo) CreateBatch(ctx context.Context, notifications []*domain.Notification) error {
	if f.createBatchFn != nil {
		return f.createBatchFn(ctx, notifications)
	}
	return nil
}

func (f *fakeNotificationRepo) GetByID(ctx context.Context, id string) (*domain.Notification, error) {
	if f.getByIDFn != nil {
		return f.getByIDFn(ctx, id)
	}
	return nil, domain.ErrNotFound
}

func (f *fakeNotificationRepo) GetByIdempotencyKey(ctx context.Context, idempotencyKey string) (*domain.Notification, error) {
	if f.getByIdempotencyKeyFn != nil {
		return f.getByIdempotencyKeyFn(ctx, idempotencyKey)
	}
	return nil, domain.ErrNotFound
}

func (f *fakeNotificationRepo) List(ctx context.Context, params repository.ListParams) ([]domain.Notification, int64, error) {
	if f.listFn != nil {
		return f.listFn(ctx, params)
	}
	return nil, 0, nil
}

func (f *fakeNotificationRepo) UpdateStatus(ctx context.Context, id string, status domain.Status) error {
	if f.updateStatusFn != nil {
		return f.updateStatusFn(ctx, id, status)
	}
	return nil
}

func (f *fakeNotificationRepo) MarkQueuedIfAccepted(ctx context.Context, id string) (bool, error) {
	if f.markQueuedIfAccepted != nil {
		return f.markQueuedIfAccepted(ctx, id)
	}
	return true, nil
}

func (f *fakeNotificationRepo) UpdateStatusWithRetry(ctx context.Context, id string, status domain.Status, nextRetryAt time.Time) error {
	if f.updateStatusWithRetry != nil {
		return f.updateStatusWithRetry(ctx, id, status, nextRetryAt)
	}
	return nil
}

func (f *fakeNotificationRepo) Cancel(ctx context.Context, id string) error {
	if f.cancelFn != nil {
		return f.cancelFn(ctx, id)
	}
	return nil
}

func (f *fakeNotificationRepo) LockForSending(ctx context.Context, id string) (*domain.Notification, error) {
	if f.lockForSendingFn != nil {
		return f.lockForSendingFn(ctx, id)
	}
	return nil, nil
}

func (f *fakeNotificationRepo) GetDueForSchedule(ctx context.Context, limit int) ([]domain.Notification, error) {
	if f.getDueForScheduleFn != nil {
		return f.getDueForScheduleFn(ctx, limit)
	}
	return nil, nil
}

func (f *fakeNotificationRepo) GetDueForRetry(ctx context.Context, limit int) ([]domain.Notification, error) {
	if f.getDueForRetryFn != nil {
		return f.getDueForRetryFn(ctx, limit)
	}
	return nil, nil
}

func (f *fakeNotificationRepo) ClearNextRetryAt(ctx context.Context, id string) error {
	if f.clearNextRetryAtFn != nil {
		return f.clearNextRetryAtFn(ctx, id)
	}
	return nil
}

func (f *fakeNotificationRepo) SetProviderMessageID(ctx context.Context, id string, providerMsgID string) error {
	if f.setProviderMessageID != nil {
		return f.setProviderMessageID(ctx, id, providerMsgID)
	}
	return nil
}

func (f *fakeNotificationRepo) GetBatchSummary(ctx context.Context, batchID string) ([]repository.BatchSummary, error) {
	if f.getBatchSummaryFn != nil {
		return f.getBatchSummaryFn(ctx, batchID)
	}
	return nil, nil
}

type fakeBatchRepo struct {
	createFn       func(ctx context.Context, b *domain.Batch) error
	getByIDFn      func(ctx context.Context, id string) (*domain.Batch, error)
	updateStatusFn func(ctx context.Context, id string, status domain.BatchStatus) error
}

func (f *fakeBatchRepo) Create(ctx context.Context, b *domain.Batch) error {
	if f.createFn != nil {
		return f.createFn(ctx, b)
	}
	return nil
}

func (f *fakeBatchRepo) GetByID(ctx context.Context, id string) (*domain.Batch, error) {
	if f.getByIDFn != nil {
		return f.getByIDFn(ctx, id)
	}
	return &domain.Batch{ID: id, TotalCount: 0, Status: domain.BatchStatusProcessing}, nil
}

func (f *fakeBatchRepo) UpdateStatus(ctx context.Context, id string, status domain.BatchStatus) error {
	if f.updateStatusFn != nil {
		return f.updateStatusFn(ctx, id, status)
	}
	return nil
}

type fakePublisher struct {
	publishFn func(ctx context.Context, queueName string, msg queue.NotificationMessage) error
	closeFn   func() error
}

func (f *fakePublisher) Publish(ctx context.Context, queueName string, msg queue.NotificationMessage) error {
	if f.publishFn != nil {
		return f.publishFn(ctx, queueName, msg)
	}
	return nil
}

func (f *fakePublisher) Close() error {
	if f.closeFn != nil {
		return f.closeFn()
	}
	return nil
}
