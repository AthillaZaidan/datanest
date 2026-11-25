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
	runningWithoutVersion := m1(0, models.MigrationRunning)
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
	}{{v1.ID, models.ExecSuccess}, {v2.ID, models.ExecFailed}, {v3.ID, models.ExecSuccess}, {running.ID, models.ExecFailed}}
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
	var interrupted models.Migration
	db.First(&interrupted, running.ID)
	if interrupted.Status != models.MigrationFailed || interrupted.ErrorMessage != models.M1InterruptedReason ||
		interrupted.FinishedAt == nil || interrupted.FrozenAt == nil || !interrupted.Irreversible {
		t.Errorf("running row was not settled as failed: %+v", interrupted)
	}
	if execs[3].ErrorMessage != models.M1InterruptedReason || execs[3].FinishedAt == nil {
		t.Errorf("interrupted attempt = %+v", execs[3])
	}

	var orphan models.Migration
	db.First(&orphan, runningWithoutVersion.ID)
	if orphan.Status != models.MigrationFailed || orphan.ErrorMessage != models.M1InterruptedReason || orphan.FinishedAt == nil {
		t.Errorf("running row without version = %+v", orphan)
	}
	var orphanRuns int64
	db.Model(&models.MigrationExecution{}).Where("migration_id = ?", orphan.ID).Count(&orphanRuns)
	if orphanRuns != 0 {
		t.Errorf("running row without version recorded %d executions", orphanRuns)
	}

	var untouched models.Migration
	db.First(&untouched, draft.ID)
	if untouched.Irreversible || untouched.FrozenAt != nil || untouched.Status != models.MigrationDraft {
		t.Errorf("draft must be left alone: %+v", untouched)
	}

	if v, err := position.Current(ctx, db, conn.ID); err != nil || v != 3 {
		t.Errorf("current version after backfill = %d, %v; want 3", v, err)
	}
}

func TestMigrateNumbersStartedExecutionsInStartOrder(t *testing.T) {
	db := testutil.PlatformDB(t)
	conn := testutil.CreateDatabaseConnection(t, db, "toko")
	other := testutil.CreateDatabaseConnection(t, db, "gudang")
	base := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	at := func(minutes int) *time.Time {
		v := base.Add(time.Duration(minutes) * time.Minute)
		return &v
	}
	create := func(databaseID uint, status models.ExecutionStatus, startedAt *time.Time) models.MigrationExecution {
		return testutil.CreateExecution(t, db, models.MigrationExecution{DatabaseID: databaseID, Status: status, StartedAt: startedAt})
	}
	late := create(conn.ID, models.ExecSuccess, at(30))
	early := create(conn.ID, models.ExecFailed, at(10))
	runningWithoutStart := create(conn.ID, models.ExecRunning, nil)
	pending := create(conn.ID, models.ExecQueued, nil)
	approved := create(conn.ID, models.ExecApproved, nil)
	elsewhere := create(other.ID, models.ExecSuccess, at(5))

	seeded := create(conn.ID, models.ExecSuccess, at(1))
	if err := db.Model(&seeded).Update("run_seq", 7).Error; err != nil {
		t.Fatal(err)
	}

	snapshot := func() map[uint]*int64 {
		var rows []models.MigrationExecution
		db.Find(&rows)
		out := map[uint]*int64{}
		for _, r := range rows {
			out[r.ID] = r.RunSeq
		}
		return out
	}
	for i := 0; i < 2; i++ {
		if err := models.Migrate(db); err != nil {
			t.Fatalf("Migrate #%d: %v", i+1, err)
		}
		got := snapshot()
		want := map[uint]int64{seeded.ID: 7, early.ID: 8, late.ID: 9, runningWithoutStart.ID: 10, elsewhere.ID: 1}
		for id, seq := range want {
			if got[id] == nil || *got[id] != seq {
				t.Errorf("Migrate #%d: execution %d run_seq = %v, want %d", i+1, id, got[id], seq)
			}
		}
		for _, id := range []uint{pending.ID, approved.ID} {
			if got[id] != nil {
				t.Errorf("Migrate #%d: pending execution %d got run_seq %d", i+1, id, *got[id])
			}
		}
	}
}

func TestMigrateBackfillsTheVersionAndRevisionOfExecutions(t *testing.T) {
	db := testutil.PlatformDB(t)
	conn := testutil.CreateDatabaseConnection(t, db, "toko")
	version := 4
	m := models.Migration{DatabaseID: conn.ID, SQL: "SELECT 1", Status: models.MigrationSuccess, Version: &version, Revision: 3}
	if err := db.Create(&m).Error; err != nil {
		t.Fatal(err)
	}
	unversioned := models.Migration{DatabaseID: conn.ID, SQL: "SELECT 2", Status: models.MigrationDraft, Revision: 2}
	if err := db.Create(&unversioned).Error; err != nil {
		t.Fatal(err)
	}
	create := func(migrationID uint, status models.ExecutionStatus) models.MigrationExecution {
		return testutil.CreateExecution(t, db, models.MigrationExecution{MigrationID: migrationID, DatabaseID: conn.ID, Status: status})
	}
	started := create(m.ID, models.ExecSuccess)
	waiting := create(m.ID, models.ExecPendingReview)
	draftRequest := create(unversioned.ID, models.ExecPendingReview)
	own := create(m.ID, models.ExecSuccess)
	ownVersion, ownRevision := 9, 8
	if err := db.Model(&own).Updates(map[string]any{"version": ownVersion, "revision": ownRevision}).Error; err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 2; i++ {
		if err := models.Migrate(db); err != nil {
			t.Fatalf("Migrate #%d: %v", i+1, err)
		}
		get := func(id uint) models.MigrationExecution {
			var e models.MigrationExecution
			if err := db.First(&e, id).Error; err != nil {
				t.Fatal(err)
			}
			return e
		}
		check := func(name string, id uint, wantVersion, wantRevision int) {
			t.Helper()
			e := get(id)
			gotVersion := 0
			if e.Version != nil {
				gotVersion = *e.Version
			}
			if gotVersion != wantVersion || e.Revision != wantRevision {
				t.Errorf("Migrate #%d: %s has version %d revision %d, want %d %d (0 means no version)", i+1, name, gotVersion, e.Revision, wantVersion, wantRevision)
			}
		}
		check("started execution", started.ID, version, 3)
		check("waiting execution", waiting.ID, 0, 3)
		check("request of a draft", draftRequest.ID, 0, 2)
		check("execution with its own values", own.ID, ownVersion, ownRevision)
	}
}
