package service

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/kursadbilgin/dispatch-engine/internal/domain"
	"github.com/kursadbilgin/dispatch-engine/internal/observability"
	"github.com/kursadbilgin/dispatch-engine/internal/provider"
	"github.com/kursadbilgin/dispatch-engine/internal/queue"
	"github.com/kursadbilgin/dispatch-engine/internal/ratelimit"
	"github.com/kursadbilgin/dispatch-engine/internal/repository"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
)

const (
	minWorkerConcurrency = 1
	maxRetryDelay        = 60 * time.Second
	baseRetryDelay       = time.Second
	maxRetryJitterMillis = 250
)

type WorkerService struct {
	notifications repository.NotificationRepository
	attempts      repository.AttemptRepository
	consumer      queue.Consumer
	provider      provider.Provider
	rateLimiter   ratelimit.RateLimiter
	logger        *zap.Logger
	metrics       *observability.Metrics
	concurrency   int
	now           func() time.Time
	randIntn      func(n int) int
}

func NewWorkerService(
	notifications repository.NotificationRepository,
	attempts repository.AttemptRepository,
	consumer queue.Consumer,
	provider provider.Provider,
	rateLimiter ratelimit.RateLimiter,
	concurrency int,
	logger *zap.Logger,
) (*WorkerService, error) {
	if concurrency < minWorkerConcurrency {
		concurrency = minWorkerConcurrency
	}
	if logger == nil {
		logger = zap.NewNop()
	}

	return &WorkerService{
		notifications: notifications,
		attempts:      attempts,
		consumer:      consumer,
		provider:      provider,
		rateLimiter:   rateLimiter,
		logger:        logger,
		concurrency:   concurrency,
		now:           time.Now,
		randIntn:      rand.Intn,
	}, nil
}

// Start consumes channel queues and processes notification messages until context cancellation.
func (s *WorkerService) Start(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}

	queueNames := queue.WorkQueueNames()
	if len(queueNames) == 0 {
		return fmt.Errorf("no work queues configured")
	}

	g, groupCtx := errgroup.WithContext(ctx)
	for i := 0; i < s.concurrency; i++ {
		queueName := queueNames[i%len(queueNames)]
		workerID := i + 1

		g.Go(func() error {
			s.logger.Info("worker started",
				zap.Int("workerId", workerID),
				zap.String("queue", queueName),
			)

			err := s.consumer.Consume(groupCtx, queueName, s.processMessage)
			if err != nil {
				s.logger.Error("worker stopped with error",
					zap.Int("workerId", workerID),
					zap.String("queue", queueName),
					zap.Error(err),
				)
				return err
			}

			s.logger.Info("worker stopped",
				zap.Int("workerId", workerID),
				zap.String("queue", queueName),
			)
			return nil
		})
	}

	return g.Wait()
}

