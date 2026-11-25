//go:build integration

package httpapi

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/AthillaZaidan/datanest/backend/internal/authz"
	"github.com/AthillaZaidan/datanest/backend/internal/migration"
	"github.com/AthillaZaidan/datanest/backend/internal/models"
	"github.com/AthillaZaidan/datanest/backend/internal/testutil"
	"github.com/AthillaZaidan/datanest/backend/internal/worker"
)

// dbAuthz grants levels per (user, database).
type dbAuthz map[[2]uint]authz.Level

func (f dbAuthz) Can(_ context.Context, userID, databaseID uint, a authz.Action) (bool, error) {
	need, ok := authz.RequiredLevel(a)
	return ok && f[[2]uint{userID, databaseID}] >= need, nil
}

type migEnv struct {
	platform                    *gorm.DB
	target                      *sql.DB
	svc                         *migration.Service
	dbA, dbB                    uint
	editor, viewer, otherEditor models.User
}

func setupMigrations(t *testing.T) migEnv {
	t.Helper()
	platform := testutil.PlatformDB(t)
	target := testutil.TargetDB(t)
	e := migEnv{platform: platform, target: target}
	e.editor = testutil.CreateUser(t, platform, "editor")
	e.viewer = testutil.CreateUser(t, platform, "viewer")
	e.otherEditor = testutil.CreateUser(t, platform, "other")
	e.dbA = testutil.CreateDatabaseConnection(t, platform, "toko").ID
	e.dbB = testutil.CreateDatabaseConnection(t, platform, "kampus").ID

	az := dbAuthz{
		{e.editor.ID, e.dbA}:      authz.Edit,
		{e.viewer.ID, e.dbA}:      authz.View,
		{e.otherEditor.ID, e.dbB}: authz.Edit,
	}
	e.svc = migration.New(platform, testutil.StaticProvider{DB: target}, az, migration.Options{
		LockTimeout:      time.Second,
		StatementTimeout: 5 * time.Second,
		ExecTimeout:      10 * time.Second,
	})
	return e
}

func (e migEnv) as(u models.User) http.Handler {
	return NewRouter(Deps{RequireAuth: testutil.AsUser(u.ID), Migrations: e.svc})
}

type resp struct {
	Code int
	Body []byte
}

func (r resp) errCode() string {
	var b struct{ Error struct{ Code string } }
	json.Unmarshal(r.Body, &b)
	return b.Error.Code
}

func (r resp) view(t *testing.T) migration.View {
	t.Helper()
	var v migration.View
	if err := json.Unmarshal(r.Body, &v); err != nil {
		t.Fatalf("decode view: %v (%s)", err, r.Body)
	}
	return v
}

