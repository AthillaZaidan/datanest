package worker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/AthillaZaidan/datanest/backend/internal/migration"
	"github.com/AthillaZaidan/datanest/backend/internal/models"
	"github.com/AthillaZaidan/datanest/backend/internal/obs"
)

func (w *Worker) Run(ctx context.Context, cl *Claimed) error {
	ctx = cl.bind(ctx)
	defer cl.end()
	runCtx, cancel := context.WithCancelCause(ctx)
	heartbeat := w.startHeartbeat(runCtx, cancel, cl)

	batches, rows, err := w.execute(runCtx, cl)
	if err != nil && cl.span != nil {
		cl.span.RecordError(err)
		cl.span.SetStatus(codes.Error, err.Error())
	}

	lost := errors.Is(context.Cause(runCtx), errLockLost) || errors.Is(err, errLockLost)
	cancel(nil)
	<-heartbeat

	if errors.Is(err, ErrCrash) {
		return err
	}
	if lost && err != nil {
		slog.WarnContext(ctx, obs.EventLockLost, logAttrs(&cl.Exec, "worker", cl.Owner)...)
		return errLockLost
	}

	ferr := w.finalize(ctx, cl, batches, rows, err)
	if errors.Is(ferr, ErrCrash) {
		return ferr
	}
	w.locks.Release(context.WithoutCancel(ctx), cl.Lease)
	return ferr
}

func (w *Worker) execute(ctx context.Context, cl *Claimed) (batches int, rows int64, err error) {
	ctx, sp := obs.Start(ctx, obs.SpanExecute, attribute.String("kind", string(cl.Exec.Kind)), attribute.String("mode", string(cl.Exec.Mode)))
	defer func() {
		if err != nil {
			sp.RecordError(err)
			sp.SetStatus(codes.Error, err.Error())
		}
		sp.End()
	}()
	e := &cl.Exec
	if err := w.store.StartAttempt(context.WithoutCancel(ctx), e, cl.Owner); err != nil {
		return e.CommittedBatches, e.CommittedRows, err
	}
	db, err := w.targets.Get(ctx, e.DatabaseID)
	if err != nil {
		w.logRecoveryCheck(ctx, cl, "unreachable", 0, err)
		return e.CommittedBatches, e.CommittedRows, fmt.Errorf("target: %w", err)
	}
	own, err := migration.LookupExecution(ctx, db, e.ID)
	if err != nil {
		w.logRecoveryCheck(ctx, cl, "unreadable", 0, err)
		if cl.From == models.ExecUncertain && classify(err) == kindPermanent {
			err = fmt.Errorf("ledger: %v", err)
		}
		return e.CommittedBatches, e.CommittedRows, err
	}
	if len(own) > 0 {
		w.logRecoveryCheck(ctx, cl, "evidence", len(own), nil)
	} else {
		w.logRecoveryCheck(ctx, cl, "no_evidence", 0, nil)
	}
	if e.Mode != models.ExecModeBatch && len(own) > 0 {
		return 1, own[0].RowCount, nil
	}
	batches, rows = totals(own)
	if err := w.revalidate(ctx, e); err != nil {
		return batches, rows, w.settleVerdict(ctx, db, cl, batches, err)
	}
	switch {
	case e.Mode == models.ExecModeBatch && e.Kind == models.ExecKindRollback:
		batches, rows, err = w.runUndo(ctx, db, cl, own)
	case e.Mode == models.ExecModeBatch:
		batches, rows, err = w.runBatches(ctx, db, cl, own)
	default:
		batches, rows, err = w.runNormal(ctx, db, cl)
	}
	return batches, rows, w.settleVerdict(ctx, db, cl, batches, err)
}

func (w *Worker) logRecoveryCheck(ctx context.Context, cl *Claimed, result string, entries int, err error) {
	if cl.From != models.ExecUncertain {
		return
	}
	attrs := logAttrs(&cl.Exec, "result", result, "ledger_entries", entries)
	if err != nil {
		slog.WarnContext(ctx, obs.EventRecoveryCheck, append(attrs, obs.AttrError, err.Error())...)
		return
	}
	slog.InfoContext(ctx, obs.EventRecoveryCheck, attrs...)
}

