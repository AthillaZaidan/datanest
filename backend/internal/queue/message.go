package queue

import (
	"fmt"
	"strconv"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

const JobsQueue = "migration.jobs"

const traceparentHeader = "traceparent"

const dialTimeout = 5 * time.Second

type Job struct {
	ExecutionID   uint
	Traceparent   string
	CorrelationID string
}

type Message struct {
	Job
	delivery amqp.Delivery
}

func (m Message) Ack() error { return m.delivery.Ack(false) }

func (m Message) Requeue() error { return m.delivery.Nack(false, true) }

func dial(url string) (*amqp.Connection, error) {
	return amqp.DialConfig(url, amqp.Config{
		Dial:      amqp.DefaultDial(dialTimeout),
		Heartbeat: 10 * time.Second,
	})
}

func declare(ch *amqp.Channel, queue string) error {
	_, err := ch.QueueDeclare(queue, true, false, false, false, nil)
	return err
}

func parse(d amqp.Delivery) (Message, error) {
	id, err := strconv.ParseUint(string(d.Body), 10, strconv.IntSize)
	if err != nil {
		return Message{}, fmt.Errorf("body %q is not an execution id: %w", d.Body, err)
	}
	traceparent, _ := d.Headers[traceparentHeader].(string)
	return Message{Job: Job{ExecutionID: uint(id), Traceparent: traceparent, CorrelationID: d.CorrelationId}, delivery: d}, nil
}
