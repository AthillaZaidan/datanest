//go:build integration

package queue

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

const wait = 10 * time.Second

func brokerURL(t *testing.T) string {
	t.Helper()
	url := os.Getenv("TEST_AMQP_URL")
	if url == "" {
		t.Skip("TEST_AMQP_URL not set; start rabbitmq with docker compose and see .env.example")
	}
	return url
}

func freshQueue(t *testing.T, url string) string {
	t.Helper()
	name := fmt.Sprintf("test.jobs.%d", time.Now().UnixNano())
	t.Cleanup(func() {
		conn, err := amqp.Dial(url)
		if err != nil {
			return
		}
		defer conn.Close()
		if ch, err := conn.Channel(); err == nil {
			ch.QueueDelete(name, false, false, false)
		}
	})
	return name
}

func startConsumer(t *testing.T, c *Consumer, handle func(context.Context, Message)) (stop func()) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		c.Run(ctx, handle)
		close(done)
	}()
	stop = func() {
		cancel()
		<-done
	}
	t.Cleanup(stop)
	return stop
}

func receive(t *testing.T, ch <-chan Message) Message {
	t.Helper()
	select {
	case m := <-ch:
		return m
	case <-time.After(wait):
		t.Fatal("no message arrived")
		return Message{}
	}
}

func expectNothing(t *testing.T, ch <-chan Message, within time.Duration) {
	t.Helper()
	select {
	case m := <-ch:
		t.Fatalf("unexpected message for execution %d", m.ExecutionID)
	case <-time.After(within):
	}
}

func ackAndForward(out chan<- Message) func(context.Context, Message) {
	return func(_ context.Context, m Message) {
		m.Ack()
		out <- m
	}
}

func publish(t *testing.T, p *Publisher, executionID uint, traceparent string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), wait)
	defer cancel()
	if err := p.Publish(ctx, executionID, traceparent); err != nil {
		t.Fatalf("publish %d: %v", executionID, err)
	}
}

func TestPublishThenConsumeCarriesTheIDAndTraceparent(t *testing.T) {
	url := brokerURL(t)
	queue := freshQueue(t, url)
	p := NewPublisher(url, queue)
	defer p.Close()
	got := make(chan Message, 4)
	startConsumer(t, NewConsumer(url, queue, 1), ackAndForward(got))

	publish(t, p, 57, "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01")

	m := receive(t, got)
	if m.ExecutionID != 57 || m.Traceparent != "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01" {
		t.Errorf("message = %+v", m)
	}
}

func TestADuplicateMessageIsDeliveredTwice(t *testing.T) {
	url := brokerURL(t)
	queue := freshQueue(t, url)
	p := NewPublisher(url, queue)
	defer p.Close()
	got := make(chan Message, 4)
	startConsumer(t, NewConsumer(url, queue, 1), ackAndForward(got))

	publish(t, p, 57, "")
	publish(t, p, 57, "")

	if a, b := receive(t, got), receive(t, got); a.ExecutionID != 57 || b.ExecutionID != 57 {
		t.Errorf("got %d and %d, want 57 twice", a.ExecutionID, b.ExecutionID)
	}
}

func TestAnUnackedMessageComesBackWhenTheConsumerStops(t *testing.T) {
	url := brokerURL(t)
	queue := freshQueue(t, url)
	p := NewPublisher(url, queue)
	defer p.Close()

	first := make(chan Message, 1)
	stopFirst := startConsumer(t, NewConsumer(url, queue, 1), func(_ context.Context, m Message) { first <- m })
	publish(t, p, 57, "")
	receive(t, first)
	stopFirst()

	second := make(chan Message, 1)
	startConsumer(t, NewConsumer(url, queue, 1), ackAndForward(second))
	m := receive(t, second)
	if m.ExecutionID != 57 || !m.delivery.Redelivered {
		t.Errorf("message = %d redelivered=%v, want 57 redelivered", m.ExecutionID, m.delivery.Redelivered)
	}
}

func TestAnAckedMessageDoesNotComeBack(t *testing.T) {
	url := brokerURL(t)
	queue := freshQueue(t, url)
	p := NewPublisher(url, queue)
	defer p.Close()

	first := make(chan Message, 1)
	stopFirst := startConsumer(t, NewConsumer(url, queue, 1), ackAndForward(first))
	publish(t, p, 57, "")
	receive(t, first)
	stopFirst()

	second := make(chan Message, 1)
	startConsumer(t, NewConsumer(url, queue, 1), ackAndForward(second))
	expectNothing(t, second, time.Second)
}

