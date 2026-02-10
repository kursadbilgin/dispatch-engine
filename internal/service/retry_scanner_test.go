package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kursadbilgin/dispatch-engine/internal/domain"
	"github.com/kursadbilgin/dispatch-engine/internal/queue"
	"go.uber.org/zap"
)

func TestNewRetryScannerValidation(t *testing.T) {
	t.Parallel()

	_, err := NewRetryScanner(nil, &fakePublisher{}, 0, 0, zap.NewNop())
	if err == nil {
		t.Fatal("expected error when notification repository is nil")
	}

	_, err = NewRetryScanner(&fakeNotificationRepo{}, nil, 0, 0, zap.NewNop())
	if err == nil {
		t.Fatal("expected error when publisher is nil")
	}
}

func TestRetryScannerScanDuePublishesNotifications(t *testing.T) {
	t.Parallel()

	cleared := make([]string, 0, 2)
	repo := &fakeNotificationRepo{
		getDueForRetryFn: func(ctx context.Context, limit int) ([]domain.Notification, error) {
			if limit != 100 {
				t.Fatalf("limit = %d, want 100", limit)
			}
			return []domain.Notification{
				{
					ID:            "n-sms-1",
					CorrelationID: "c-1",
					Channel:       domain.ChannelSMS,
					Priority:      domain.PriorityHigh,
				},
				{
					ID:            "n-email-1",
					CorrelationID: "c-2",
					Channel:       domain.ChannelEmail,
					Priority:      domain.PriorityLow,
				},
			}, nil
		},
		clearNextRetryAtFn: func(ctx context.Context, id string) error {
			cleared = append(cleared, id)
			return nil
		},
	}

	published := make([]string, 0, 2)
	publisher := &fakePublisher{
		publishFn: func(ctx context.Context, queueName string, msg queue.NotificationMessage) error {
			published = append(published, queueName+":"+msg.NotificationID)
			return nil
		},
	}

	scanner, err := NewRetryScanner(repo, publisher, 5*time.Second, 100, zap.NewNop())
	if err != nil {
		t.Fatalf("NewRetryScanner() error = %v", err)
	}

	if err := scanner.scanDue(context.Background()); err != nil {
		t.Fatalf("scanDue() error = %v", err)
	}

	if len(published) != 2 {
		t.Fatalf("published count = %d, want 2", len(published))
	}
	if published[0] != "sms:n-sms-1" {
		t.Fatalf("first published = %s, want sms:n-sms-1", published[0])
	}
	if published[1] != "email:n-email-1" {
		t.Fatalf("second published = %s, want email:n-email-1", published[1])
	}
	if len(cleared) != 2 {
		t.Fatalf("clearNextRetryAt count = %d, want 2", len(cleared))
	}
}

func TestRetryScannerScanDueContinuesOnPublishError(t *testing.T) {
	t.Parallel()

	repo := &fakeNotificationRepo{
		getDueForRetryFn: func(ctx context.Context, limit int) ([]domain.Notification, error) {
			return []domain.Notification{
				{ID: "n1", Channel: domain.ChannelSMS, Priority: domain.PriorityNormal},
				{ID: "n2", Channel: domain.ChannelPush, Priority: domain.PriorityNormal},
			}, nil
		},
	}

	calls := 0
	publisher := &fakePublisher{
		publishFn: func(ctx context.Context, queueName string, msg queue.NotificationMessage) error {
			calls++
			if msg.NotificationID == "n1" {
				return errors.New("publish failed")
			}
			return nil
		},
	}

	scanner, err := NewRetryScanner(repo, publisher, time.Second, 100, zap.NewNop())
	if err != nil {
		t.Fatalf("NewRetryScanner() error = %v", err)
	}

	if err := scanner.scanDue(context.Background()); err != nil {
		t.Fatalf("scanDue() error = %v", err)
	}

	if calls != 2 {
		t.Fatalf("publish calls = %d, want 2", calls)
	}
}

func TestRetryScannerScanDueRepositoryError(t *testing.T) {
	t.Parallel()

	repo := &fakeNotificationRepo{
		getDueForRetryFn: func(ctx context.Context, limit int) ([]domain.Notification, error) {
			return nil, errors.New("db unavailable")
		},
	}

	scanner, err := NewRetryScanner(repo, &fakePublisher{}, time.Second, 100, zap.NewNop())
	if err != nil {
		t.Fatalf("NewRetryScanner() error = %v", err)
	}

	err = scanner.scanDue(context.Background())
	if err == nil {
		t.Fatal("expected scanDue() error")
	}
}

func TestRetryScannerStartReturnsOnContextCancel(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	scanner, err := NewRetryScanner(&fakeNotificationRepo{}, &fakePublisher{}, time.Second, 100, zap.NewNop())
	if err != nil {
		t.Fatalf("NewRetryScanner() error = %v", err)
	}

	if err := scanner.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
}
