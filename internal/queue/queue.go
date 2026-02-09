package queue

import (
	"context"
	"fmt"
	"strings"

	"github.com/kursadbilgin/dispatch-engine/internal/domain"
)

// Publisher publishes notification messages to a queue.
type Publisher interface {
	Publish(ctx context.Context, queue string, msg NotificationMessage) error
	Close() error
}

// MessageHandler handles a consumed queue message.
type MessageHandler func(ctx context.Context, msg NotificationMessage) error

// Consumer consumes notification messages from a queue.
type Consumer interface {
	Consume(ctx context.Context, queue string, handler MessageHandler) error
	Close() error
}

var supportedChannels = []domain.Channel{
	domain.ChannelSMS,
	domain.ChannelEmail,
	domain.ChannelPush,
}

const (
	// queueMaxPriority is the RabbitMQ x-max-priority value for work queues.
	queueMaxPriority int32 = 3
)

// QueueName returns the channel work queue name, e.g. sms.
func QueueName(channel domain.Channel) string {
	return strings.ToLower(channel.String())
}

// DLQName returns the dead-letter queue name for a channel, e.g. dlq.sms.
func DLQName(channel domain.Channel) string {
	return fmt.Sprintf("dlq.%s", QueueName(channel))
}

// WorkQueueNames returns all channel work queues (3 total).
func WorkQueueNames() []string {
	queues := make([]string, 0, len(supportedChannels))
	for _, channel := range supportedChannels {
		queues = append(queues, QueueName(channel))
	}
	return queues
}

// DLQNames returns all dead-letter queues (3 total).
func DLQNames() []string {
	queues := make([]string, 0, len(supportedChannels))
	for _, channel := range supportedChannels {
		queues = append(queues, DLQName(channel))
	}
	return queues
}

// PriorityValue maps domain priority to RabbitMQ message priority.
func PriorityValue(priority domain.Priority) uint8 {
	switch priority {
	case domain.PriorityHigh:
		return 3
	case domain.PriorityNormal:
		return 2
	case domain.PriorityLow:
		return 1
	default:
		return 0
	}
}
