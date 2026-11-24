//go:build integration

package migration_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/AthillaZaidan/datanest/backend/internal/migration"
	"github.com/AthillaZaidan/datanest/backend/internal/testutil"
)

func strp(s string) *string { return &s }

func targetWithItems(t *testing.T) *sql.DB {
	t.Helper()
	db := testutil.TargetDB(t)
	if _, err := db.Exec(`CREATE TABLE items (id int PRIMARY KEY, qty int NOT NULL);
		INSERT INTO items SELECT g, 0 FROM generate_series(1, 10) g`); err != nil {
		t.Fatal(err)
	}
	return db
}

func applyBatch(ctx context.Context, db *sql.DB, entry migration.LedgerEntry, bump int) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := migration.EnsureExecutionLedger(ctx, tx); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE items SET qty = qty + $1`, bump); err != nil {
		return err
	}
	if err := migration.RecordExecution(ctx, tx, entry); err != nil {
		return err
	}
	return tx.Commit()
}

func qty(t *testing.T, db *sql.DB) int {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT qty FROM items WHERE id = 1`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestLedgerRecordThenLookup(t *testing.T) {
	db := targetWithItems(t)
	ctx := context.Background()
	entry := migration.LedgerEntry{ExecutionID: 57, BatchNo: 3, RangeFrom: strp("20000"), RangeTo: strp("30000"), RowCount: 10000}
	if err := applyBatch(ctx, db, entry, 1); err != nil {
		t.Fatal(err)
	}

	got, err := migration.LookupExecution(ctx, db, 57)
	if err != nil || len(got) != 1 {
		t.Fatalf("lookup = %+v, err %v", got, err)
	}
	e := got[0]
	if e.ExecutionID != 57 || e.BatchNo != 3 || *e.RangeFrom != "20000" || *e.RangeTo != "30000" || e.RowCount != 10000 {
		t.Errorf("entry = %+v", e)
	}
}

func TestLedgerRejectsTheSameBatchTwiceAndUndoesItsData(t *testing.T) {
	db := targetWithItems(t)
	ctx := context.Background()
	entry := migration.LedgerEntry{ExecutionID: 57, BatchNo: 1, RowCount: 10}

	if err := applyBatch(ctx, db, entry, 1); err != nil {
		t.Fatal(err)
	}
	err := applyBatch(ctx, db, entry, 1)
	var pgErr *pgconn.PgError
	if err == nil || !errors.As(err, &pgErr) || pgErr.Code != "23505" {
		t.Fatalf("duplicate batch error = %v, want unique violation 23505", err)
	}
	if got := qty(t, db); got != 1 {
		t.Errorf("qty = %d, want 1: the duplicate's data change must be rolled back with it", got)
	}
}

func TestLedgerRolledBackTransactionLeavesNoRow(t *testing.T) {
	db := targetWithItems(t)
	ctx := context.Background()

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := migration.EnsureExecutionLedger(ctx, tx); err != nil {
		t.Fatal(err)
	}
	if err := migration.RecordExecution(ctx, tx, migration.LedgerEntry{ExecutionID: 9, BatchNo: 1}); err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}

	got, err := migration.LookupExecution(ctx, db, 9)
	if err != nil || len(got) != 0 {
		t.Fatalf("lookup after rollback = %+v, err %v; want nothing", got, err)
	}
}

func TestLedgerLookupWithoutTheTableIsNoEvidence(t *testing.T) {
	db := testutil.TargetDB(t)
	got, err := migration.LookupExecution(context.Background(), db, 1)
	if err != nil || got != nil {
		t.Fatalf("lookup = %+v, err %v; want nil without error", got, err)
	}
}

func TestLedgerBatchesComeBackInBatchOrder(t *testing.T) {
	db := targetWithItems(t)
	ctx := context.Background()
	for _, n := range []int{10, 2, 1} {
		if err := applyBatch(ctx, db, migration.LedgerEntry{ExecutionID: 5, BatchNo: n, RowCount: 1}, 0); err != nil {
			t.Fatal(err)
		}
	}
	if err := applyBatch(ctx, db, migration.LedgerEntry{ExecutionID: 6, BatchNo: migration.NormalBatch, RowCount: 1}, 0); err != nil {
		t.Fatal(err)
	}

	got, err := migration.LookupExecution(ctx, db, 5)
	if err != nil || len(got) != 3 {
		t.Fatalf("lookup = %+v, err %v", got, err)
	}
	if got[0].BatchNo != 1 || got[1].BatchNo != 2 || got[2].BatchNo != 10 {
		t.Errorf("order = %d, %d, %d; want 1, 2, 10 (numeric, not text)", got[0].BatchNo, got[1].BatchNo, got[2].BatchNo)
	}
	other, _ := migration.LookupExecution(ctx, db, 6)
	if len(other) != 1 || other[0].BatchNo != 0 {
		t.Errorf("execution 6 = %+v, want one normal-mode row with batch 0", other)
	}
}
