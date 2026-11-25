package worker

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/AthillaZaidan/datanest/backend/internal/migration"
	"github.com/AthillaZaidan/datanest/backend/internal/obs"
)

func sqlState(err error) string {
	var pg *pgconn.PgError
	if errors.As(err, &pg) {
		return pg.Code
	}
	return ""
}

func (w *Worker) limits() []string {
	out := []string{"SET LOCAL standard_conforming_strings = on"}
	if w.cfg.LockTimeout > 0 {
		out = append(out, fmt.Sprintf("SET LOCAL lock_timeout = %d", w.cfg.LockTimeout.Milliseconds()))
	}
	if w.cfg.StatementTimeout > 0 {
		out = append(out, fmt.Sprintf("SET LOCAL statement_timeout = %d", w.cfg.StatementTimeout.Milliseconds()))
	}
	return out
}

func (w *Worker) applyTx(ctx context.Context, db *sql.DB, cl *Claimed, batchNo int, from, to *string, query string, args ...any) (int64, error) {
	if err := w.begin(ctx, db, cl); err != nil {
		return 0, err
	}
	e := &cl.Exec
	if batchNo != migration.NormalBatch {
		var batchSpan trace.Span
		ctx, batchSpan = obs.StartBatch(ctx, batchNo, attribute.Int(obs.AttrBatchNo, batchNo))
		defer batchSpan.End()
	}
	base := logAttrs(e, obs.AttrAttempt, e.Attempts, obs.AttrBatchNo, batchNo)
	with := func(extra ...any) []any { return append(slices.Clone(base), extra...) }

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	// After Commit the outcome is the commit's own, so no sql.rollback is claimed.
	finished := false
	defer func() {
		if !finished {
			tx.Rollback()
			slog.InfoContext(ctx, obs.EventSQLRollback, with(obs.AttrStatus, "rolled_back")...)
		}
	}()

	for _, q := range w.limits() {
		if _, err := tx.ExecContext(ctx, q); err != nil {
			return 0, err
		}
	}

	txCtx, cancel := context.WithTimeout(ctx, w.cfg.ExecTimeout)
	defer cancel()
	entry := migration.LedgerEntry{ExecutionID: e.ID, BatchNo: batchNo, RangeFrom: from, RangeTo: to}
	if err := migration.RecordExecution(txCtx, tx, entry); err != nil {
		if ledgerConflict(err) {
			return 0, &unknownError{err: err}
		}
		return 0, err
	}
	cl.fenced = true
	owned, err := w.store.Owns(txCtx, e.ID, cl.Owner)
	if err != nil {
		return 0, err
	}
	if !owned {
		return 0, errLockLost
	}

	sqlCtx, sqlSpan := obs.Start(txCtx, obs.SpanSQLQuery)
	start := with(obs.AttrStatus, "started", obs.AttrQuery, query)
	if len(args) > 0 {
		start = append(start, obs.AttrParams, args)
	}
	slog.InfoContext(sqlCtx, obs.EventSQLStart, start...)
	started := time.Now()
	res, err := tx.ExecContext(sqlCtx, query, args...)
	if err != nil {
		sqlSpan.SetStatus(codes.Error, "sql failed")
		sqlSpan.End()
		slog.WarnContext(sqlCtx, obs.EventSQLResult, with(obs.AttrStatus, "error", obs.AttrDurationMS, time.Since(started).Milliseconds(),
			obs.AttrSQLState, sqlState(err), obs.AttrError, err.Error())...)
		return 0, err
	}
	rows, _ := res.RowsAffected()
	sqlSpan.End()
	slog.InfoContext(sqlCtx, obs.EventSQLResult, with(obs.AttrStatus, "ok", obs.AttrDurationMS, time.Since(started).Milliseconds(), obs.AttrRows, rows)...)
	if err := migration.RecordRowCount(txCtx, tx, e.ID, batchNo, max(rows, 0)); err != nil {
		return 0, err
	}

	if h := w.Hooks.BeforeCommit; h != nil {
		if herr := h(e.ID, batchNo); herr != nil {
			if errors.Is(herr, ErrInjectedCommitUnknown) {
				return 0, &unknownError{err: herr}
			}
			return 0, herr
		}
	}
	finished = true
	if err := tx.Commit(); err != nil {
		slog.WarnContext(ctx, obs.EventSQLCommit, with(obs.AttrStatus, "unknown", obs.AttrError, err.Error())...)
		return 0, &unknownError{err: err}
	}
	cl.fenced = false
	slog.InfoContext(ctx, obs.EventSQLCommit, with(obs.AttrStatus, "committed", obs.AttrRows, max(rows, 0))...)
	if h := w.Hooks.AfterCommit; h != nil {
		if herr := h(e.ID, batchNo); herr != nil {
			if errors.Is(herr, ErrInjectedCommitUnknown) {
				return 0, &unknownError{err: herr}
			}
			return 0, herr
		}
	}
	return max(rows, 0), nil
}

func (w *Worker) runNormal(ctx context.Context, db *sql.DB, cl *Claimed) (int, int64, error) {
	rows, err := w.applyTx(ctx, db, cl, migration.NormalBatch, nil, nil, cl.Exec.SQLExecuted)
	if err != nil {
		return 0, 0, err
	}
	return 1, rows, nil
}
