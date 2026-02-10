package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kursadbilgin/dispatch-engine/internal/domain"
	"github.com/kursadbilgin/dispatch-engine/internal/provider"
	"github.com/kursadbilgin/dispatch-engine/internal/queue"
	"github.com/kursadbilgin/dispatch-engine/internal/ratelimit"
	"go.uber.org/zap"
)

func TestWorkerServiceProcessMessageSuccess(t *testing.T) {
	t.Parallel()

	var gotAttempt *domain.NotificationAttempt
	notification := &domain.Notification{
		ID:           "n1",
		Channel:      domain.ChannelSMS,
		Priority:     domain.PriorityNormal,
		Recipient:    "+905551112233",
		Content:      "hello",
		AttemptCount: 0,
		MaxRetries:   5,
	}

	repo := &fakeNotificationRepo{
		lockForSendingFn: func(ctx context.Context, id string) (*domain.Notification, error) {
			return notification, nil
		},
		setProviderMessageID: func(ctx context.Context, id string, providerMsgID string) error {
			if providerMsgID != "provider-123" {
				t.Fatalf("provider message id = %q, want provider-123", providerMsgID)
			}
			return nil
		},
		updateStatusFn: func(ctx context.Context, id string, status domain.Status) error {
			if status != domain.StatusSent {
				t.Fatalf("status = %s, want SENT", status)
			}
			return nil
		},
	}
	attemptRepo := &fakeAttemptRepo{
		createFn: func(ctx context.Context, a *domain.NotificationAttempt) error {
			gotAttempt = a
			return nil
		},
	}
	providerClient := &fakeProvider{
		sendFn: func(ctx context.Context, n domain.Notification) (*provider.ProviderResponse, error) {
			return &provider.ProviderResponse{
				StatusCode: 202,
				Body:       `{"ok":true}`,
				MessageID:  "provider-123",
			}, nil
		},
	}
	limiter := &fakeRateLimiter{
		waitFn: func(ctx context.Context, channel string) error {
			if channel != "sms" {
				t.Fatalf("channel = %q, want sms", channel)
			}
			return nil
		},
	}

	worker, err := NewWorkerService(
		repo,
		attemptRepo,
		&fakeConsumer{},
		providerClient,
		limiter,
		3,
		zap.NewNop(),
	)
	if err != nil {
		t.Fatalf("NewWorkerService() error = %v", err)
	}
	worker.now = func() time.Time { return time.Unix(1_700_000_000, 0) }
	worker.randIntn = func(n int) int { return 0 }

	err = worker.processMessage(context.Background(), queue.NotificationMessage{
		NotificationID: "n1",
		Channel:        domain.ChannelSMS,
		Priority:       domain.PriorityNormal,
	})
	if err != nil {
		t.Fatalf("processMessage() error = %v", err)
	}

	if gotAttempt == nil {
		t.Fatal("attempt should be recorded")
	}
	if gotAttempt.AttemptNumber != 1 {
		t.Fatalf("attempt number = %d, want 1", gotAttempt.AttemptNumber)
	}
	if gotAttempt.StatusCode == nil || *gotAttempt.StatusCode != 202 {
		t.Fatalf("attempt status code = %v, want 202", gotAttempt.StatusCode)
	}
}

