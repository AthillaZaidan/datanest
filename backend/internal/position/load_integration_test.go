//go:build integration

package position_test

import (
	"context"
	"testing"

	"github.com/AthillaZaidan/datanest/backend/internal/models"
	"github.com/AthillaZaidan/datanest/backend/internal/position"
	"github.com/AthillaZaidan/datanest/backend/internal/testutil"
)

func TestLoadDerivesFromStoredExecutions(t *testing.T) {
	db := testutil.PlatformDB(t)
	ctx := context.Background()
	conn := testutil.CreateDatabaseConnection(t, db, "toko")
	other := testutil.CreateDatabaseConnection(t, db, "kampus")

	migration := func(databaseID uint, version int) models.Migration {
		m := testutil.CreateMigration(t, db, databaseID)
		if err := db.Model(&m).Update("version", version).Error; err != nil {
			t.Fatal(err)
		}
		return m
	}
	exec := func(m models.Migration, kind models.ExecutionKind, status models.ExecutionStatus, reverts *uint) models.MigrationExecution {
		return testutil.CreateExecution(t, db, models.MigrationExecution{MigrationID: m.ID, DatabaseID: m.DatabaseID,
			Kind: kind, Status: status, RevertsExecutionID: reverts})
	}

	if v, err := position.Current(ctx, db, conn.ID); err != nil || v != 0 {
		t.Fatalf("empty database = %d, %v", v, err)
	}

	a, b := migration(conn.ID, 1), migration(conn.ID, 3)
	exec(a, models.ExecKindRun, models.ExecSuccess, nil)
	runB := exec(b, models.ExecKindRun, models.ExecSuccess, nil)
	exec(migration(other.ID, 9), models.ExecKindRun, models.ExecSuccess, nil) // another database

	if v, err := position.Current(ctx, db, conn.ID); err != nil || v != 3 {
		t.Fatalf("after a and b = %d, %v; want 3", v, err)
	}

	undo := exec(b, models.ExecKindRollback, models.ExecSuccess, &runB.ID)
	if v, err := position.Current(ctx, db, conn.ID); err != nil || v != 1 {
		t.Fatalf("after rolling back b = %d, %v; want 1", v, err)
	}

	// The worker checks its own execution against everything else.
	st, versions, err := position.Load(ctx, db, conn.ID, undo.ID)
	if err != nil || position.ActiveVersion(st, versions) != 3 {
		t.Fatalf("excluding the rollback = %d, %v; want 3", position.ActiveVersion(st, versions), err)
	}
	if next, ok := position.NextForward(position.Derive(nil), versions); ok {
		t.Errorf("empty state forwards %d", next)
	}
}

func TestLoadLeavesOutRunsThatNeverStarted(t *testing.T) {
	db := testutil.PlatformDB(t)
	ctx := context.Background()
	conn := testutil.CreateDatabaseConnection(t, db, "toko")

	exec := func(m models.Migration, kind models.ExecutionKind, status models.ExecutionStatus, reverts *uint) models.MigrationExecution {
		return testutil.CreateExecution(t, db, models.MigrationExecution{MigrationID: m.ID, DatabaseID: m.DatabaseID,
			Kind: kind, Status: status, RevertsExecutionID: reverts})
	}
	setVersion := func(m models.Migration, version int) {
		if err := db.Model(&m).Update("version", version).Error; err != nil {
			t.Fatal(err)
		}
	}
	load := func(exclude uint) (position.State, map[uint]int) {
		st, versions, err := position.Load(ctx, db, conn.ID, exclude)
		if err != nil {
			t.Fatal(err)
		}
		return st, versions
	}
	a, b, c := testutil.CreateMigration(t, db, conn.ID), testutil.CreateMigration(t, db, conn.ID), testutil.CreateMigration(t, db, conn.ID)
	recipe := func(m models.Migration) *models.Migration {
		m.SQLDown = "SELECT 1"
		return &m
	}

	runA := exec(a, models.ExecKindRun, models.ExecSuccess, nil)
	setVersion(a, 1)
	exec(a, models.ExecKindRollback, models.ExecSuccess, &runA.ID)
	exec(b, models.ExecKindRun, models.ExecFailed, nil)
	setVersion(b, 2)
	exec(c, models.ExecKindRun, models.ExecFailed, nil)

	st, versions := load(0)
	if st.Retry != b.ID || st.Started[c.ID] {
		t.Fatalf("retry = %d started = %v, want the failed run of %d and no trace of %d", st.Retry, st.Started, b.ID, c.ID)
	}
	if _, err := position.Decide(st, versions, recipe(c), models.ExecKindRun); err != nil {
		t.Errorf("run of a migration whose run never started = %v", err)
	}

	exec(b, models.ExecKindForward, models.ExecSuccess, nil)
	secondC := exec(c, models.ExecKindRun, models.ExecFailed, nil)
	setVersion(c, 3)

	st, versions = load(0)
	if position.Forwardable(st, versions, a.ID) || !position.Forwardable(st, versions, c.ID) {
		t.Errorf("redo = %v retry = %d, want only %d forwardable", st.Redo, st.Retry, c.ID)
	}
	if _, err := position.Decide(st, versions, recipe(a), models.ExecKindForward); err == nil {
		t.Error("forward of a discarded migration is accepted")
	}

	st, versions = load(secondC.ID)
	if _, err := position.Decide(st, versions, recipe(c), models.ExecKindRun); err != nil {
		t.Errorf("second run checked against everything else = %v", err)
	}
}

func TestLoadReadsTheContinuedExecution(t *testing.T) {
	db := testutil.PlatformDB(t)
	conn := testutil.CreateDatabaseConnection(t, db, "toko")
	m := testutil.CreateMigration(t, db, conn.ID)
	if err := db.Model(&m).Update("version", 1).Error; err != nil {
		t.Fatal(err)
	}
	partial := testutil.CreateExecution(t, db, models.MigrationExecution{MigrationID: m.ID, DatabaseID: conn.ID, Status: models.ExecPartial})
	testutil.CreateExecution(t, db, models.MigrationExecution{MigrationID: m.ID, DatabaseID: conn.ID,
		Kind: models.ExecKindForward, Status: models.ExecSuccess, ContinuesExecutionID: &partial.ID})

	st, _, err := position.Load(context.Background(), db, conn.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if st.Partial != nil || !st.IsActive(m.ID) {
		t.Errorf("partial = %+v active = %v, want the continued migration active", st.Partial, st.Active)
	}
}
