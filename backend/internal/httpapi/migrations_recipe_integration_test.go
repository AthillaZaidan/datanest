//go:build integration

package httpapi

import (
	"testing"
	"time"

	"github.com/AthillaZaidan/datanest/backend/internal/models"
)

func (e migEnv) recipe(t *testing.T, body map[string]any) resp {
	t.Helper()
	return call(e.as(e.editor), "POST", "/api/databases/"+itoa(e.dbA)+"/migrations", body)
}

func TestRecipeCreateAndValidate(t *testing.T) {
	e := setupMigrations(t)
	list := "/api/databases/" + itoa(e.dbA) + "/migrations"

	if r := call(e.as(e.viewer), "POST", list, map[string]any{"sqlUp": "SELECT 1", "sqlDown": "SELECT 2"}); r.Code != 403 {
		t.Errorf("viewer create = %d, want 403", r.Code)
	}
	if r := e.recipe(t, map[string]any{"sqlUp": "SELECT 1", "irreversible": true}); r.Code != 400 || r.errCode() != "VALIDATION_ERROR" {
		t.Errorf("irreversible without reason = %d %s", r.Code, r.errCode())
	}
	if r := e.recipe(t, map[string]any{"sqlDown": "SELECT 2"}); r.Code != 400 {
		t.Errorf("missing up SQL = %d", r.Code)
	}

	r := e.recipe(t, map[string]any{"sqlUp": "CREATE TABLE t (id int)", "sqlDown": "DROP TABLE t"})
	v := r.view(t)
	if r.Code != 201 || v.SQLUp != "CREATE TABLE t (id int)" || v.SQL != v.SQLUp || v.SQLDown != "DROP TABLE t" || v.Irreversible || v.FrozenAt != nil {
		t.Fatalf("reversible = %d %+v", r.Code, v)
	}

	r = e.recipe(t, map[string]any{"sqlUp": "DROP TABLE t", "sqlDown": "ignored", "irreversible": true, "irreversibleReason": "data hilang"})
	v = r.view(t)
	if r.Code != 201 || !v.Irreversible || v.IrreversibleReason != "data hilang" || v.SQLDown != "" {
		t.Fatalf("irreversible = %d %+v", r.Code, v)
	}

	dup := call(e.as(e.editor), "POST", "/api/migrations/"+itoa(v.ID)+"/duplicate", nil).view(t)
	if !dup.Irreversible || dup.IrreversibleReason != "data hilang" || dup.SQLUp != v.SQLUp {
		t.Errorf("duplicate lost the recipe: %+v", dup)
	}
}

func TestRecipeLockedWhileAnExecutionIsOpen(t *testing.T) {
	e := setupMigrations(t)
	d := e.recipe(t, map[string]any{"sqlUp": "SELECT 1", "sqlDown": "SELECT 2"}).view(t)
	path := "/api/migrations/" + itoa(d.ID)
	exec := models.MigrationExecution{MigrationID: d.ID, DatabaseID: e.dbA, Kind: models.ExecKindRun,
		Status: models.ExecPendingReview, SQLExecuted: "SELECT 1"}
	if err := e.platform.Create(&exec).Error; err != nil {
		t.Fatal(err)
	}

	r := call(e.as(e.editor), "PATCH", path, map[string]any{"sqlUp": "SELECT 3", "sqlDown": "SELECT 4", "revision": 1})
	if r.Code != 409 || r.errCode() != "EXECUTION_STATE_CONFLICT" {
		t.Fatalf("edit while pending review = %d %s", r.Code, r.errCode())
	}
	if r := call(e.as(e.editor), "DELETE", path, nil); r.Code != 409 {
		t.Errorf("delete while pending review = %d, want 409", r.Code)
	}

	// A rejected request unlocks the recipe so it can be fixed (design D4).
	e.platform.Model(&exec).Update("status", models.ExecRejected)
	r = call(e.as(e.editor), "PATCH", path, map[string]any{"sqlUp": "SELECT 3", "sqlDown": "SELECT 4", "revision": 1})
	if v := r.view(t); r.Code != 200 || v.SQLUp != "SELECT 3" || v.Revision != 2 {
		t.Fatalf("edit after rejection = %d %+v", r.Code, v)
	}

	// But its history keeps it from being deleted.
	if r := call(e.as(e.editor), "DELETE", path, nil); r.Code != 409 {
		t.Errorf("delete with history = %d, want 409", r.Code)
	}
	var n int64
	e.platform.Model(&models.MigrationExecution{}).Where("migration_id = ?", d.ID).Count(&n)
	if n != 1 {
		t.Errorf("history rows = %d, want 1", n)
	}
}

func TestRecipeImmutableOnceFrozen(t *testing.T) {
	e := setupMigrations(t)
	d := e.recipe(t, map[string]any{"sqlUp": "SELECT 1", "sqlDown": "SELECT 2"}).view(t)
	if err := e.platform.Model(&models.Migration{}).Where("id = ?", d.ID).Update("frozen_at", time.Now()).Error; err != nil {
		t.Fatal(err)
	}
	r := call(e.as(e.editor), "PATCH", "/api/migrations/"+itoa(d.ID), map[string]any{"sqlUp": "DROP TABLE x", "revision": 1})
	if r.Code != 409 || r.errCode() != "MIGRATION_NOT_DRAFT" {
		t.Fatalf("edit frozen = %d %s", r.Code, r.errCode())
	}
	if v := call(e.as(e.viewer), "GET", "/api/migrations/"+itoa(d.ID), nil).view(t); v.SQLUp != "SELECT 1" || v.FrozenAt == nil {
		t.Errorf("frozen recipe changed: %+v", v)
	}
}
