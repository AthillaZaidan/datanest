//go:build integration

package httpapi

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/AthillaZaidan/datanest/backend/internal/authz"
	"github.com/AthillaZaidan/datanest/backend/internal/execution"
	"github.com/AthillaZaidan/datanest/backend/internal/models"
	"github.com/AthillaZaidan/datanest/backend/internal/testutil"
)

func (e migEnv) withExecutions(u models.User) http.Handler {
	az := dbAuthz{
		{e.editor.ID, e.dbA}:      authz.Edit,
		{e.viewer.ID, e.dbA}:      authz.View,
		{e.otherEditor.ID, e.dbB}: authz.Edit,
	}
	return NewRouter(Deps{RequireAuth: testutil.AsUser(u.ID), Migrations: e.svc, Executions: execution.NewService(e.platform, az)})
}

func (e migEnv) submit(u models.User, migrationID uint, body any) resp {
	return call(e.withExecutions(u), "POST", "/api/migrations/"+itoa(migrationID)+"/executions", body)
}

func TestSubmitExecution(t *testing.T) {
	e := setupMigrations(t)
	d := e.recipe(t, map[string]any{"sqlUp": "CREATE TABLE s (id int)", "sqlDown": "DROP TABLE s"}).view(t)
	incomplete := e.recipe(t, map[string]any{"sqlUp": "SELECT 1"}).view(t)

	if r := e.submit(e.viewer, d.ID, map[string]any{"kind": "run"}); r.Code != 403 {
		t.Errorf("viewer submit = %d, want 403", r.Code)
	}
	if r := e.submit(e.otherEditor, d.ID, map[string]any{"kind": "run"}); r.Code != 403 {
		t.Errorf("editor of another database = %d, want 403", r.Code)
	}
	if r := e.submit(e.editor, d.ID, map[string]any{"kind": "drop"}); r.Code != 400 {
		t.Errorf("unknown kind = %d, want 400", r.Code)
	}
	if r := e.submit(e.editor, d.ID, map[string]any{"kind": "run", "batchSize": 100}); r.Code != 400 {
		t.Errorf("batch size = %d, want 400 until batch mode can be submitted", r.Code)
	}
	if r := e.submit(e.editor, incomplete.ID, map[string]any{"kind": "run"}); r.Code != 400 || r.errCode() != "VALIDATION_ERROR" {
		t.Errorf("recipe without down SQL = %d %s", r.Code, r.errCode())
	}
	if r := e.submit(e.editor, d.ID, map[string]any{"kind": "rollback"}); r.Code != 409 || r.errCode() != "ROLLBACK_NOT_ALLOWED" {
		t.Errorf("rollback of a draft = %d %s", r.Code, r.errCode())
	}
	if r := e.submit(e.editor, d.ID, map[string]any{"kind": "forward"}); r.Code != 409 || r.errCode() != "FORWARD_NOT_ALLOWED" {
		t.Errorf("forward of a draft = %d %s", r.Code, r.errCode())
	}

	r := e.submit(e.editor, d.ID, map[string]any{"kind": "run"})
	var v execution.View
	if err := json.Unmarshal(r.Body, &v); err != nil || r.Code != 201 {
		t.Fatalf("submit run = %d %s", r.Code, r.Body)
	}
	if v.Kind != "run" || v.Status != "pending_review" || v.SQLExecuted == nil || *v.SQLExecuted != d.SQLUp || v.RequestedBy != "editor" {
		t.Errorf("submitted execution = %+v", v)
	}
	if e.tableExists(t, "s") {
		t.Error("submitting ran the SQL")
	}
	if r := e.submit(e.editor, d.ID, map[string]any{"kind": "run"}); r.Code != 409 || r.errCode() != "EXECUTION_STATE_CONFLICT" {
		t.Errorf("second request while one is open = %d %s", r.Code, r.errCode())
	}
}