func TestWorkerServiceProcessMessageTransientRetry(t *testing.T) {
	t.Parallel()

	var retryCalled bool
	var nextRetryAt time.Time

	notification := &domain.Notification{
		ID:           "n2",
		Channel:      domain.ChannelSMS,
		Priority:     domain.PriorityNormal,
		Recipient:    "+905551112233",
		Content:      "hello",
		AttemptCount: 0,
		MaxRetries:   5,
	}

	repo := &fakeNotificationRepo{
		lockForSendingFn: func(ctx context.Context, id string) (*domain.Notification, error) {
			return notification, nil
		},
		updateStatusWithRetry: func(ctx context.Context, id string, status domain.Status, next time.Time) error {
			retryCalled = true
			nextRetryAt = next
			if status != domain.StatusQueued {
				t.Fatalf("status = %s, want QUEUED", status)
			}
			return nil
		},
		updateStatusFn: func(ctx context.Context, id string, status domain.Status) error {
			t.Fatalf("UpdateStatus should not be called on transient retry")
			return nil
		},
	}
	attemptRepo := &fakeAttemptRepo{
		createFn: func(ctx context.Context, a *domain.NotificationAttempt) error {
			return nil
		},
	}
	providerClient := &fakeProvider{
		sendFn: func(ctx context.Context, n domain.Notification) (*provider.ProviderResponse, error) {
			return nil, &provider.ProviderError{
				StatusCode: 500,
				Message:    "temporary failure",
				Transient:  true,
			}
		},
	}

	worker, err := NewWorkerService(
		repo,
		attemptRepo,
		&fakeConsumer{},
		providerClient,
		&fakeRateLimiter{},
		3,
		zap.NewNop(),
	)
	if err != nil {
		t.Fatalf("NewWorkerService() error = %v", err)
	}

	baseNow := time.Unix(1_700_000_000, 0)
	worker.now = func() time.Time { return baseNow }
	worker.randIntn = func(n int) int { return 0 }

	err = worker.processMessage(context.Background(), queue.NotificationMessage{
		NotificationID: "n2",
		Channel:        domain.ChannelSMS,
		Priority:       domain.PriorityNormal,
	})
	if err != nil {
		t.Fatalf("processMessage() error = %v", err)
	}
	if !retryCalled {
		t.Fatal("expected retry status update to be called")
	}

	wantNext := baseNow.Add(time.Second)
	if !nextRetryAt.Equal(wantNext) {
		t.Fatalf("nextRetryAt = %v, want %v", nextRetryAt, wantNext)
	}
}

func TestWorkerServiceProcessMessageRateLimiterError(t *testing.T) {
	t.Parallel()

	providerCalled := false
	repo := &fakeNotificationRepo{
		lockForSendingFn: func(ctx context.Context, id string) (*domain.Notification, error) {
			return &domain.Notification{
				ID:           "n-rate-limit",
				Channel:      domain.ChannelSMS,
				Priority:     domain.PriorityNormal,
				Recipient:    "+905551112233",
				Content:      "hello",
				AttemptCount: 0,
				MaxRetries:   5,
			}, nil
		},
	}

	providerClient := &fakeProvider{
		sendFn: func(ctx context.Context, n domain.Notification) (*provider.ProviderResponse, error) {
			providerCalled = true
			return &provider.ProviderResponse{StatusCode: 202}, nil
		},
	}

	worker, err := NewWorkerService(
		repo,
		&fakeAttemptRepo{},
		&fakeConsumer{},
		providerClient,
		&fakeRateLimiter{
			waitFn: func(ctx context.Context, channel string) error {
				return errors.New("rate limit wait timeout")
			},
		},
		3,
		zap.NewNop(),
	)
	if err != nil {
		t.Fatalf("NewWorkerService() error = %v", err)
	}

	err = worker.processMessage(context.Background(), queue.NotificationMessage{
		NotificationID: "n-rate-limit",
		Channel:        domain.ChannelSMS,
		Priority:       domain.PriorityNormal,
	})
	if err == nil {
		t.Fatal("processMessage() expected error, got nil")
	}
	if !strings.Contains(err.Error(), "rate limiter wait failed") {
		t.Fatalf("processMessage() error = %v, want rate limiter wait failure", err)
	}
	if providerCalled {
		t.Fatal("provider should not be called when rate limiter fails")
	}
}

func TestWorkerServiceProcessMessageTransientMaxRetries(t *testing.T) {
	t.Parallel()

	var failedCalled bool

	notification := &domain.Notification{
		ID:           "n3",
		Channel:      domain.ChannelSMS,
		Priority:     domain.PriorityNormal,
		Recipient:    "+905551112233",
		Content:      "hello",
		AttemptCount: 4,
		MaxRetries:   5,
	}

	repo := &fakeNotificationRepo{
		lockForSendingFn: func(ctx context.Context, id string) (*domain.Notification, error) {
			return notification, nil
		},
		updateStatusFn: func(ctx context.Context, id string, status domain.Status) error {
			if status != domain.StatusFailed {
				t.Fatalf("status = %s, want FAILED", status)
			}
			failedCalled = true
			return nil
		},
		updateStatusWithRetry: func(ctx context.Context, id string, status domain.Status, nextRetryAt time.Time) error {
			t.Fatalf("UpdateStatusWithRetry should not be called at max retries")
			return nil
		},
	}

	providerClient := &fakeProvider{
		sendFn: func(ctx context.Context, n domain.Notification) (*provider.ProviderResponse, error) {
			return nil, &provider.ProviderError{
				StatusCode: 503,
				Message:    "temporary failure",
				Transient:  true,
			}
		},
	}

	worker, err := NewWorkerService(
		repo,
		&fakeAttemptRepo{},
		&fakeConsumer{},
		providerClient,
		&fakeRateLimiter{},
		3,
		zap.NewNop(),
	)
	if err != nil {
		t.Fatalf("NewWorkerService() error = %v", err)
	}

	err = worker.processMessage(context.Background(), queue.NotificationMessage{
		NotificationID: "n3",
		Channel:        domain.ChannelSMS,
		Priority:       domain.PriorityNormal,
	})
	if err != nil {
		t.Fatalf("processMessage() error = %v", err)
	}
	if !failedCalled {
		t.Fatal("expected status to be updated as FAILED")
	}
}