func (s *WorkerService) processMessage(ctx context.Context, msg queue.NotificationMessage) error {
	notification, err := s.notifications.LockForSending(ctx, msg.NotificationID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			s.logger.Warn("notification not found during lock, skipping",
				zap.String("notificationId", msg.NotificationID),
			)
			return nil
		}
		return fmt.Errorf("failed to lock notification for sending: %w", err)
	}

	// Nil means terminal/sending state; ack and skip.
	if notification == nil {
		return nil
	}

	channelName := strings.ToLower(notification.Channel.String())
	if s.metrics != nil {
		s.metrics.IncWorkerInFlight(channelName)
		defer s.metrics.DecWorkerInFlight(channelName)
	}

	if err := s.rateLimiter.Wait(ctx, channelName); err != nil {
		return fmt.Errorf("rate limiter wait failed: %w", err)
	}

	attemptNumber := notification.AttemptCount + 1
	sendStart := s.now()
	providerResp, sendErr := s.provider.Send(ctx, *notification)
	if s.metrics != nil {
		s.metrics.ObserveNotificationSendDuration(channelName, s.now().Sub(sendStart))
	}

	if err := s.recordAttempt(ctx, notification.ID, attemptNumber, providerResp, sendErr); err != nil {
		return fmt.Errorf("failed to record attempt: %w", err)
	}

	if sendErr == nil {
		if providerResp != nil && strings.TrimSpace(providerResp.MessageID) != "" {
			if err := s.notifications.SetProviderMessageID(ctx, notification.ID, providerResp.MessageID); err != nil {
				return fmt.Errorf("failed to set provider message id: %w", err)
			}
		}

		if err := s.notifications.UpdateStatus(ctx, notification.ID, domain.StatusSent); err != nil {
			return fmt.Errorf("failed to update notification status to sent: %w", err)
		}
		if s.metrics != nil {
			s.metrics.IncNotificationSent(channelName)
		}
		return nil
	}

	isTransient := provider.IsTransient(sendErr)
	maxRetries := notification.MaxRetries
	if maxRetries <= 0 {
		maxRetries = defaultMaxRetries
	}

	if isTransient && attemptNumber < maxRetries {
		nextRetryAt := s.now().Add(s.computeRetryDelay(attemptNumber))
		if err := s.notifications.UpdateStatusWithRetry(ctx, notification.ID, domain.StatusQueued, nextRetryAt); err != nil {
			return fmt.Errorf("failed to update notification for retry: %w", err)
		}
		if s.metrics != nil {
			s.metrics.IncRetryScheduled(channelName)
		}
		return nil
	}

	if err := s.notifications.UpdateStatus(ctx, notification.ID, domain.StatusFailed); err != nil {
		return fmt.Errorf("failed to update notification status to failed: %w", err)
	}
	if s.metrics != nil {
		reason := "permanent_error"
		if isTransient {
			reason = "retry_exhausted"
		}
		s.metrics.IncNotificationFailed(channelName, reason)
	}

	return nil
}

func (s *WorkerService) SetMetrics(metrics *observability.Metrics) {
	if s == nil {
		return
	}
	s.metrics = metrics
}

func (s *WorkerService) computeRetryDelay(attemptNumber int) time.Duration {
	if attemptNumber < 1 {
		attemptNumber = 1
	}

	delay := baseRetryDelay
	for i := 1; i < attemptNumber; i++ {
		delay *= 2
		if delay >= maxRetryDelay {
			delay = maxRetryDelay
			break
		}
	}

	if delay > maxRetryDelay {
		delay = maxRetryDelay
	}

	jitterMillis := 0
	if s.randIntn != nil && maxRetryJitterMillis > 0 {
		jitterMillis = s.randIntn(maxRetryJitterMillis + 1)
	}

	return delay + time.Duration(jitterMillis)*time.Millisecond
}

func (s *WorkerService) recordAttempt(
	ctx context.Context,
	notificationID string,
	attemptNumber int,
	providerResp *provider.ProviderResponse,
	sendErr error,
) error {
	var statusCode *int
	var responseBody *string
	var attemptErr *string

	if providerResp != nil {
		if providerResp.StatusCode > 0 {
			value := providerResp.StatusCode
			statusCode = &value
		}
		if body := strings.TrimSpace(providerResp.Body); body != "" {
			value := providerResp.Body
			responseBody = &value
		}
	}

	if sendErr != nil {
		value := sendErr.Error()
		attemptErr = &value

		var providerErr *provider.ProviderError
		if errors.As(sendErr, &providerErr) && providerErr.StatusCode > 0 && statusCode == nil {
			value := providerErr.StatusCode
			statusCode = &value
		}
	}

	attempt := &domain.NotificationAttempt{
		ID:             uuid.NewString(),
		NotificationID: notificationID,
		AttemptNumber:  attemptNumber,
		StatusCode:     statusCode,
		ResponseBody:   responseBody,
		Error:          attemptErr,
		CreatedAt:      s.now().UTC(),
	}

	return s.attempts.Create(ctx, attempt)
}
