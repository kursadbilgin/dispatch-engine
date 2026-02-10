package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"go.uber.org/zap"
)

type RabbitMQConsumer struct {
	client   *RabbitMQ
	prefetch int
	logger   *zap.Logger
}

func NewRabbitMQConsumer(client *RabbitMQ, prefetch int, logger *zap.Logger) *RabbitMQConsumer {
	if prefetch < 1 {
		prefetch = 1
	}
	if logger == nil {
		logger = zap.NewNop()
	}

	return &RabbitMQConsumer{
		client:   client,
		prefetch: prefetch,
		logger:   logger,
	}
}

func (c *RabbitMQConsumer) Consume(ctx context.Context, queue string, handler MessageHandler) error {
	if c == nil || c.client == nil {
		return fmt.Errorf("consumer is not initialized")
	}
	if queue == "" {
		return fmt.Errorf("queue name is required")
	}
	if handler == nil {
		return fmt.Errorf("message handler is required")
	}

	backoff := reconnectBackoff
	for {
		err := c.consumeOnce(ctx, queue, handler)
		if ctx.Err() != nil {
			return nil
		}
		if err == nil {
			backoff = reconnectBackoff
			continue
		}

		select {
		case <-ctx.Done():
			return nil
		case <-time.After(backoff):
		}

		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

func (c *RabbitMQConsumer) consumeOnce(ctx context.Context, queue string, handler MessageHandler) error {
	ch, err := c.client.channel(ctx)
	if err != nil {
		return err
	}
	defer ch.Close() //nolint:errcheck // best-effort channel close

	if err := ch.Qos(c.prefetch, 0, false); err != nil {
		return fmt.Errorf("failed to set qos: %w", err)
	}

	deliveries, err := ch.Consume(
		queue,
		"",
		false,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		return fmt.Errorf("failed to consume queue %q: %w", queue, err)
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case d, ok := <-deliveries:
			if !ok {
				return fmt.Errorf("delivery channel closed")
			}

			if err := c.handleDelivery(ctx, d, handler); err != nil {
				return err
			}
		}
	}
}

func (c *RabbitMQConsumer) handleDelivery(ctx context.Context, d amqp.Delivery, handler MessageHandler) error {
	var msg NotificationMessage
	if err := json.Unmarshal(d.Body, &msg); err != nil {
		c.logger.Warn("rejecting message: invalid JSON",
			zap.Error(err),
			zap.String("routingKey", d.RoutingKey),
		)
		if rejectErr := d.Reject(false); rejectErr != nil {
			return fmt.Errorf("failed to reject invalid message: %w", rejectErr)
		}
		return nil
	}

	if err := msg.Validate(); err != nil {
		c.logger.Warn("rejecting message: validation failed",
			zap.Error(err),
			zap.String("notificationId", msg.NotificationID),
		)
		if rejectErr := d.Reject(false); rejectErr != nil {
			return fmt.Errorf("failed to reject invalid payload: %w", rejectErr)
		}
		return nil
	}

	if err := handler(ctx, msg); err != nil {
		if nackErr := d.Nack(false, true); nackErr != nil {
			return fmt.Errorf("handler failed and nack failed: %w", nackErr)
		}
		return nil
	}

	if err := d.Ack(false); err != nil {
		return fmt.Errorf("failed to ack delivery: %w", err)
	}

	return nil
}

func (c *RabbitMQConsumer) Close() error {
	if c == nil || c.client == nil {
		return nil
	}
	return c.client.Close()
}