func TestWorkerServiceProcessMessagePermanentFailure(t *testing.T) {
	t.Parallel()

	var failedCalled bool

	notification := &domain.Notification{
		ID:           "n4",
		Channel:      domain.ChannelSMS,
		Priority:     domain.PriorityNormal,
		Recipient:    "+905551112233",
		Content:      "hello",
		AttemptCount: 0,
		MaxRetries:   5,
	}

	repo := &fakeNotificationRepo{
		lockForSendingFn: func(ctx context.Context, id string) (*domain.Notification, error) {
			return notification, nil
		},
		updateStatusFn: func(ctx context.Context, id string, status domain.Status) error {
			if status != domain.StatusFailed {
				t.Fatalf("status = %s, want FAILED", status)
			}
			failedCalled = true
			return nil
		},
	}

	providerClient := &fakeProvider{
		sendFn: func(ctx context.Context, n domain.Notification) (*provider.ProviderResponse, error) {
			return nil, &provider.ProviderError{
				StatusCode: 400,
				Message:    "invalid request",
				Transient:  false,
			}
		},
	}

	worker, err := NewWorkerService(
		repo,
		&fakeAttemptRepo{},
		&fakeConsumer{},
		providerClient,
		&fakeRateLimiter{},
		3,
		zap.NewNop(),
	)
	if err != nil {
		t.Fatalf("NewWorkerService() error = %v", err)
	}

	err = worker.processMessage(context.Background(), queue.NotificationMessage{
		NotificationID: "n4",
		Channel:        domain.ChannelSMS,
		Priority:       domain.PriorityNormal,
	})
	if err != nil {
		t.Fatalf("processMessage() error = %v", err)
	}
	if !failedCalled {
		t.Fatal("expected status to be updated as FAILED")
	}
}

func TestWorkerServiceProcessMessageSkipTerminal(t *testing.T) {
	t.Parallel()

	providerCalled := false
	limiterCalled := false

	repo := &fakeNotificationRepo{
		lockForSendingFn: func(ctx context.Context, id string) (*domain.Notification, error) {
			return nil, nil
		},
	}

	providerClient := &fakeProvider{
		sendFn: func(ctx context.Context, n domain.Notification) (*provider.ProviderResponse, error) {
			providerCalled = true
			return nil, nil
		},
	}

	limiter := &fakeRateLimiter{
		waitFn: func(ctx context.Context, channel string) error {
			limiterCalled = true
			return nil
		},
	}

	worker, err := NewWorkerService(
		repo,
		&fakeAttemptRepo{},
		&fakeConsumer{},
		providerClient,
		limiter,
		3,
		zap.NewNop(),
	)
	if err != nil {
		t.Fatalf("NewWorkerService() error = %v", err)
	}

	err = worker.processMessage(context.Background(), queue.NotificationMessage{
		NotificationID: "n5",
		Channel:        domain.ChannelSMS,
		Priority:       domain.PriorityNormal,
	})
	if err != nil {
		t.Fatalf("processMessage() error = %v", err)
	}

	if providerCalled {
		t.Fatal("provider should not be called for skipped notification")
	}
	if limiterCalled {
		t.Fatal("rate limiter should not be called for skipped notification")
	}
}

type fakeProvider struct {
	sendFn func(ctx context.Context, notification domain.Notification) (*provider.ProviderResponse, error)
}

func (f *fakeProvider) Send(ctx context.Context, notification domain.Notification) (*provider.ProviderResponse, error) {
	if f.sendFn != nil {
		return f.sendFn(ctx, notification)
	}
	return &provider.ProviderResponse{}, nil
}

type fakeRateLimiter struct {
	allowFn func(ctx context.Context, channel string) (bool, error)
	waitFn  func(ctx context.Context, channel string) error
}

func (f *fakeRateLimiter) Allow(ctx context.Context, channel string) (bool, error) {
	if f.allowFn != nil {
		return f.allowFn(ctx, channel)
	}
	return true, nil
}

