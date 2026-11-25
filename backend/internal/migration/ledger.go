// Package migration manages migration recipes and submits their executions; the worker runs them.
package migration

import (
	"context"
	"database/sql"
)

// LedgerTable is the version ledger the retired Milestone 1 execution wrote in each target's
// public schema. It is no longer written, but old targets still have it, so the explorer hides it.
const LedgerTable = "_platform_migrations"

const ExecutionLedgerTable = "_platform_executions"

const NormalBatch = 0

const createExecutionLedger = `CREATE TABLE IF NOT EXISTS public.` + ExecutionLedgerTable + ` (
	execution_id bigint NOT NULL,
	batch_no integer NOT NULL DEFAULT 0,
	range_from text,
	range_to text,
	row_count bigint NOT NULL DEFAULT 0,
	applied_at timestamptz NOT NULL DEFAULT now(),
	PRIMARY KEY (execution_id, batch_no)
)`

type LedgerEntry struct {
	ExecutionID uint
	BatchNo     int
	RangeFrom   *string
	RangeTo     *string
	RowCount    int64
}

type queryer interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func EnsureExecutionLedger(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, createExecutionLedger)
	return err
}

func RecordExecution(ctx context.Context, tx *sql.Tx, e LedgerEntry) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO public.`+ExecutionLedgerTable+` (execution_id, batch_no, range_from, range_to, row_count)
		VALUES ($1, $2, $3, $4, $5)`,
		e.ExecutionID, e.BatchNo, e.RangeFrom, e.RangeTo, e.RowCount)
	return err
}

func RecordRowCount(ctx context.Context, tx *sql.Tx, executionID uint, batchNo int, rows int64) error {
	_, err := tx.ExecContext(ctx, `
		UPDATE public.`+ExecutionLedgerTable+` SET row_count = $3 WHERE execution_id = $1 AND batch_no = $2`,
		executionID, batchNo, rows)
	return err
}

func LookupExecution(ctx context.Context, q queryer, executionID uint) ([]LedgerEntry, error) {
	var exists bool
	if err := q.QueryRowContext(ctx, `SELECT to_regclass('public.`+ExecutionLedgerTable+`') IS NOT NULL`).Scan(&exists); err != nil {
		return nil, err
	}
	if !exists {
		return nil, nil
	}
	rows, err := q.QueryContext(ctx, `
		SELECT execution_id, batch_no, range_from, range_to, row_count
		FROM public.`+ExecutionLedgerTable+` WHERE execution_id = $1 ORDER BY batch_no`, executionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []LedgerEntry
	for rows.Next() {
		var e LedgerEntry
		if err := rows.Scan(&e.ExecutionID, &e.BatchNo, &e.RangeFrom, &e.RangeTo, &e.RowCount); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
