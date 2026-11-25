//go:build integration

package httpapi

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/AthillaZaidan/datanest/backend/internal/migration"
	"github.com/AthillaZaidan/datanest/backend/internal/models"
)

func (e migEnv) runsOf(t *testing.T, migrationID uint) []models.MigrationExecution {
	t.Helper()
	var rows []models.MigrationExecution
	if err := e.platform.Where("migration_id = ?", migrationID).Order("id").Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	return rows
}

// While POST /execute still exists, its result joins the execution history.
func TestDirectExecuteRecordsARunExecution(t *testing.T) {
	e := setupMigrations(t)
	good := e.draft(t, "CREATE TABLE recorded (id int)")
	bad := e.draft(t, "INSERT INTO missing VALUES (1)")

	if r := e.execute(t, e.viewer, good.ID); r.Code != 403 {
		t.Fatalf("viewer execute = %d", r.Code)
	}
	if rows := e.runsOf(t, good.ID); len(rows) != 0 {
		t.Fatalf("refused execute recorded %d executions", len(rows))
	}

	e.execute(t, e.editor, good.ID)
	e.execute(t, e.editor, bad.ID)

	rows := e.runsOf(t, good.ID)
	if len(rows) != 1 || rows[0].Kind != models.ExecKindRun || rows[0].Status != models.ExecSuccess ||
		rows[0].SQLExecuted != good.SQL || rows[0].RequestedByUsername != "editor" || rows[0].FinishedAt == nil {
		t.Fatalf("success run = %+v", rows)
	}
	rows = e.runsOf(t, bad.ID)
	if len(rows) != 1 || rows[0].Status != models.ExecFailed || rows[0].ErrorCode != "42P01" {
		t.Fatalf("failed run = %+v", rows)
	}

	var m models.Migration
	e.platform.First(&m, good.ID)
	if m.FrozenAt == nil {
		t.Error("executed migration was not frozen")
	}
}

func TestReconcileRecordsARunExecution(t *testing.T) {
	e := setupMigrations(t)
	if _, err := e.target.Exec(`CREATE TABLE _platform_migrations (version integer PRIMARY KEY, applied_at timestamptz NOT NULL DEFAULT now());
		INSERT INTO _platform_migrations (version) VALUES (5)`); err != nil {
		t.Fatal(err)
	}
	five, six := 5, 6
	committed := models.Migration{DatabaseID: e.dbA, Version: &five, SQL: "SELECT 5", Status: models.MigrationRunning, CreatedByUsername: "editor"}
	lost := models.Migration{DatabaseID: e.dbA, Version: &six, SQL: "SELECT 6", Status: models.MigrationRunning, CreatedByUsername: "editor"}
	e.platform.Create(&committed)
	e.platform.Create(&lost)

	for i := 0; i < 2; i++ { // reconciling twice must not record twice
		if err := e.svc.Reconcile(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if rows := e.runsOf(t, committed.ID); len(rows) != 1 || rows[0].Status != models.ExecSuccess {
		t.Errorf("committed = %+v", rows)
	}
	if rows := e.runsOf(t, lost.ID); len(rows) != 1 || rows[0].Status != models.ExecFailed || rows[0].ErrorMessage == "" {
		t.Errorf("lost = %+v", rows)
	}
}

func TestMigrationListFollowsTheActivePosition(t *testing.T) {
	e := setupMigrations(t)
	d := e.draft(t, "CREATE TABLE first (id int)")
	e.execute(t, e.editor, d.ID)

	current := func() int {
		t.Helper()
		var l migration.ListResult
		r := call(e.as(e.viewer), "GET", "/api/databases/"+itoa(e.dbA)+"/migrations", nil)
		if err := json.Unmarshal(r.Body, &l); err != nil || r.Code != 200 {
			t.Fatalf("list = %d %s", r.Code, r.Body)
		}
		return l.CurrentVersion
	}
	if v := current(); v != 1 {
		t.Fatalf("after run = %d, want 1", v)
	}
	run := e.runsOf(t, d.ID)[0]
	if err := e.platform.Create(&models.MigrationExecution{MigrationID: d.ID, DatabaseID: e.dbA, Kind: models.ExecKindRollback,
		Status: models.ExecSuccess, SQLExecuted: "DROP TABLE first", RevertsExecutionID: &run.ID}).Error; err != nil {
		t.Fatal(err)
	}
	if v := current(); v != 0 {
		t.Errorf("after rolling back the first migration = %d, want 0", v)
	}
	if r := call(e.as(e.otherEditor), "GET", "/api/databases/"+itoa(e.dbA)+"/migrations", nil); r.Code != 403 {
		t.Errorf("foreign editor list = %d, want 403", r.Code)
	}
}
