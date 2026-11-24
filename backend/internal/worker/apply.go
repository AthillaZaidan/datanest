package worker

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/AthillaZaidan/datanest/backend/internal/migration"
	"github.com/AthillaZaidan/datanest/backend/internal/models"
)

func (w *Worker) limits() []string {
	var out []string
	if w.cfg.LockTimeout > 0 {
		out = append(out, fmt.Sprintf("SET LOCAL lock_timeout = %d", w.cfg.LockTimeout.Milliseconds()))
	}
	if w.cfg.StatementTimeout > 0 {
		out = append(out, fmt.Sprintf("SET LOCAL statement_timeout = %d", w.cfg.StatementTimeout.Milliseconds()))
	}
	return out
}

func (w *Worker) applyTx(ctx context.Context, db *sql.DB, e *models.MigrationExecution, batchNo int, from, to *string, query string, args ...any) (int64, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	if err := migration.EnsureExecutionLedger(ctx, tx); err != nil {
		return 0, err
	}
	for _, q := range w.limits() {
		if _, err := tx.ExecContext(ctx, q); err != nil {
			return 0, err
		}
	}

	txCtx, cancel := context.WithTimeout(ctx, w.cfg.ExecTimeout)
	defer cancel()
	res, err := tx.ExecContext(txCtx, query, args...)
	if err != nil {
		return 0, err
	}
	rows, _ := res.RowsAffected()
	entry := migration.LedgerEntry{ExecutionID: e.ID, BatchNo: batchNo, RangeFrom: from, RangeTo: to, RowCount: max(rows, 0)}
	if err := migration.RecordExecution(txCtx, tx, entry); err != nil {
		var pg *pgconn.PgError
		if errors.As(err, &pg) && pg.Code == "23505" && pg.TableName == migration.ExecutionLedgerTable {
			return 0, &unknownError{err: err}
		}
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
	if err := tx.Commit(); err != nil {
		return 0, &unknownError{err: err}
	}
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

func (w *Worker) runNormal(ctx context.Context, db *sql.DB, e *models.MigrationExecution) (int, int64, error) {
	entries, err := migration.LookupExecution(ctx, db, e.ID)
	if err != nil {
		return 0, 0, err
	}
	for _, entry := range entries {
		if entry.BatchNo == migration.NormalBatch {
			return 1, entry.RowCount, nil
		}
	}
	rows, err := w.applyTx(ctx, db, e, migration.NormalBatch, nil, nil, e.SQLExecuted)
	if err != nil {
		return 0, 0, err
	}
	return 1, rows, nil
}