func (f *fakeRateLimiter) Wait(ctx context.Context, channel string) error {
	if f.waitFn != nil {
		return f.waitFn(ctx, channel)
	}
	return nil
}

var _ ratelimit.RateLimiter = (*fakeRateLimiter)(nil)

type fakeConsumer struct {
	consumeFn func(ctx context.Context, queue string, handler queue.MessageHandler) error
	closeFn   func() error
}

func (f *fakeConsumer) Consume(ctx context.Context, queueName string, handler queue.MessageHandler) error {
	if f.consumeFn != nil {
		return f.consumeFn(ctx, queueName, handler)
	}
	return nil
}

func (f *fakeConsumer) Close() error {
	if f.closeFn != nil {
		return f.closeFn()
	}
	return nil
}

type fakeAttemptRepo struct {
	createFn              func(ctx context.Context, a *domain.NotificationAttempt) error
	getByNotificationIDFn func(ctx context.Context, notificationID string) ([]domain.NotificationAttempt, error)
}

func (f *fakeAttemptRepo) Create(ctx context.Context, a *domain.NotificationAttempt) error {
	if f.createFn != nil {
		return f.createFn(ctx, a)
	}
	return nil
}

func (f *fakeAttemptRepo) GetByNotificationID(ctx context.Context, notificationID string) ([]domain.NotificationAttempt, error) {
	if f.getByNotificationIDFn != nil {
		return f.getByNotificationIDFn(ctx, notificationID)
	}
	return nil, nil
}

func TestWorkerServiceProcessMessageLockNotFoundAck(t *testing.T) {
	t.Parallel()

	repo := &fakeNotificationRepo{
		lockForSendingFn: func(ctx context.Context, id string) (*domain.Notification, error) {
			return nil, domain.ErrNotFound
		},
	}

	worker, err := NewWorkerService(
		repo,
		&fakeAttemptRepo{},
		&fakeConsumer{},
		&fakeProvider{},
		&fakeRateLimiter{},
		3,
		zap.NewNop(),
	)
	if err != nil {
		t.Fatalf("NewWorkerService() error = %v", err)
	}

	if err := worker.processMessage(context.Background(), queue.NotificationMessage{
		NotificationID: "missing",
		Channel:        domain.ChannelSMS,
		Priority:       domain.PriorityNormal,
	}); err != nil {
		t.Fatalf("processMessage() unexpected error: %v", err)
	}
}

func TestWorkerServiceStartPropagatesConsumerError(t *testing.T) {
	t.Parallel()

	consumeErr := errors.New("consume failed")
	consumer := &fakeConsumer{
		consumeFn: func(ctx context.Context, queueName string, handler queue.MessageHandler) error {
			return consumeErr
		},
	}

	worker, err := NewWorkerService(
		&fakeNotificationRepo{},
		&fakeAttemptRepo{},
		consumer,
		&fakeProvider{},
		&fakeRateLimiter{},
		3,
		zap.NewNop(),
	)
	if err != nil {
		t.Fatalf("NewWorkerService() error = %v", err)
	}

	err = worker.Start(context.Background())
	if !errors.Is(err, consumeErr) {
		t.Fatalf("Start() error = %v, want %v", err, consumeErr)
	}
}

func TestWorkerServiceComputeRetryDelay(t *testing.T) {
	t.Parallel()

	worker, err := NewWorkerService(
		&fakeNotificationRepo{},
		&fakeAttemptRepo{},
		&fakeConsumer{},
		&fakeProvider{},
		&fakeRateLimiter{},
		3,
		zap.NewNop(),
	)
	if err != nil {
		t.Fatalf("NewWorkerService() error = %v", err)
	}

	worker.randIntn = func(n int) int { return 0 }

	if got := worker.computeRetryDelay(1); got != time.Second {
		t.Fatalf("computeRetryDelay(1) = %v, want %v", got, time.Second)
	}

	if got := worker.computeRetryDelay(10); got != maxRetryDelay {
		t.Fatalf("computeRetryDelay(10) = %v, want %v", got, maxRetryDelay)
	}

	worker.randIntn = func(n int) int {
		if n != maxRetryJitterMillis+1 {
			t.Fatalf("randIntn arg = %d, want %d", n, maxRetryJitterMillis+1)
		}
		return 125
	}

	want := 2*time.Second + 125*time.Millisecond
	if got := worker.computeRetryDelay(2); got != want {
		t.Fatalf("computeRetryDelay(2) = %v, want %v", got, want)
	}
}
