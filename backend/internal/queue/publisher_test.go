package queue

import (
	"context"
	"testing"
	"time"
)

func TestPublishToAnUnreachableBrokerFails(t *testing.T) {
	p := NewPublisher("amqp://guest:guest@127.0.0.1:1/", "unreachable")
	defer p.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := p.Publish(ctx, 1, ""); err == nil {
		t.Fatal("publish to an unreachable broker should fail")
	}
}

func TestParseRejectsABodyThatIsNotAnExecutionID(t *testing.T) {
	for _, body := range []string{"", "abc", "-5", "1.5", "12 "} {
		if _, err := parse(deliveryWithBody(body)); err == nil {
			t.Errorf("body %q was accepted", body)
		}
	}
	msg, err := parse(deliveryWithBody("57"))
	if err != nil || msg.ExecutionID != 57 {
		t.Errorf("parse(57) = %+v, %v", msg, err)
	}
}
