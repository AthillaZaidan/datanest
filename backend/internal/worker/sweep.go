package worker

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/AthillaZaidan/datanest/backend/internal/models"
	"github.com/AthillaZaidan/datanest/backend/internal/queue"
)

type SweepResult struct {
	Dead      int
	Published int
}

func (w *Worker) Dispatch(ctx context.Context, e *models.MigrationExecution) error {
	if w.Publisher == nil {
		return errors.New("worker: no publisher configured")
	}
	job := queue.Job{ExecutionID: e.ID, Traceparent: e.Traceparent, CorrelationID: e.CorrelationID}
	if err := w.Publisher.Publish(ctx, job); err != nil {
		return err
	}
	return w.store.MarkPublished(ctx, e.ID)
}

func (w *Worker) Sweep(ctx context.Context) (SweepResult, error) {
	var res SweepResult
	dead, err := w.store.MarkDeadRunning(ctx)
	if err != nil {
		return res, err
	}
	for _, id := range dead {
		slog.Warn("execution.worker_lost", "execution_id", id)
	}
	res.Dead = len(dead)

	due, err := w.store.DueForPublish(ctx, w.cfg.RepublishAfter, 100)
	if err != nil {
		return res, err
	}
	for i := range due {
		if err := w.Dispatch(ctx, &due[i]); err != nil {
			slog.Warn("queue.publish_failed", "execution_id", due[i].ID, "error", err)
			continue
		}
		res.Published++
	}
	return res, nil
}

func (w *Worker) RunSweeper(ctx context.Context, every time.Duration) {
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for {
		if _, err := w.Sweep(ctx); err != nil && ctx.Err() == nil {
			slog.Error("sweeper.failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
