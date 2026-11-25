//go:build integration

package models_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/AthillaZaidan/datanest/backend/internal/models"
	"github.com/AthillaZaidan/datanest/backend/internal/position"
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

func TestMigrateBackfillsM1History(t *testing.T) {
	db := testutil.PlatformDB(t)
	ctx := context.Background()
	conn := testutil.CreateDatabaseConnection(t, db, "toko")
	user := testutil.CreateUser(t, db, "budi")
	started := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)

	m1 := func(version int, status models.MigrationStatus) models.Migration {
		m := models.Migration{DatabaseID: conn.ID, SQL: fmt.Sprintf("SELECT %d", version), Status: status,
			CreatedByUsername: "budi", ExecutedByID: &user.ID, ExecutedByUsername: "budi", StartedAt: &started}
		if version > 0 {
			m.Version = &version
		}
		if status == models.MigrationFailed {
			m.ErrorMessage, m.ErrorCode = "boom", "42P01"
		}
		if err := db.Create(&m).Error; err != nil {
			t.Fatal(err)
		}
		return m
	}
	// Created out of order on purpose: executions must follow version order.
	v3 := m1(3, models.MigrationSuccess)
	v1 := m1(1, models.MigrationSuccess)
	v2 := m1(2, models.MigrationFailed)
	running := m1(4, models.MigrationRunning)
	draft := m1(0, models.MigrationDraft)

	for i := 0; i < 2; i++ { // the second run must change nothing
		if err := models.Migrate(db); err != nil {
			t.Fatalf("Migrate #%d: %v", i+1, err)
		}
	}

	var execs []models.MigrationExecution
	db.Where("database_id = ?", conn.ID).Order("id").Find(&execs)
	want := []struct {
		id     uint
		status models.ExecutionStatus
	}{{v1.ID, models.ExecSuccess}, {v2.ID, models.ExecFailed}, {v3.ID, models.ExecSuccess}}
	if len(execs) != len(want) {
		t.Fatalf("executions = %d, want %d: %+v", len(execs), len(want), execs)
	}
	for i, w := range want {
		e := execs[i]
		if e.MigrationID != w.id || e.Kind != models.ExecKindRun || e.Status != w.status || e.RequestedByUsername != "budi" ||
			e.SQLExecuted == "" || e.StartedAt == nil {
			t.Errorf("execution %d = %+v, want migration %d %s", i, e, w.id, w.status)
		}
	}
	if execs[1].ErrorCode != "42P01" || execs[1].ErrorMessage != "boom" {
		t.Errorf("failed attempt lost its error: %+v", execs[1])
	}

	for _, m := range []models.Migration{v1, v2, v3} {
		var got models.Migration
		db.First(&got, m.ID)
		if !got.Irreversible || got.IrreversibleReason != models.M1Reason || got.FrozenAt == nil || got.SQL != m.SQL || got.Status != m.Status {
			t.Errorf("recipe %d = %+v", m.ID, got)
		}
	}
	for _, m := range []models.Migration{running, draft} {
		var got models.Migration
		db.First(&got, m.ID)
		if got.Irreversible || got.FrozenAt != nil || got.Status != m.Status {
			t.Errorf("migration %d (%s) must be left alone: %+v", m.ID, m.Status, got)
		}
	}

	if v, err := position.Current(ctx, db, conn.ID); err != nil || v != 3 {
		t.Errorf("current version after backfill = %d, %v; want 3", v, err)
	}
}
