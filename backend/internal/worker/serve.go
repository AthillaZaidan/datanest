package worker

import (
	"context"
	"log/slog"
	"time"

	"github.com/AthillaZaidan/datanest/backend/internal/queue"
)

func (w *Worker) Handle(ctx context.Context, m queue.Message) {
	select {
	case w.slots <- struct{}{}:
	case <-ctx.Done():
		m.Requeue()
		return
	}

	claimed, result, err := w.ClaimWith(ctx, m.ExecutionID, m.Traceparent)
	switch {
	case err != nil:
		slog.Error("execution.claim_failed", "execution_id", m.ExecutionID, "error", err)
		<-w.slots
		select {
		case <-time.After(time.Second):
		case <-ctx.Done():
		}
		m.Requeue()
	case result == ClaimOK:
		m.Ack()
		w.running.Add(1)
		go func() {
			defer w.running.Done()
			defer func() { <-w.slots }()
			w.Run(ctx, claimed)
		}()
	default:
		m.Ack()
		<-w.slots
	}
}

func (w *Worker) Serve(ctx context.Context, amqpURL string, sweepEvery time.Duration) error {
	consumer := queue.NewConsumer(amqpURL, w.cfg.Queue, w.cfg.Concurrency)
	go w.RunSweeper(ctx, sweepEvery)
	err := consumer.Run(ctx, w.Handle)
	w.Wait()
	return err
}
