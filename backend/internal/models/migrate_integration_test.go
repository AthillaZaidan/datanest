//go:build integration

package models_test

import (
	"testing"

	"github.com/AthillaZaidan/datanest/backend/internal/models"
	"github.com/AthillaZaidan/datanest/backend/internal/testutil"
)

func TestAnExecutionIsRevertedOnlyOnce(t *testing.T) {
	db := testutil.PlatformDB(t)
	conn := testutil.CreateDatabaseConnection(t, db, "toko")
	run := testutil.CreateExecution(t, db, models.MigrationExecution{DatabaseID: conn.ID, Status: models.ExecSuccess})
	rollback := func(status models.ExecutionStatus) error {
		return db.Create(&models.MigrationExecution{MigrationID: run.MigrationID, DatabaseID: conn.ID, Kind: models.ExecKindRollback,
			Status: status, SQLExecuted: "SELECT 1", RevertsExecutionID: &run.ID}).Error
	}

	if err := rollback(models.ExecFailed); err != nil {
		t.Fatalf("failed rollback: %v", err)
	}
	if err := rollback(models.ExecSuccess); err != nil {
		t.Fatalf("first successful rollback: %v", err)
	}
	if err := rollback(models.ExecSuccess); err == nil {
		t.Fatal("second successful rollback of the same execution was accepted")
	}
}

func TestMigrateIsRepeatable(t *testing.T) {
	db := testutil.PlatformDB(t)
	if err := models.Migrate(db); err != nil {
		t.Fatalf("second Migrate: %v", err)
	}
}
