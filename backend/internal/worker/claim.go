package worker

import (
	"context"
	"errors"
	"log/slog"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/AthillaZaidan/datanest/backend/internal/dblock"
	"github.com/AthillaZaidan/datanest/backend/internal/execution"
	"github.com/AthillaZaidan/datanest/backend/internal/models"
	"github.com/AthillaZaidan/datanest/backend/internal/obs"
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

	span trace.Span
}

func (cl *Claimed) bind(ctx context.Context) context.Context {
	ctx = obs.WithCorrelationID(ctx, cl.Exec.CorrelationID)
	if cl.span != nil {
		ctx = trace.ContextWithSpan(ctx, cl.span)
	}
	return ctx
}

func (cl *Claimed) end() {
	if cl.span != nil {
		cl.span.End()
	}
}

func (w *Worker) Claim(ctx context.Context, id uint) (*Claimed, ClaimResult, error) {
	return w.ClaimWith(ctx, id, "")
}

// ClaimWith continues the trace of the message, or of the execution when the message has none.
func (w *Worker) ClaimWith(ctx context.Context, id uint, traceparent string) (*Claimed, ClaimResult, error) {
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

	// Join the trace of the request that queued the execution.
	if traceparent == "" {
		traceparent = e.Traceparent
	}
	ctx = obs.WithTraceparent(obs.WithCorrelationID(ctx, e.CorrelationID), traceparent)
	ctx, sp := obs.Start(ctx, obs.SpanWorkerProcess,
		attribute.Int(obs.AttrExecutionID, int(e.ID)), attribute.Int(obs.AttrDatabaseID, int(e.DatabaseID)))
	handedOver := false
	defer func() {
		if !handedOver {
			sp.End()
		}
	}()

	owner := w.newOwner()
	lockCtx, lockSpan := obs.Start(ctx, obs.SpanLockAcquire)
	lease, ok, err := w.locks.Acquire(lockCtx, e.DatabaseID, e.ID, owner, w.cfg.LeaseTTL)
	lockSpan.End()
	if err != nil {
		return nil, ClaimNone, err
	}
	if !ok {
		if err := w.store.Postpone(ctx, e.ID, w.cfg.LockBusyDelay); err != nil {
			return nil, ClaimNone, err
		}
		slog.InfoContext(ctx, "lock.busy", jobAttrs(e)...)
		return nil, ClaimBusy, nil
	}
	slog.InfoContext(ctx, obs.EventLockAcquired, logAttrs(e, "owner", owner, "lock_token", lease.Token)...)

	claimed, err := w.store.Claim(ctx, e.ID, e.Status, owner)
	if err != nil || claimed == nil {
		w.locks.Release(context.WithoutCancel(ctx), lease)
		return nil, ClaimNone, err
	}
	slog.InfoContext(ctx, "execution.claimed", logAttrs(e, obs.AttrAttempt, claimed.Attempts, "from", string(e.Status))...)
	handedOver = true
	return &Claimed{Exec: *claimed, Owner: owner, Lease: lease, From: e.Status, span: sp}, ClaimOK, nil
}
