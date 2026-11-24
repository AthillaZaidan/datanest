package queue

import (
	"context"
	"errors"
	"log/slog"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

const (
	minBackoff = time.Second
	maxBackoff = 30 * time.Second
)

var errDeliveriesClosed = errors.New("queue: the broker closed the delivery stream")

type Consumer struct {
	url      string
	queue    string
	prefetch int
	dial     func(url string) (*amqp.Connection, error)
}

func NewConsumer(url, queue string, prefetch int) *Consumer {
	return &Consumer{url: url, queue: queue, prefetch: max(prefetch, 1), dial: dial}
}

func (c *Consumer) Run(ctx context.Context, handle func(context.Context, Message)) error {
	backoff := minBackoff
	for ctx.Err() == nil {
		connected, err := c.session(ctx, handle)
		if connected {
			backoff = minBackoff
		}
		if ctx.Err() != nil {
			break
		}
		slog.Warn("queue.consumer_disconnected", "queue", c.queue, "error", err, "retry_in", backoff)
		select {
		case <-ctx.Done():
		case <-time.After(backoff):
		}
		backoff = min(backoff*2, maxBackoff)
	}
	return nil
}

func (c *Consumer) session(ctx context.Context, handle func(context.Context, Message)) (bool, error) {
	conn, err := c.dial(c.url)
	if err != nil {
		return false, err
	}
	defer conn.Close()

	ch, err := conn.Channel()
	if err != nil {
		return false, err
	}
	if err := declare(ch, c.queue); err != nil {
		return false, err
	}
	if err := ch.Qos(c.prefetch, 0, false); err != nil {
		return false, err
	}
	deliveries, err := ch.Consume(c.queue, "", false, false, false, false, nil)
	if err != nil {
		return false, err
	}

	for {
		select {
		case <-ctx.Done():
			return true, nil
		case d, ok := <-deliveries:
			if !ok {
				return true, errDeliveriesClosed
			}
			msg, err := parse(d)
			if err != nil {
				slog.Warn("queue.poison_message", "queue", c.queue, "error", err)
				d.Nack(false, false)
				continue
			}
			handle(ctx, msg)
		}
	}
}