func call(h http.Handler, method, path string, body any) resp {
	var buf bytes.Buffer
	if body != nil {
		json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return resp{w.Code, w.Body.Bytes()}
}

func (e migEnv) draft(t *testing.T, sql string) migration.View {
	t.Helper()
	r := call(e.as(e.editor), "POST", "/api/databases/"+itoa(e.dbA)+"/migrations",
		map[string]any{"sql": sql, "irreversible": true, "irreversibleReason": "test"})
	if r.Code != 201 {
		t.Fatalf("create draft = %d %s", r.Code, r.Body)
	}
	return r.view(t)
}

func (e migEnv) run(t *testing.T, id uint) models.MigrationExecution {
	t.Helper()
	ctx := context.Background()
	submitted, err := e.svc.Submit(ctx, e.editor.ID, id, models.ExecKindRun)
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	if err := e.platform.Model(submitted).Update("status", models.ExecQueued).Error; err != nil {
		t.Fatal(err)
	}
	w := worker.New(e.platform, testutil.StaticProvider{DB: e.target}, worker.Config{
		MaxAttempts:      1,
		LockTimeout:      time.Second,
		StatementTimeout: 5 * time.Second,
		ExecTimeout:      10 * time.Second,
		HeartbeatEvery:   time.Hour,
	})
	w.Revalidate = e.svc.Revalidate
	claimed, result, err := w.Claim(ctx, submitted.ID)
	if err != nil || result != worker.ClaimOK {
		t.Fatalf("claim = %v, %v", result, err)
	}
	if err := w.Run(ctx, claimed); err != nil {
		t.Fatalf("run: %v", err)
	}
	var finished models.MigrationExecution
	if err := e.platform.First(&finished, submitted.ID).Error; err != nil {
		t.Fatal(err)
	}
	return finished
}

func (e migEnv) get(t *testing.T, id uint) migration.View {
	t.Helper()
	r := call(e.as(e.viewer), "GET", "/api/migrations/"+itoa(id), nil)
	if r.Code != 200 {
		t.Fatalf("get migration = %d %s", r.Code, r.Body)
	}
	return r.view(t)
}

func (e migEnv) tableExists(t *testing.T, name string) bool {
	t.Helper()
	var ok bool
	e.target.QueryRow(`SELECT to_regclass($1) IS NOT NULL`, "public."+name).Scan(&ok)
	return ok
}

func TestMigrationDraftLifecycle(t *testing.T) {
	e := setupMigrations(t)
	list := "/api/databases/" + itoa(e.dbA) + "/migrations"

	if r := call(e.as(e.viewer), "POST", list, map[string]string{"sql": "SELECT 1"}); r.Code != 403 {
		t.Errorf("viewer create = %d, want 403", r.Code)
	}
	if r := call(e.as(e.editor), "POST", list, map[string]string{"sql": "   "}); r.Code != 400 || r.errCode() != "VALIDATION_ERROR" {
		t.Errorf("blank sql = %d %s", r.Code, r.errCode())
	}
	if r := call(e.as(e.editor), "POST", list, map[string]string{}); r.Code != 400 {
		t.Errorf("missing sql = %d", r.Code)
	}

	d := e.draft(t, "CREATE TABLE a (id int)")
	if d.Status != models.MigrationDraft || d.Revision != 1 || d.Version != nil || d.CreatedBy != "editor" {
		t.Fatalf("draft = %+v", d)
	}

	var listed migration.ListResult
	r := call(e.as(e.viewer), "GET", list, nil)
	json.Unmarshal(r.Body, &listed)
	if r.Code != 200 || len(listed.Migrations) != 1 || listed.CurrentVersion != 0 {
		t.Fatalf("viewer list = %d %+v", r.Code, listed)
	}
	if r := call(e.as(e.viewer), "GET", list+"?status=bogus", nil); r.Code != 400 {
		t.Errorf("bad status filter = %d", r.Code)
	}

	path := "/api/migrations/" + itoa(d.ID)
	if r := call(e.as(e.viewer), "PATCH", path, map[string]any{"sql": "SELECT 2", "revision": 1}); r.Code != 403 {
		t.Errorf("viewer update = %d, want 403", r.Code)
	}
	r = call(e.as(e.editor), "PATCH", path, map[string]any{"sql": "CREATE TABLE b (id int)", "revision": 1})
	if v := r.view(t); r.Code != 200 || v.Revision != 2 || v.SQL != "CREATE TABLE b (id int)" || v.UpdatedBy != "editor" {
		t.Fatalf("update = %d %+v", r.Code, v)
	}
	if r := call(e.as(e.editor), "PATCH", path, map[string]any{"sql": "SELECT 3", "revision": 1}); r.Code != 409 || r.errCode() != "REVISION_CONFLICT" {
		t.Errorf("stale revision = %d %s", r.Code, r.errCode())
	}
	if r := call(e.as(e.editor), "PATCH", path, map[string]any{"sql": "SELECT 3"}); r.Code != 400 {
		t.Errorf("missing revision = %d", r.Code)
	}

	if r := call(e.as(e.viewer), "DELETE", path, nil); r.Code != 403 {
		t.Errorf("viewer delete = %d", r.Code)
	}
	if r := call(e.as(e.editor), "DELETE", path, nil); r.Code != 204 {
		t.Errorf("delete draft = %d", r.Code)
	}
	if r := call(e.as(e.viewer), "GET", path, nil); r.Code != 404 {
		t.Errorf("get deleted = %d", r.Code)
	}
}

func TestMigrationRunSuccess(t *testing.T) {
	e := setupMigrations(t)
	d := e.draft(t, `
		CREATE TABLE products (id serial PRIMARY KEY, name text);
		CREATE FUNCTION product_count() RETURNS bigint AS $$
			SELECT count(*) FROM products; -- semicolon inside a function body
		$$ LANGUAGE sql;
		INSERT INTO products (name) VALUES ('a'), ('b');`)

	x := e.run(t, d.ID)
	v := e.get(t, d.ID)
	if x.Status != models.ExecSuccess || x.FinishedAt == nil || v.Version == nil || *v.Version != 1 || v.FrozenAt == nil {
		t.Fatalf("run = %+v, view = %+v", x, v)
	}
	var n int
	if err := e.target.QueryRow(`SELECT product_count()`).Scan(&n); err != nil || n != 2 {
		t.Fatalf("product_count = %d, %v", n, err)
	}

	d2 := e.draft(t, "ALTER TABLE products ADD COLUMN price int")
	if d2.BaseVersion == nil || *d2.BaseVersion != 1 || d2.BaseOutdated {
		t.Errorf("new draft should record base version 1: %+v", d2)
	}
	e.run(t, d2.ID)
	if v := e.get(t, d2.ID); *v.Version != 2 {
		t.Errorf("second version = %v", v.Version)
	}

	path := "/api/migrations/" + itoa(d.ID)
	if r := call(e.as(e.editor), "PATCH", path, map[string]any{"sql": "SELECT 1", "revision": 1}); r.errCode() != "MIGRATION_NOT_DRAFT" {
		t.Errorf("edit executed = %d %s", r.Code, r.errCode())
	}
	if r := call(e.as(e.editor), "DELETE", path, nil); r.errCode() != "MIGRATION_NOT_DRAFT" {
		t.Errorf("delete executed = %d %s", r.Code, r.errCode())
	}

	var listed migration.ListResult
	json.Unmarshal(call(e.as(e.viewer), "GET", "/api/databases/"+itoa(e.dbA)+"/migrations", nil).Body, &listed)
	if listed.CurrentVersion != 2 || *listed.Migrations[0].Version != 2 {
		t.Errorf("history = %+v", listed)
	}
}

func TestMigrationRunFailureRollsBack(t *testing.T) {
	e := setupMigrations(t)
	d := e.draft(t, "CREATE TABLE half (id int); INSERT INTO missing VALUES (1);")

	x := e.run(t, d.ID)
	if x.Status != models.ExecFailed || x.ErrorCode != "42P01" || !strings.Contains(x.ErrorMessage, "missing") {
		t.Fatalf("failed run = %+v", x)
	}
	if e.tableExists(t, "half") {
		t.Error("first statement must be rolled back")
	}

	r := call(e.as(e.editor), "POST", "/api/migrations/"+itoa(d.ID)+"/duplicate", nil)
	dup := r.view(t)
	if r.Code != 201 || dup.Status != models.MigrationDraft || dup.SQL != d.SQL {
		t.Fatalf("duplicate = %d %+v", r.Code, dup)
	}

	ok := e.draft(t, "CREATE TABLE fine (id int)")
	e.run(t, ok.ID)
	if v := e.get(t, ok.ID); *v.Version != 2 {
		t.Errorf("version after failed v1 = %d, want 2", *v.Version)
	}
}

func TestMigrationNotInTransaction(t *testing.T) {
	e := setupMigrations(t)
	if _, err := e.target.Exec(`CREATE TABLE t (id int)`); err != nil {
		t.Fatal(err)
	}
	d := e.draft(t, "CREATE INDEX CONCURRENTLY idx_t ON t (id)")
	x := e.run(t, d.ID)
	if x.Status != models.ExecFailed || x.ErrorCode != "25001" || !strings.Contains(x.ErrorMessage, "dalam transaction") {
		t.Fatalf("concurrent index = %+v", x)
	}
}

func TestMigrationIDOR(t *testing.T) {
	e := setupMigrations(t)
	d := e.draft(t, "CREATE TABLE secret (id int)")
	other := e.as(e.otherEditor)

	if r := e.submit(e.otherEditor, d.ID, map[string]any{"kind": "run"}); r.Code != 403 {
		t.Errorf("submit foreign migration = %d", r.Code)
	}
	if r := call(other, "PATCH", "/api/migrations/"+itoa(d.ID), map[string]any{"sql": "DROP TABLE x", "revision": 1}); r.Code != 403 {
		t.Errorf("update foreign migration = %d", r.Code)
	}
	if r := call(other, "GET", "/api/databases/"+itoa(e.dbA)+"/migrations", nil); r.Code != 403 {
		t.Errorf("list foreign database = %d", r.Code)
	}
	if e.tableExists(t, "secret") {
		t.Error("migration must not have run")
	}
}

func TestMigrationLockTimeout(t *testing.T) {
	e := setupMigrations(t)
	if _, err := e.target.Exec(`CREATE TABLE busy (id int)`); err != nil {
		t.Fatal(err)
	}
	holder, err := e.target.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer holder.Rollback()
	if _, err := holder.Exec(`LOCK TABLE busy IN ACCESS EXCLUSIVE MODE`); err != nil {
		t.Fatal(err)
	}

	d := e.draft(t, "ALTER TABLE busy ADD COLUMN x int")
	start := time.Now()
	x := e.run(t, d.ID)
	if x.Status != models.ExecFailed || x.ErrorCode != "55P03" {
		t.Fatalf("locked table = %+v", x)
	}
	if time.Since(start) > 4*time.Second {
		t.Errorf("lock_timeout not applied, took %v", time.Since(start))
	}
}

func TestMigrationDeletedRequester(t *testing.T) {
	e := setupMigrations(t)
	d := e.draft(t, "CREATE TABLE kept (id int)")
	x := e.run(t, d.ID)

	if err := e.platform.Delete(&e.editor).Error; err != nil {
		t.Fatal(err)
	}
	v := call(e.as(e.viewer), "GET", "/api/migrations/"+itoa(d.ID), nil).view(t)
	if v.CreatedBy != "editor (deleted)" || *v.Version != 1 {
		t.Errorf("view after deleting the requester = %+v", v)
	}
	code, ids := e.history(t, e.viewer, "")
	if code != 200 || len(ids) != 1 || ids[0] != x.ID {
		t.Fatalf("history = %d %v", code, ids)
	}
	var body struct{ Executions []migration.HistoryItem }
	json.Unmarshal(call(e.as(e.viewer), "GET", "/api/databases/"+itoa(e.dbA)+"/executions", nil).Body, &body)
	if body.Executions[0].RequestedBy != "editor (deleted)" {
		t.Errorf("history requester = %q", body.Executions[0].RequestedBy)
	}
}

func TestExplainMigration(t *testing.T) {
	e := setupMigrations(t)
	v := e.draft(t, "CREATE TABLE extra (id int);\nSELECT * FROM extra WHERE id = 1;")
	path := "/api/migrations/" + itoa(v.ID) + "/explain"

	type plans struct {
		Plans []migration.StatementPlan `json:"plans"`
	}
	decode := func(r resp) plans {
		t.Helper()
		var p plans
		if err := json.Unmarshal(r.Body, &p); err != nil {
			t.Fatalf("decode: %v (%s)", err, r.Body)
		}
		return p
	}

	// VIEW may EXPLAIN. The DDL is only run, so the SELECT after it can still be planned.
	r := call(e.as(e.viewer), "POST", path, nil)
	if r.Code != 200 {
		t.Fatalf("view explain = %d %s", r.Code, r.Body)
	}
	p := decode(r)
	if len(p.Plans) != 2 || p.Plans[0].Explained || !p.Plans[1].Explained || p.Plans[1].Nodes[0].Relation != "extra" {
		t.Fatalf("plans = %+v", p.Plans)
	}
	if p.Plans[1].Nodes[0].ActualMs != nil {
		t.Fatal("plain EXPLAIN reports actual time")
	}

	// ANALYZE executes the SQL, so VIEW is refused and EDIT gets actual times.
	if r := call(e.as(e.viewer), "POST", path+"?analyze=true", nil); r.Code != 403 {
		t.Fatalf("view analyze = %d", r.Code)
	}
	if r := call(e.as(e.otherEditor), "POST", path, nil); r.Code != 403 {
		t.Fatalf("other database editor = %d", r.Code)
	}
	r = call(e.as(e.editor), "POST", path+"?analyze=true", nil)
	if r.Code != 200 {
		t.Fatalf("editor analyze = %d %s", r.Code, r.Body)
	}
	if p := decode(r); p.Plans[1].Nodes[0].ActualMs == nil || p.Plans[1].ExecutionMs == nil {
		t.Fatalf("ANALYZE without actual times: %s", r.Body)
	}

	if e.tableExists(t, "extra") {
		t.Fatal("explain kept the DDL")
	}
}

func TestDirectExecuteRouteIsGone(t *testing.T) {
	e := setupMigrations(t)
	d := e.draft(t, "CREATE TABLE direct (id int)")

	r := call(e.as(e.editor), "POST", "/api/migrations/"+itoa(d.ID)+"/execute", nil)
	if r.Code != 404 {
		t.Errorf("POST /execute = %d, want 404", r.Code)
	}
	if e.tableExists(t, "direct") {
		t.Error("the SQL ran")
	}
	if rows := e.runsOf(t, d.ID); len(rows) != 0 {
		t.Errorf("recorded %d executions", len(rows))
	}
	if v := e.get(t, d.ID); v.Status != models.MigrationDraft || v.Version != nil {
		t.Errorf("migration changed: %+v", v)
	}
}
