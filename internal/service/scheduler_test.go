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

func TestNewSchedulerAppliesDefaults(t *testing.T) {
	t.Parallel()

	scheduler, err := NewScheduler(&fakeNotificationRepo{}, &fakePublisher{}, 0, 0, nil)
	if err != nil {
		t.Fatalf("NewScheduler() error = %v", err)
	}
	if scheduler.interval != defaultSchedulerScanInterval {
		t.Fatalf("interval = %s, want %s", scheduler.interval, defaultSchedulerScanInterval)
	}
	if scheduler.limit != defaultSchedulerScanLimit {
		t.Fatalf("limit = %d, want %d", scheduler.limit, defaultSchedulerScanLimit)
	}
}

func TestSchedulerScanDuePublishesAndMarksQueued(t *testing.T) {
	t.Parallel()

	marked := make([]string, 0, 2)
	repo := &fakeNotificationRepo{
		getDueForScheduleFn: func(ctx context.Context, limit int) ([]domain.Notification, error) {
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
		markQueuedIfAccepted: func(ctx context.Context, id string) (bool, error) {
			marked = append(marked, id)
			return true, nil
		},
	}

	published := make([]string, 0, 2)
	publisher := &fakePublisher{
		publishFn: func(ctx context.Context, queueName string, msg queue.NotificationMessage) error {
			published = append(published, queueName+":"+msg.NotificationID)
			return nil
		},
	}

	scheduler, err := NewScheduler(repo, publisher, 5*time.Second, 100, zap.NewNop())
	if err != nil {
		t.Fatalf("NewScheduler() error = %v", err)
	}

	if err := scheduler.scanDue(context.Background()); err != nil {
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
	if len(marked) != 2 {
		t.Fatalf("markQueuedIfAccepted count = %d, want 2", len(marked))
	}
}

func TestSchedulerScanDueContinuesOnPublishError(t *testing.T) {
	t.Parallel()

	marked := 0
	repo := &fakeNotificationRepo{
		getDueForScheduleFn: func(ctx context.Context, limit int) ([]domain.Notification, error) {
			return []domain.Notification{
				{ID: "n1", Channel: domain.ChannelSMS, Priority: domain.PriorityNormal},
				{ID: "n2", Channel: domain.ChannelPush, Priority: domain.PriorityNormal},
			}, nil
		},
		markQueuedIfAccepted: func(ctx context.Context, id string) (bool, error) {
			marked++
			return true, nil
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

	scheduler, err := NewScheduler(repo, publisher, time.Second, 100, zap.NewNop())
	if err != nil {
		t.Fatalf("NewScheduler() error = %v", err)
	}

	if err := scheduler.scanDue(context.Background()); err != nil {
		t.Fatalf("scanDue() error = %v", err)
	}

	if calls != 2 {
		t.Fatalf("publish calls = %d, want 2", calls)
	}
	if marked != 1 {
		t.Fatalf("marked count = %d, want 1", marked)
	}
}

func TestSchedulerScanDueRepositoryError(t *testing.T) {
	t.Parallel()

	repo := &fakeNotificationRepo{
		getDueForScheduleFn: func(ctx context.Context, limit int) ([]domain.Notification, error) {
			return nil, errors.New("db unavailable")
		},
	}

	scheduler, err := NewScheduler(repo, &fakePublisher{}, time.Second, 100, zap.NewNop())
	if err != nil {
		t.Fatalf("NewScheduler() error = %v", err)
	}

	err = scheduler.scanDue(context.Background())
	if err == nil {
		t.Fatal("expected scanDue() error")
	}
}

func TestSchedulerStartReturnsOnContextCancel(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	scheduler, err := NewScheduler(&fakeNotificationRepo{}, &fakePublisher{}, time.Second, 100, zap.NewNop())
	if err != nil {
		t.Fatalf("NewScheduler() error = %v", err)
	}

	if err := scheduler.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
}
