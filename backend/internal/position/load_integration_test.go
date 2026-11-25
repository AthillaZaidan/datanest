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