func TestRequeuePutsTheMessageBack(t *testing.T) {
	url := brokerURL(t)
	queue := freshQueue(t, url)
	p := NewPublisher(url, queue)
	defer p.Close()

	got := make(chan Message, 4)
	var once sync.Once
	startConsumer(t, NewConsumer(url, queue, 1), func(_ context.Context, m Message) {
		once.Do(func() {
			m.Requeue()
			got <- m
		})
		if m.delivery.Redelivered {
			m.Ack()
			got <- m
		}
	})
	publish(t, p, 57, "")

	receive(t, got)
	if m := receive(t, got); !m.delivery.Redelivered {
		t.Error("the requeued message should come back marked as redelivered")
	}
}

func TestPrefetchLimitsHowManyMessagesAreHeldUnacked(t *testing.T) {
	url := brokerURL(t)
	queue := freshQueue(t, url)
	p := NewPublisher(url, queue)
	defer p.Close()

	got := make(chan Message, 4)
	startConsumer(t, NewConsumer(url, queue, 1), func(_ context.Context, m Message) { got <- m })
	publish(t, p, 1, "")
	publish(t, p, 2, "")

	first := receive(t, got)
	expectNothing(t, got, time.Second)
	first.Ack()
	if second := receive(t, got); second.ExecutionID != 2 {
		t.Errorf("second message = %d, want 2", second.ExecutionID)
	}
}

func TestAMalformedBodyIsDroppedAndTheQueueKeepsFlowing(t *testing.T) {
	url := brokerURL(t)
	queue := freshQueue(t, url)
	p := NewPublisher(url, queue)
	defer p.Close()
	got := make(chan Message, 4)
	stop := startConsumer(t, NewConsumer(url, queue, 1), ackAndForward(got))

	conn, err := amqp.Dial(url)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	ch, err := conn.Channel()
	if err != nil {
		t.Fatal(err)
	}
	if err := declare(ch, queue); err != nil {
		t.Fatal(err)
	}
	err = ch.PublishWithContext(context.Background(), "", queue, false, false, amqp.Publishing{
		DeliveryMode: amqp.Persistent,
		Body:         []byte("not-a-number"),
	})
	if err != nil {
		t.Fatal(err)
	}
	publish(t, p, 58, "")

	if m := receive(t, got); m.ExecutionID != 58 {
		t.Errorf("message = %d, want 58", m.ExecutionID)
	}
	expectNothing(t, got, time.Second)

	stop()
	if n := readyMessages(t, ch, queue); n != 0 {
		t.Errorf("%d message(s) left in the queue; the malformed one must be dropped, not requeued", n)
	}
}

func readyMessages(t *testing.T, ch *amqp.Channel, queue string) int {
	t.Helper()
	q, err := ch.QueueDeclarePassive(queue, true, false, false, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	return q.Messages
}

func TestTheConsumerReconnectsAfterTheConnectionDrops(t *testing.T) {
	url := brokerURL(t)
	queue := freshQueue(t, url)
	p := NewPublisher(url, queue)
	defer p.Close()

	var mu sync.Mutex
	var latest *amqp.Connection
	c := NewConsumer(url, queue, 1)
	c.dial = func(u string) (*amqp.Connection, error) {
		conn, err := dial(u)
		mu.Lock()
		latest = conn
		mu.Unlock()
		return conn, err
	}
	got := make(chan Message, 4)
	startConsumer(t, c, ackAndForward(got))

	publish(t, p, 1, "")
	receive(t, got)

	mu.Lock()
	latest.Close()
	mu.Unlock()
	publish(t, p, 2, "")

	if m := receive(t, got); m.ExecutionID != 2 {
		t.Errorf("message after reconnect = %d, want 2", m.ExecutionID)
	}
}

func TestThePublisherRecoversAfterItsConnectionDrops(t *testing.T) {
	url := brokerURL(t)
	queue := freshQueue(t, url)
	p := NewPublisher(url, queue)
	defer p.Close()
	got := make(chan Message, 4)
	startConsumer(t, NewConsumer(url, queue, 1), ackAndForward(got))

	publish(t, p, 1, "")
	receive(t, got)

	p.mu.Lock()
	p.conn.Close()
	p.mu.Unlock()
	publish(t, p, 2, "")

	if m := receive(t, got); m.ExecutionID != 2 {
		t.Errorf("message after publisher reconnect = %d, want 2", m.ExecutionID)
	}
}
