package queue

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/kursadbilgin/dispatch-engine/internal/domain"
	amqp "github.com/rabbitmq/amqp091-go"
)

const (
	dlxExchangeName  = "dispatch.dlx"
	reconnectBackoff = time.Second
	maxBackoff       = 30 * time.Second
)

// RabbitMQ manages RabbitMQ connectivity and topology declaration.
type RabbitMQ struct {
	url string

	mu          sync.RWMutex
	reconnectMu sync.Mutex
	conn        *amqp.Connection
}

func NewRabbitMQ(url string) (*RabbitMQ, error) {
	if strings.TrimSpace(url) == "" {
		return nil, fmt.Errorf("rabbitmq url is required")
	}

	r := &RabbitMQ{url: url}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := r.ensureConnected(ctx); err != nil {
		return nil, err
	}

	return r, nil
}

func (r *RabbitMQ) Close() error {
	r.mu.Lock()
	conn := r.conn
	r.conn = nil
	r.mu.Unlock()

	if conn == nil || conn.IsClosed() {
		return nil
	}

	return conn.Close()
}

func (r *RabbitMQ) channel(ctx context.Context) (*amqp.Channel, error) {
	if err := r.ensureConnected(ctx); err != nil {
		return nil, err
	}

	r.mu.RLock()
	conn := r.conn
	r.mu.RUnlock()

	if conn == nil || conn.IsClosed() {
		if err := r.ensureConnected(ctx); err != nil {
			return nil, err
		}
		r.mu.RLock()
		conn = r.conn
		r.mu.RUnlock()
	}

	ch, err := conn.Channel()
	if err != nil {
		if errReconnect := r.reconnectWithBackoff(ctx); errReconnect != nil {
			return nil, errReconnect
		}

		r.mu.RLock()
		conn = r.conn
		r.mu.RUnlock()

		ch, err = conn.Channel()
		if err != nil {
			return nil, fmt.Errorf("failed to create rabbitmq channel after reconnect: %w", err)
		}
	}

	if err := declareTopology(ch); err != nil {
		_ = ch.Close()
		return nil, err
	}

	return ch, nil
}

func (r *RabbitMQ) ensureConnected(ctx context.Context) error {
	r.mu.RLock()
	conn := r.conn
	r.mu.RUnlock()

	if conn != nil && !conn.IsClosed() {
		return nil
	}

	return r.reconnectWithBackoff(ctx)
}

func (r *RabbitMQ) reconnectWithBackoff(ctx context.Context) error {
	r.reconnectMu.Lock()
	defer r.reconnectMu.Unlock()

	r.mu.RLock()
	conn := r.conn
	r.mu.RUnlock()
	if conn != nil && !conn.IsClosed() {
		return nil
	}

	wait := reconnectBackoff
	for {
		newConn, err := amqp.Dial(r.url)
		if err == nil {
			r.mu.Lock()
			oldConn := r.conn
			r.conn = newConn
			r.mu.Unlock()

			if oldConn != nil && !oldConn.IsClosed() {
				_ = oldConn.Close()
			}

			return nil
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("rabbitmq reconnect canceled: %w", ctx.Err())
		case <-time.After(wait):
		}

		wait *= 2
		if wait > maxBackoff {
			wait = maxBackoff
		}
	}
}

func declareTopology(ch *amqp.Channel) error {
	if err := ch.ExchangeDeclare(
		dlxExchangeName,
		"direct",
		true,
		false,
		false,
		false,
		nil,
	); err != nil {
		return fmt.Errorf("failed to declare dlx exchange: %w", err)
	}

	for _, channel := range supportedChannels {
		dlqName := DLQName(channel)
		routingKey := channelRoutingKey(channel)

		if _, err := ch.QueueDeclare(
			dlqName,
			true,
			false,
			false,
			false,
			nil,
		); err != nil {
			return fmt.Errorf("failed to declare dlq %q: %w", dlqName, err)
		}

		if err := ch.QueueBind(dlqName, routingKey, dlxExchangeName, false, nil); err != nil {
			return fmt.Errorf("failed to bind dlq %q: %w", dlqName, err)
		}

		queueName := QueueName(channel)

		args := amqp.Table{
			"x-dead-letter-exchange":    dlxExchangeName,
			"x-dead-letter-routing-key": routingKey,
			"x-max-priority":            queueMaxPriority,
		}

		if _, err := ch.QueueDeclare(
			queueName,
			true,
			false,
			false,
			false,
			args,
		); err != nil {
			return fmt.Errorf("failed to declare queue %q: %w", queueName, err)
		}
	}

	return nil
}

func channelRoutingKey(channel domain.Channel) string {
	return strings.ToLower(channel.String())
}
