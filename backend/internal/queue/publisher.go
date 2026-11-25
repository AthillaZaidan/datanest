package queue

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"sync"

	amqp "github.com/rabbitmq/amqp091-go"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/AthillaZaidan/datanest/backend/internal/obs"
)

var ErrNotConfirmed = errors.New("queue: the broker did not confirm the message")

type Publisher struct {
	url   string
	queue string

	mu       sync.Mutex
	conn     *amqp.Connection
	ch       *amqp.Channel
	confirms chan amqp.Confirmation
}

func NewPublisher(url, queue string) *Publisher {
	return &Publisher{url: url, queue: queue}
}

func (p *Publisher) Publish(ctx context.Context, job Job) error {
	ctx, span := obs.Start(ctx, obs.SpanQueuePublish, attribute.Int(obs.AttrExecutionID, int(job.ExecutionID)))
	defer span.End()
	// The worker continues from this span; without an active span the stored context is kept.
	if tp := obs.Traceparent(ctx); tp != "" {
		job.Traceparent = tp
	}
	if err := p.publish(ctx, job); err != nil {
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	slog.InfoContext(ctx, obs.EventQueuePublished, obs.AttrExecutionID, job.ExecutionID, "queue", p.queue)
	return nil
}

func (p *Publisher) publish(ctx context.Context, job Job) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if err := p.connect(); err != nil {
		return err
	}
	err := p.ch.PublishWithContext(ctx, "", p.queue, false, false, amqp.Publishing{
		DeliveryMode:  amqp.Persistent,
		ContentType:   "text/plain",
		Body:          []byte(strconv.FormatUint(uint64(job.ExecutionID), 10)),
		CorrelationId: job.CorrelationID,
		Headers:       amqp.Table{traceparentHeader: job.Traceparent},
	})
	if err != nil {
		p.disconnect()
		return err
	}

	select {
	case confirmation, ok := <-p.confirms:
		if !ok {
			p.disconnect()
			return ErrNotConfirmed
		}
		if !confirmation.Ack {
			return ErrNotConfirmed
		}
		return nil
	case <-ctx.Done():
		p.disconnect()
		return ctx.Err()
	}
}

func (p *Publisher) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.disconnect()
	return nil
}

func (p *Publisher) connect() error {
	if p.ch != nil && !p.ch.IsClosed() {
		return nil
	}
	p.disconnect()

	conn, err := dial(p.url)
	if err != nil {
		return err
	}
	ch, err := conn.Channel()
	if err != nil {
		conn.Close()
		return err
	}
	if err := declare(ch, p.queue); err != nil {
		conn.Close()
		return err
	}
	if err := ch.Confirm(false); err != nil {
		conn.Close()
		return err
	}
	p.conn, p.ch = conn, ch
	p.confirms = ch.NotifyPublish(make(chan amqp.Confirmation, 1))
	return nil
}

func (p *Publisher) disconnect() {
	if p.conn != nil {
		p.conn.Close()
	}
	p.conn, p.ch, p.confirms = nil, nil, nil
}
