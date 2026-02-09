package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

type RabbitMQPublisher struct {
	client *RabbitMQ
}

func NewRabbitMQPublisher(client *RabbitMQ) *RabbitMQPublisher {
	return &RabbitMQPublisher{client: client}
}

func (p *RabbitMQPublisher) Publish(ctx context.Context, queue string, msg NotificationMessage) error {
	if p == nil || p.client == nil {
		return fmt.Errorf("publisher is not initialized")
	}
	if queue == "" {
		return fmt.Errorf("queue name is required")
	}
	if err := msg.Validate(); err != nil {
		return fmt.Errorf("invalid notification message: %w", err)
	}

	payload, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal notification message: %w", err)
	}

	ch, err := p.client.channel(ctx)
	if err != nil {
		return err
	}
	defer ch.Close()

	publishing := amqp.Publishing{
		ContentType:   "application/json",
		DeliveryMode:  amqp.Persistent,
		Timestamp:     time.Now().UTC(),
		MessageId:     msg.NotificationID,
		CorrelationId: msg.CorrelationID,
		Priority:      PriorityValue(msg.Priority),
		Body:          payload,
	}

	if err := ch.PublishWithContext(ctx, "", queue, false, false, publishing); err != nil {
		return fmt.Errorf("failed to publish message to queue %q: %w", queue, err)
	}

	return nil
}

func (p *RabbitMQPublisher) Close() error {
	if p == nil || p.client == nil {
		return nil
	}
	return p.client.Close()
}
