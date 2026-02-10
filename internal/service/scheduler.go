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
	defaultSchedulerScanInterval = 5 * time.Second
	defaultSchedulerScanLimit    = 100
)

// Scheduler periodically enqueues due scheduled notifications.
type Scheduler struct {
	notifications repository.NotificationRepository
	publisher     queue.Publisher
	logger        *zap.Logger
	interval      time.Duration
	limit         int
}

func NewScheduler(
	notifications repository.NotificationRepository,
	publisher queue.Publisher,
	interval time.Duration,
	limit int,
	logger *zap.Logger,
) (*Scheduler, error) {
	if interval <= 0 {
		interval = defaultSchedulerScanInterval
	}
	if limit <= 0 {
		limit = defaultSchedulerScanLimit
	}
	if logger == nil {
		logger = zap.NewNop()
	}

	return &Scheduler{
		notifications: notifications,
		publisher:     publisher,
		logger:        logger,
		interval:      interval,
		limit:         limit,
	}, nil
}

func (s *Scheduler) Start(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}

	if err := s.scanDue(ctx); err != nil && ctx.Err() == nil {
		s.logger.Error("scheduler initial scan failed", zap.Error(err))
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
				s.logger.Error("scheduler scan failed", zap.Error(err))
			}
		}
	}
}

func (s *Scheduler) scanDue(ctx context.Context) error {
	dueNotifications, err := s.notifications.GetDueForSchedule(ctx, s.limit)
	if err != nil {
		return fmt.Errorf("failed to fetch due scheduled notifications: %w", err)
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
			s.logger.Error("failed to enqueue scheduled notification",
				zap.String("notificationId", notification.ID),
				zap.String("queue", queueName),
				zap.Error(err),
			)
			continue
		}

		updated, err := s.notifications.MarkQueuedIfAccepted(ctx, notification.ID)
		if err != nil {
			s.logger.Error("failed to mark scheduled notification as queued",
				zap.String("notificationId", notification.ID),
				zap.Error(err),
			)
			continue
		}
		if !updated {
			s.logger.Info("scheduled notification status changed before queue mark",
				zap.String("notificationId", notification.ID),
			)
		}
	}

	return nil
}