func (w *Worker) revalidate(ctx context.Context, e *models.MigrationExecution) error {
	if w.Revalidate == nil {
		return nil
	}
	err := w.Revalidate(ctx, e)
	if err == nil || unavailable(err) {
		return err
	}
	return permanent("REVALIDATION_FAILED", "%s", err.Error())
}

func (w *Worker) startHeartbeat(ctx context.Context, cancel context.CancelCauseFunc, cl *Claimed) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(w.cfg.HeartbeatEvery)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if h := w.Hooks.BeforeHeartbeat; h != nil {
					if err := h(cl.Exec.ID); errors.Is(err, ErrCrash) {
						return
					}
				}
				ok, err := w.locks.Renew(ctx, cl.Lease, w.cfg.LeaseTTL)
				if err == nil && !ok {
					cancel(errLockLost)
					return
				}
			}
		}
	}()
	return done
}

func (w *Worker) finalize(ctx context.Context, cl *Claimed, batches int, rows int64, runErr error) error {
	wctx := context.WithoutCancel(ctx)
	e := &cl.Exec
	attempt := func(status models.AttemptStatus, code, message string) {
		if err := w.store.FinishAttempt(wctx, e.ID, e.Attempts, status, code, message); err != nil {
			slog.ErrorContext(ctx, "attempt.finish_failed", logAttrs(e, obs.AttrError, err)...)
		}
	}
	logIgnored := func(ok bool, err error, what string) error {
		if err != nil {
			return err
		}
		if !ok {
			slog.WarnContext(ctx, "execution.write_ignored", logAttrs(e, "what", what, "worker", cl.Owner)...)
		}
		return nil
	}

	if runErr == nil {
		if h := w.Hooks.BeforeResult; h != nil {
			if err := h(e.ID); errors.Is(err, ErrCrash) {
				return err
			}
		}
		ok, err := w.store.Finish(wctx, e.ID, cl.Owner, models.ExecSuccess, "", "", batches, rows)
		attempt(models.AttemptSuccess, "", "")
		slog.InfoContext(ctx, "execution.finished", logAttrs(e, obs.AttrStatus, "success", "batches", batches, obs.AttrRows, rows)...)
		return logIgnored(ok, err, "success")
	}

	code, message := describe(runErr)
	slog.WarnContext(ctx, "execution.error", logAttrs(e, obs.AttrAttempt, e.Attempts, "code", code, obs.AttrError, runErr)...)
	switch classify(runErr) {
	case kindUnknown:
		attempt(models.AttemptUncertain, code, message)
		ok, err := w.store.MarkUncertain(wctx, e.ID, cl.Owner, 0, code, message)
		slog.WarnContext(ctx, "execution.uncertain", logAttrs(e, obs.AttrStatus, "uncertain", "code", code)...)
		return logIgnored(ok, err, "uncertain")

	case kindTransient:
		if cl.From == models.ExecUncertain {
			attempt(models.AttemptUncertain, code, message)
			ok, err := w.store.MarkUncertain(wctx, e.ID, cl.Owner, w.cfg.UncertainRetry, code, message)
			slog.WarnContext(ctx, "execution.still_uncertain", logAttrs(e, obs.AttrStatus, "uncertain", "code", code)...)
			return logIgnored(ok, err, "still uncertain")
		}
		if e.Attempts < w.cfg.MaxAttempts {
			attempt(models.AttemptFailed, code, message)
			delay := withJitter(w.retryDelay(e.Attempts))
			ok, err := w.store.Requeue(wctx, e.ID, cl.Owner, delay, code, message)
			slog.WarnContext(ctx, "execution.retry", logAttrs(e, obs.AttrAttempt, e.Attempts, "retry_in", delay, "code", code)...)
			return logIgnored(ok, err, "requeue")
		}
		message = fmt.Sprintf("Gagal setelah %d percobaan. %s", e.Attempts, message)
	}

	status := models.ExecFailed
	if batches > 0 {
		status = models.ExecPartial
	}
	attempt(models.AttemptFailed, code, message)
	ok, err := w.store.Finish(wctx, e.ID, cl.Owner, status, code, message, batches, rows)
	slog.WarnContext(ctx, "execution.finished", logAttrs(e, obs.AttrStatus, string(status), "code", code, "batches", batches)...)
	return logIgnored(ok, err, string(status))
}
