package service

import (
	"context"
	"fmt"
	"time"

	"github.com/kursadbilgin/dispatch-engine/internal/queue"
	"github.com/kursadbilgin/dispatch-engine/internal/repository"
	"go.uber.org/zap"
)

const (
	defaultRetryScanInterval = 5 * time.Second
	defaultRetryScanLimit    = 100
)

// RetryScanner periodically re-enqueues due notifications marked for retry.
type RetryScanner struct {
	notifications repository.NotificationRepository
	publisher     queue.Publisher
	logger        *zap.Logger
	interval      time.Duration
	limit         int
}

func NewRetryScanner(
	notifications repository.NotificationRepository,
	publisher queue.Publisher,
	interval time.Duration,
	limit int,
	logger *zap.Logger,
) (*RetryScanner, error) {
	if notifications == nil {
		return nil, fmt.Errorf("notification repository is required")
	}
	if publisher == nil {
		return nil, fmt.Errorf("publisher is required")
	}
	if interval <= 0 {
		interval = defaultRetryScanInterval
	}
	if limit <= 0 {
		limit = defaultRetryScanLimit
	}
	if logger == nil {
		logger = zap.NewNop()
	}

	return &RetryScanner{
		notifications: notifications,
		publisher:     publisher,
		logger:        logger,
		interval:      interval,
		limit:         limit,
	}, nil
}

func (s *RetryScanner) Start(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}

	// Run an initial scan so already-due retries do not wait for the first ticker edge.
	if err := s.scanDue(ctx); err != nil && ctx.Err() == nil {
		s.logger.Error("retry scanner initial scan failed", zap.Error(err))
	}

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := s.scanDue(ctx); err != nil {
				if ctx.Err() != nil {
					return nil
				}
				s.logger.Error("retry scanner scan failed", zap.Error(err))
			}
		}
	}
}

func (s *RetryScanner) scanDue(ctx context.Context) error {
	dueNotifications, err := s.notifications.GetDueForRetry(ctx, s.limit)
	if err != nil {
		return fmt.Errorf("failed to fetch due retries: %w", err)
	}

	for i := range dueNotifications {
		notification := dueNotifications[i]
		msg := queue.NotificationMessage{
			NotificationID: notification.ID,
			CorrelationID:  notification.CorrelationID,
			Channel:        notification.Channel,
			Priority:       notification.Priority,
		}

		queueName := queue.QueueName(notification.Channel)
		if err := s.publisher.Publish(ctx, queueName, msg); err != nil {
			s.logger.Error("failed to enqueue retry notification",
				zap.String("notificationId", notification.ID),
				zap.String("queue", queueName),
				zap.Error(err),
			)
			continue
		}

		if err := s.notifications.ClearNextRetryAt(ctx, notification.ID); err != nil {
			s.logger.Error("failed to clear next retry timestamp after enqueue",
				zap.String("notificationId", notification.ID),
				zap.Error(err),
			)
			continue
		}
	}

	return nil
}
