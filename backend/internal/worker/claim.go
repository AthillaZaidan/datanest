package worker

import (
	"context"
	"errors"
	"log/slog"

	"github.com/AthillaZaidan/datanest/backend/internal/dblock"
	"github.com/AthillaZaidan/datanest/backend/internal/execution"
	"github.com/AthillaZaidan/datanest/backend/internal/models"
)

type ClaimResult int

const (
	ClaimNone ClaimResult = iota
	ClaimBusy
	ClaimOK
)

type Claimed struct {
	Exec  models.MigrationExecution
	Owner string
	Lease dblock.Lease
	From  models.ExecutionStatus
}

func (w *Worker) Claim(ctx context.Context, id uint) (*Claimed, ClaimResult, error) {
	e, err := w.store.Get(ctx, id)
	if errors.Is(err, execution.ErrNotFound) {
		return nil, ClaimNone, nil
	}
	if err != nil {
		return nil, ClaimNone, err
	}
	if e.Status != models.ExecQueued && e.Status != models.ExecUncertain {
		return nil, ClaimNone, nil
	}

	owner := w.newOwner()
	lease, ok, err := w.locks.Acquire(ctx, e.DatabaseID, e.ID, owner, w.cfg.LeaseTTL)
	if err != nil {
		return nil, ClaimNone, err
	}
	if !ok {
		if err := w.store.Postpone(ctx, e.ID, w.cfg.LockBusyDelay); err != nil {
			return nil, ClaimNone, err
		}
		slog.Info("lock.busy", "execution_id", e.ID, "database_id", e.DatabaseID)
		return nil, ClaimBusy, nil
	}

	claimed, err := w.store.Claim(ctx, e.ID, e.Status, owner)
	if err != nil || claimed == nil {
		w.locks.Release(context.WithoutCancel(ctx), lease)
		return nil, ClaimNone, err
	}
	slog.Info("execution.claimed", "execution_id", e.ID, "database_id", e.DatabaseID, "attempt", claimed.Attempts,
		"from", string(e.Status), "correlation_id", claimed.CorrelationID)
	return &Claimed{Exec: *claimed, Owner: owner, Lease: lease, From: e.Status}, ClaimOK, nil
}
