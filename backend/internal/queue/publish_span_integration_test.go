//go:build integration

package queue

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/AthillaZaidan/datanest/backend/internal/obs"
)

func recordSpans(t *testing.T) *tracetest.SpanRecorder {
	t.Helper()
	if _, err := obs.SetupTracing(nil); err != nil {
		t.Fatal(err)
	}
	rec := tracetest.NewSpanRecorder()
	prev := otel.GetTracerProvider()
	otel.SetTracerProvider(sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec)))
	t.Cleanup(func() { otel.SetTracerProvider(prev) })
	return rec
}

func TestTheMessageHeaderCarriesThePublishSpan(t *testing.T) {
	rec := recordSpans(t)
	url := brokerURL(t)
	name := freshQueue(t, url)
	p := NewPublisher(url, name)
	defer p.Close()
	got := make(chan Message, 1)
	startConsumer(t, NewConsumer(url, name, 1), ackAndForward(got))

	parentCtx, parent := obs.Start(context.Background(), "api.approve")
	stale := "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01"
	if err := p.Publish(parentCtx, Job{ExecutionID: 9, Traceparent: stale, CorrelationID: "corr-9"}); err != nil {
		t.Fatal(err)
	}
	parent.End()

	m := receive(t, got)
	var publish sdktrace.ReadOnlySpan
	for _, s := range rec.Ended() {
		if s.Name() == obs.SpanQueuePublish {
			publish = s
		}
	}
	if publish == nil {
		t.Fatal("no queue.publish span")
	}
	ctx := obs.WithTraceparent(context.Background(), m.Traceparent)
	_, child := obs.Start(ctx, "worker.process")
	child.End()
	var worker sdktrace.ReadOnlySpan
	for _, s := range rec.Ended() {
		if s.Name() == "worker.process" {
			worker = s
		}
	}
	if m.Traceparent == stale || worker == nil || worker.Parent().SpanID() != publish.SpanContext().SpanID() {
		t.Errorf("header %q: the worker is not a child of the queue.publish span", m.Traceparent)
	}
	if publish.Parent().SpanID() != parent.SpanContext().SpanID() {
		t.Error("queue.publish is not a child of the span that published")
	}
}

func TestWithTracingOffTheStoredTraceparentIsKept(t *testing.T) {
	otel.SetTracerProvider(noop.NewTracerProvider())
	url := brokerURL(t)
	name := freshQueue(t, url)
	p := NewPublisher(url, name)
	defer p.Close()
	got := make(chan Message, 1)
	startConsumer(t, NewConsumer(url, name, 1), ackAndForward(got))

	stored := "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01"
	if err := p.Publish(context.Background(), Job{ExecutionID: 10, Traceparent: stored}); err != nil {
		t.Fatal(err)
	}
	if m := receive(t, got); m.Traceparent != stored {
		t.Errorf("traceparent = %q, want the stored one", m.Traceparent)
	}
}
