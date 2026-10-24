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
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/AthillaZaidan/datanest/backend/internal/authz"
	"github.com/AthillaZaidan/datanest/backend/internal/migration"
	"github.com/AthillaZaidan/datanest/backend/internal/models"
	"github.com/AthillaZaidan/datanest/backend/internal/testutil"
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
	r := call(e.as(e.editor), "POST", "/api/databases/"+itoa(e.dbA)+"/migrations", map[string]string{"sql": sql})
	if r.Code != 201 {
		t.Fatalf("create draft = %d %s", r.Code, r.Body)
	}
	return r.view(t)
}

func (e migEnv) execute(t *testing.T, u models.User, id uint) resp {
	t.Helper()
	return call(e.as(u), "POST", "/api/migrations/"+itoa(id)+"/execute", nil)
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

func TestMigrationExecuteSuccess(t *testing.T) {
	e := setupMigrations(t)
	d := e.draft(t, `
		CREATE TABLE products (id serial PRIMARY KEY, name text);
		CREATE FUNCTION product_count() RETURNS bigint AS $$
			SELECT count(*) FROM products; -- semicolon inside a function body
		$$ LANGUAGE sql;
		INSERT INTO products (name) VALUES ('a'), ('b');`)

	r := e.execute(t, e.editor, d.ID)
	v := r.view(t)
	if r.Code != 200 || v.Status != models.MigrationSuccess || v.Version == nil || *v.Version != 1 || v.ExecutedBy != "editor" || v.FinishedAt == nil {
		t.Fatalf("execute = %d %+v", r.Code, v)
	}
	var n int
	if err := e.target.QueryRow(`SELECT product_count()`).Scan(&n); err != nil || n != 2 {
		t.Fatalf("product_count = %d, %v", n, err)
	}
	if err := e.target.QueryRow(`SELECT count(*) FROM _platform_migrations WHERE version = 1`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("ledger row = %d, %v", n, err)
	}

	d2 := e.draft(t, "ALTER TABLE products ADD COLUMN price int")
	if d2.BaseVersion == nil || *d2.BaseVersion != 1 || d2.BaseOutdated {
		t.Errorf("new draft should record base version 1: %+v", d2)
	}
	if v := e.execute(t, e.editor, d2.ID).view(t); *v.Version != 2 {
		t.Errorf("second version = %v", v.Version)
	}

	path := "/api/migrations/" + itoa(d.ID)
	if r := call(e.as(e.editor), "PATCH", path, map[string]any{"sql": "SELECT 1", "revision": 1}); r.errCode() != "MIGRATION_NOT_DRAFT" {
		t.Errorf("edit executed = %d %s", r.Code, r.errCode())
	}
	if r := call(e.as(e.editor), "DELETE", path, nil); r.errCode() != "MIGRATION_NOT_DRAFT" {
		t.Errorf("delete executed = %d %s", r.Code, r.errCode())
	}
	if r := e.execute(t, e.editor, d.ID); r.errCode() != "MIGRATION_NOT_DRAFT" {
		t.Errorf("re-execute = %d %s", r.Code, r.errCode())
	}
	if r := e.execute(t, e.viewer, d2.ID); r.Code != 403 {
		t.Errorf("viewer execute = %d", r.Code)
	}

	var listed migration.ListResult
	json.Unmarshal(call(e.as(e.viewer), "GET", "/api/databases/"+itoa(e.dbA)+"/migrations", nil).Body, &listed)
	if listed.CurrentVersion != 2 || *listed.Migrations[0].Version != 2 {
		t.Errorf("history = %+v", listed)
	}
}

func TestMigrationExecuteFailureRollsBack(t *testing.T) {
	e := setupMigrations(t)
	d := e.draft(t, "CREATE TABLE half (id int); INSERT INTO missing VALUES (1);")

	r := e.execute(t, e.editor, d.ID)
	v := r.view(t)
	if r.Code != 200 || v.Status != models.MigrationFailed || v.ErrorCode != "42P01" || !strings.Contains(v.ErrorMessage, "missing") {
		t.Fatalf("failed execute = %d %+v", r.Code, v)
	}
	if e.tableExists(t, "half") {
		t.Error("first statement must be rolled back")
	}
	var n int
	e.target.QueryRow(`SELECT count(*) FROM _platform_migrations`).Scan(&n)
	if n != 0 {
		t.Errorf("ledger rows = %d after failure", n)
	}

	r = call(e.as(e.editor), "POST", "/api/migrations/"+itoa(d.ID)+"/duplicate", nil)
	dup := r.view(t)
	if r.Code != 201 || dup.Status != models.MigrationDraft || dup.SQL != d.SQL {
		t.Fatalf("duplicate = %d %+v", r.Code, dup)
	}

	ok := e.draft(t, "CREATE TABLE fine (id int)")
	if v := e.execute(t, e.editor, ok.ID).view(t); *v.Version != 2 {
		t.Errorf("version after failed v1 = %d, want 2", *v.Version)
	}
}

func TestMigrationNotInTransaction(t *testing.T) {
	e := setupMigrations(t)
	if _, err := e.target.Exec(`CREATE TABLE t (id int)`); err != nil {
		t.Fatal(err)
	}
	d := e.draft(t, "CREATE INDEX CONCURRENTLY idx_t ON t (id)")
	v := e.execute(t, e.editor, d.ID).view(t)
	if v.Status != models.MigrationFailed || v.ErrorCode != "25001" || !strings.Contains(v.ErrorMessage, "dalam transaction") {
		t.Fatalf("concurrent index = %+v", v)
	}
}

func TestMigrationIDOR(t *testing.T) {
	e := setupMigrations(t)
	d := e.draft(t, "CREATE TABLE secret (id int)")
	other := e.as(e.otherEditor) // EDIT on database B only

	if r := e.execute(t, e.otherEditor, d.ID); r.Code != 403 {
		t.Errorf("execute foreign migration = %d", r.Code)
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

func TestMigrationConcurrency(t *testing.T) {
	e := setupMigrations(t)
	if _, err := e.target.Exec(`CREATE TABLE counter (n int)`); err != nil {
		t.Fatal(err)
	}

	slow := e.draft(t, "SELECT pg_sleep(1.5); CREATE TABLE slow (id int);")
	fast := e.draft(t, "CREATE TABLE fast (id int)")
	done := make(chan resp)
	go func() { done <- e.execute(t, e.editor, slow.ID) }()
	time.Sleep(300 * time.Millisecond)
	if r := e.execute(t, e.editor, fast.ID); r.Code != 409 || r.errCode() != "MIGRATION_RUNNING" {
		t.Errorf("execute while running = %d %s", r.Code, r.errCode())
	}
	if v := (<-done).view(t); v.Status != models.MigrationSuccess {
		t.Fatalf("slow migration = %+v", v)
	}

	once := e.draft(t, "INSERT INTO counter VALUES (1)")
	var wg sync.WaitGroup
	codes := make([]int, 2)
	for i := range codes {
		wg.Add(1)
		go func() {
			defer wg.Done()
			codes[i] = e.execute(t, e.editor, once.ID).Code
		}()
	}
	wg.Wait()
	var n int
	e.target.QueryRow(`SELECT count(*) FROM counter`).Scan(&n)
	if n != 1 {
		t.Errorf("double execute ran the SQL %d times (codes %v)", n, codes)
	}
	if !(codes[0] == 200 && codes[1] == 409 || codes[0] == 409 && codes[1] == 200) {
		t.Errorf("double execute codes = %v, want one 200 and one 409", codes)
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
	v := e.execute(t, e.editor, d.ID).view(t)
	if v.Status != models.MigrationFailed || v.ErrorCode != "55P03" {
		t.Fatalf("locked table = %+v", v)
	}
	if time.Since(start) > 4*time.Second {
		t.Errorf("lock_timeout not applied, took %v", time.Since(start))
	}
}

func TestMigrationReconcile(t *testing.T) {
	e := setupMigrations(t)
	if _, err := e.target.Exec(`CREATE TABLE _platform_migrations (version integer PRIMARY KEY, applied_at timestamptz NOT NULL DEFAULT now());
		INSERT INTO _platform_migrations (version) VALUES (5)`); err != nil {
		t.Fatal(err)
	}
	five, six := 5, 6
	committed := models.Migration{DatabaseID: e.dbA, Version: &five, SQL: "SELECT 1", Status: models.MigrationRunning, CreatedByUsername: "editor"}
	lost := models.Migration{DatabaseID: e.dbA, Version: &six, SQL: "SELECT 1", Status: models.MigrationRunning, CreatedByUsername: "editor"}
	e.platform.Create(&committed)
	e.platform.Create(&lost)

	if err := e.svc.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	e.platform.First(&committed, committed.ID)
	e.platform.First(&lost, lost.ID)
	if committed.Status != models.MigrationSuccess {
		t.Errorf("committed = %s, want success", committed.Status)
	}
	if lost.Status != models.MigrationFailed || lost.ErrorMessage == "" {
		t.Errorf("lost = %s %q, want failed with message", lost.Status, lost.ErrorMessage)
	}
}

func TestMigrationDeletedExecutor(t *testing.T) {
	e := setupMigrations(t)
	d := e.draft(t, "CREATE TABLE kept (id int)")
	e.execute(t, e.editor, d.ID)

	if err := e.platform.Delete(&e.editor).Error; err != nil {
		t.Fatal(err)
	}
	v := call(e.as(e.viewer), "GET", "/api/migrations/"+itoa(d.ID), nil).view(t)
	if v.ExecutedBy != "editor (deleted)" || v.CreatedBy != "editor (deleted)" || *v.Version != 1 {
		t.Errorf("after deleting executor = %+v", v)
	}
}
