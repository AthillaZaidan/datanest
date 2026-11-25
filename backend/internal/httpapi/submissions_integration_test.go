//go:build integration

package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/AthillaZaidan/datanest/backend/internal/authz"
	"github.com/AthillaZaidan/datanest/backend/internal/execution"
	"github.com/AthillaZaidan/datanest/backend/internal/migration"
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
	if r := e.submit(e.editor, d.ID, map[string]any{"kind": "run", "batchSize": 100}); r.Code != 400 || r.detail("batchSize") == "" {
		t.Errorf("batch size on a normal migration = %d %s", r.Code, r.Body)
	}
	hidden := e.recipe(t, map[string]any{"sqlUp": "SELECT 1", "sqlDown": "COMMIT"}).view(t)
	if r := e.submit(e.editor, hidden.ID, map[string]any{"kind": "run"}); r.Code != 400 || r.errCode() != "TRANSACTION_CONTROL_NOT_ALLOWED" {
		t.Errorf("run with a down SQL that can never run = %d %s", r.Code, r.errCode())
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

func (r resp) detail(field string) string {
	var b struct {
		Error struct{ Details map[string]string }
	}
	json.Unmarshal(r.Body, &b)
	return b.Error.Details[field]
}

const (
	batchUp   = "UPDATE orders SET qty = qty + 1 WHERE id > $1 AND id <= $2"
	batchDown = "UPDATE orders SET qty = qty - 1 WHERE id > $1 AND id <= $2"
)

func (e migEnv) batchRecipe(t *testing.T, table, column string) migration.View {
	t.Helper()
	return e.recipe(t, map[string]any{"sqlUp": batchUp, "sqlDown": batchDown, "mode": "batch", "batchTable": table, "batchColumn": column}).view(t)
}

func (e migEnv) createOrders(t *testing.T) {
	t.Helper()
	if _, err := e.target.Exec(`CREATE TABLE orders (id int PRIMARY KEY, qty int NOT NULL DEFAULT 0, note text UNIQUE)`); err != nil {
		t.Fatal(err)
	}
}

func TestSubmitBatchExecution(t *testing.T) {
	e := setupMigrations(t)
	e.createOrders(t)
	d := e.batchRecipe(t, "orders", "id")

	for _, size := range []any{nil, 0, -5, 1_000_001} {
		body := map[string]any{"kind": "run"}
		if size != nil {
			body["batchSize"] = size
		}
		if r := e.submit(e.editor, d.ID, body); r.Code != 400 || r.detail("batchSize") != "wajib diisi, 1 sampai 1000000" {
			t.Errorf("size %v = %d %s", size, r.Code, r.Body)
		}
	}
	if r := e.submit(e.editor, d.ID, map[string]any{"kind": "rollback", "batchSize": 10}); r.Code != 400 || r.detail("batchSize") == "" {
		t.Errorf("size on a rollback = %d %s", r.Code, r.Body)
	}
	for table, column := range map[string]string{"orders": "missing", "nowhere": "id", "public.orders": "qty", "public.Orders": "note"} {
		bad := e.batchRecipe(t, table, column)
		if r := e.submit(e.editor, bad.ID, map[string]any{"kind": "run", "batchSize": 10}); r.Code != 400 || r.detail("batchColumn") == "" {
			t.Errorf("batch by %s.%s = %d %s", table, column, r.Code, r.Body)
		}
	}

	r := e.submit(e.editor, d.ID, map[string]any{"kind": "run", "batchSize": 250})
	var submitted, seen execution.View
	if err := json.Unmarshal(r.Body, &submitted); err != nil || r.Code != 201 {
		t.Fatalf("submit = %d %s", r.Code, r.Body)
	}
	json.Unmarshal(call(e.withExecutions(e.viewer), "GET", "/api/executions/"+itoa(submitted.ID), nil).Body, &seen)
	for name, v := range map[string]execution.View{"submitted": submitted, "seen by a viewer": seen} {
		if v.Status != "pending_review" || v.Mode != "batch" || v.BatchSize == nil || *v.BatchSize != 250 || v.BatchTable != "orders" ||
			v.BatchColumn != "id" || v.Progress == nil || v.RevertsExecutionID != nil || v.ContinuesExecutionID != nil {
			t.Errorf("%s = %+v", name, v)
		}
	}
}

type watchedProvider struct {
	testutil.StaticProvider
	onGet func()
}

func (p watchedProvider) Get(ctx context.Context, id uint) (*sql.DB, error) {
	p.onGet()
	return p.StaticProvider.Get(ctx, id)
}

func TestSubmitBatchChecksAccessAndSizeBeforeTheTarget(t *testing.T) {
	e := setupMigrations(t)
	e.createOrders(t)
	d := e.batchRecipe(t, "orders", "id")
	reached := 0
	az := dbAuthz{{e.editor.ID, e.dbA}: authz.Edit, {e.viewer.ID, e.dbA}: authz.View}
	e.svc = migration.New(e.platform, watchedProvider{testutil.StaticProvider{DB: e.target}, func() { reached++ }}, az, migration.Options{})

	if r := e.submit(e.viewer, d.ID, map[string]any{"kind": "run", "batchSize": 10}); r.Code != 403 || reached != 0 {
		t.Errorf("viewer = %d, target reached %d times", r.Code, reached)
	}
	if r := e.submit(e.editor, d.ID, map[string]any{"kind": "run"}); r.Code != 400 || reached != 0 {
		t.Errorf("no size = %d, target reached %d times", r.Code, reached)
	}
	if r := e.submit(e.editor, d.ID, map[string]any{"kind": "run", "batchSize": 10}); r.Code != 201 || reached != 1 {
		t.Errorf("editor = %d %s, target reached %d times", r.Code, r.Body, reached)
	}
}

func TestSubmitRefusesARecipeEditedWhileItWasChecked(t *testing.T) {
	e := setupMigrations(t)
	e.createOrders(t)
	d := e.batchRecipe(t, "orders", "id")
	az := dbAuthz{{e.editor.ID, e.dbA}: authz.Edit}
	edit := func() {
		e.platform.Exec(`UPDATE migrations SET batch_column = 'qty', revision = revision + 1 WHERE id = ?`, d.ID)
	}
	e.svc = migration.New(e.platform, watchedProvider{testutil.StaticProvider{DB: e.target}, edit}, az, migration.Options{})

	if r := e.submit(e.editor, d.ID, map[string]any{"kind": "run", "batchSize": 10}); r.Code != 409 || r.errCode() != "REVISION_CONFLICT" {
		t.Errorf("edited meanwhile = %d %s", r.Code, r.Body)
	}
}

func TestExplainOfABatchMigrationIsRefused(t *testing.T) {
	e := setupMigrations(t)
	e.createOrders(t)
	d := e.batchRecipe(t, "orders", "id")
	for _, path := range []string{"/explain", "/explain?analyze=true"} {
		if r := call(e.as(e.editor), "POST", "/api/migrations/"+itoa(d.ID)+path, nil); r.Code != 400 || r.errCode() != "VALIDATION_ERROR" {
			t.Errorf("%s = %d %s", path, r.Code, r.Body)
		}
	}
	if r := call(e.as(e.otherEditor), "POST", "/api/migrations/"+itoa(d.ID)+"/explain", nil); r.Code != 403 {
		t.Errorf("stranger = %d, want 403", r.Code)
	}
}
