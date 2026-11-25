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

func ledgerConflict(err error) bool {
	var pg *pgconn.PgError
	return errors.As(err, &pg) && pg.Code == "23505" && pg.TableName == migration.ExecutionLedgerTable
}

func (w *Worker) ensureLedger(ctx context.Context, db *sql.DB) error {
	create := func() error {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		for _, q := range w.limits() {
			if _, err := tx.ExecContext(ctx, q); err != nil {
				return err
			}
		}
		if err := migration.EnsureExecutionLedger(ctx, tx); err != nil {
			return err
		}
		return tx.Commit()
	}
	err := create()
	if sqlState(err) != "23505" && sqlState(err) != "42P07" {
		return err
	}
	if err := create(); err != nil {
		return fmt.Errorf("ledger: %v", err)
	}
	return nil
}

func (w *Worker) begin(ctx context.Context, db *sql.DB, cl *Claimed) error {
	if cl.began {
		return nil
	}
	if err := w.ensureLedger(ctx, db); err != nil {
		return err
	}
	runs, ok, err := w.store.BeginSQL(context.WithoutCancel(ctx), cl.Exec.ID, cl.Owner)
	if err != nil {
		return err
	}
	if !ok {
		return errLockLost
	}
	if runs > w.cfg.MaxAttempts {
		return permanent("RETRY_EXHAUSTED", "Dihentikan setelah %d kali menjalankan SQL tanpa selesai.", w.cfg.MaxAttempts)
	}
	cl.began = true
	return nil
}

func (w *Worker) settleVerdict(ctx context.Context, db *sql.DB, cl *Claimed, batches int, err error) error {
	if err == nil || cl.From != models.ExecUncertain || cl.fenced || classify(err) != kindPermanent {
		return err
	}
	next := migration.NormalBatch
	if cl.Exec.Mode == models.ExecModeBatch {
		next = batches + 1
	}
	if perr := w.probe(ctx, db, cl.Exec.ID, next); perr != nil {
		return perr
	}
	return err
}

func (w *Worker) probe(ctx context.Context, db *sql.DB, executionID uint, batchNo int) error {
	if err := w.ensureLedger(ctx, db); err != nil {
		return fmt.Errorf("ledger: %v", err)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, q := range w.limits() {
		if _, err := tx.ExecContext(ctx, q); err != nil {
			return err
		}
	}
	err = migration.RecordExecution(ctx, tx, migration.LedgerEntry{ExecutionID: executionID, BatchNo: batchNo})
	switch {
	case err == nil:
		return nil
	case ledgerConflict(err):
		return &unknownError{err: err}
	case classify(err) == kindPermanent:
		return fmt.Errorf("ledger: %v", err)
	}
	return err
}
