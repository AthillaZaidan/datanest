//go:build integration

package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/AthillaZaidan/datanest/backend/internal/authz"
	"github.com/AthillaZaidan/datanest/backend/internal/explorer"
	"github.com/AthillaZaidan/datanest/backend/internal/testutil"
)

// fakeAuthz grants each user a fixed level on every database.
type fakeAuthz map[uint]authz.Level

func (f fakeAuthz) Can(_ context.Context, userID, _ uint, a authz.Action) (bool, error) {
	need, ok := authz.RequiredLevel(a)
	return ok && f[userID] >= need, nil
}

const explorerFixture = `
	CREATE TABLE products (
		id serial PRIMARY KEY,
		name text NOT NULL UNIQUE,
		price int CHECK (price >= 0),
		data bytea,
		meta jsonb DEFAULT '{}'
	);
	CREATE INDEX idx_products_price ON products (price);
	CREATE TABLE orders (id int PRIMARY KEY, product_id int REFERENCES products (id), qty int NOT NULL);
	CREATE TABLE logs (msg text, payload json);
	CREATE SCHEMA sales;
	CREATE TABLE sales.regions (code text PRIMARY KEY);
	CREATE TABLE _platform_migrations (version int PRIMARY KEY);
	INSERT INTO products (name, price, data, meta)
		SELECT 'p' || g, g, '\xdead'::bytea, jsonb_build_object('n', g) FROM generate_series(1, 30) g;
	INSERT INTO logs VALUES ('a', '{"x":1}'), ('b', NULL);`

type explorerEnv struct {
	target           *sql.DB
	dbID             uint
	viewer, outsider uint
	router           func(userID uint) http.Handler
}

func setupExplorer(t *testing.T) explorerEnv {
	t.Helper()
	platform := testutil.PlatformDB(t)
	target := testutil.TargetDB(t)
	if _, err := target.Exec(explorerFixture); err != nil {
		t.Fatalf("fixture: %v", err)
	}
	viewer := testutil.CreateUser(t, platform, "viewer")
	outsider := testutil.CreateUser(t, platform, "outsider")
	conn := testutil.CreateDatabaseConnection(t, platform, "toko")

	svc := explorer.New(platform, testutil.StaticProvider{DB: target},
		fakeAuthz{viewer.ID: authz.View, outsider.ID: authz.None}, 5*time.Second)

	return explorerEnv{
		target:   target,
		dbID:     conn.ID,
		viewer:   viewer.ID,
		outsider: outsider.ID,
		router: func(userID uint) http.Handler {
			return NewRouter(Deps{RequireAuth: testutil.AsUser(userID), Explorer: svc})
		},
	}
}

func getJSON(t *testing.T, h http.Handler, path string, out any) int {
	t.Helper()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
	if out != nil && w.Code == http.StatusOK {
		if err := json.Unmarshal(w.Body.Bytes(), out); err != nil {
			t.Fatalf("decode %s: %v (%s)", path, err, w.Body.String())
		}
	}
	return w.Code
}

func errorCode(t *testing.T, h http.Handler, path string) (int, string) {
	t.Helper()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
	var body struct{ Error struct{ Code string } }
	json.Unmarshal(w.Body.Bytes(), &body)
	return w.Code, body.Error.Code
}

func TestExplorerSchemasAndTables(t *testing.T) {
	env := setupExplorer(t)
	h := env.router(env.viewer)
	base := "/api/databases/" + itoa(env.dbID)

	var schemas struct{ Schemas []explorer.Schema }
	if code := getJSON(t, h, base+"/schemas", &schemas); code != 200 {
		t.Fatalf("schemas = %d", code)
	}
	names := map[string]bool{}
	for _, s := range schemas.Schemas {
		names[s.Name] = true
	}
	if !names["public"] || !names["sales"] || names["pg_catalog"] || names["information_schema"] {
		t.Errorf("schemas = %v", schemas.Schemas)
	}

	var tables struct{ Tables []explorer.Table }
	getJSON(t, h, base+"/schemas/public/tables", &tables)
	got := []string{}
	for _, tb := range tables.Tables {
		got = append(got, tb.Name)
	}
	if want := []string{"logs", "orders", "products"}; !equal(got, want) {
		t.Errorf("tables = %v, want %v (ledger table must be hidden)", got, want)
	}

	if code, ec := errorCode(t, h, base+"/schemas/nope/tables"); code != 404 || ec != "NOT_FOUND" {
		t.Errorf("unknown schema = %d %s", code, ec)
	}
	if code, _ := errorCode(t, h, base+"/schemas/pg_catalog/tables"); code != 404 {
		t.Errorf("system schema must be hidden, got %d", code)
	}
}

func TestExplorerTableDetail(t *testing.T) {
	env := setupExplorer(t)
	h := env.router(env.viewer)
	base := "/api/databases/" + itoa(env.dbID) + "/schemas/public/tables/"

	var products explorer.TableDetail
	if code := getJSON(t, h, base+"products", &products); code != 200 {
		t.Fatalf("products = %d", code)
	}
	if len(products.Columns) != 5 || products.Columns[0].Name != "id" || products.Columns[0].DataType != "integer" {
		t.Errorf("columns = %+v", products.Columns)
	}
	if products.Columns[1].Nullable || !products.Columns[2].Nullable || products.Columns[4].Default == nil {
		t.Errorf("nullable/default wrong: %+v", products.Columns)
	}
	types := map[string]bool{}
	for _, c := range products.Constraints {
		types[c.Type] = true
	}
	for _, want := range []string{"PRIMARY KEY", "UNIQUE", "CHECK"} {
		if !types[want] {
			t.Errorf("missing %s constraint in %+v", want, products.Constraints)
		}
	}
	idx := map[string]bool{}
	for _, i := range products.Indexes {
		idx[i.Name] = true
	}
	if !idx["idx_products_price"] || !idx["products_pkey"] {
		t.Errorf("indexes = %+v", products.Indexes)
	}

	var orders explorer.TableDetail
	getJSON(t, h, base+"orders", &orders)
	var fk *explorer.Constraint
	for i := range orders.Constraints {
		if orders.Constraints[i].Type == "FOREIGN KEY" {
			fk = &orders.Constraints[i]
		}
	}
	if fk == nil || fk.References == nil || fk.References.Table != "products" ||
		!equal(fk.Columns, []string{"product_id"}) || !equal(fk.References.Columns, []string{"id"}) {
		t.Errorf("foreign key = %+v", fk)
	}

	for _, name := range []string{"_platform_migrations", "missing", "products;%20DROP%20TABLE%20orders"} {
		if code, _ := errorCode(t, h, base+name); code != 404 {
			t.Errorf("%s = %d, want 404", name, code)
		}
	}
	if _, err := env.target.Exec(`SELECT 1 FROM orders`); err != nil {
		t.Errorf("orders must survive the injection attempt: %v", err)
	}
}

func TestExplorerRows(t *testing.T) {
	env := setupExplorer(t)
	h := env.router(env.viewer)
	base := "/api/databases/" + itoa(env.dbID) + "/schemas/public/tables/"

	var rows explorer.Rows
	if code := getJSON(t, h, base+"products/rows?page=2&pageSize=10", &rows); code != 200 {
		t.Fatalf("rows = %d", code)
	}
	if rows.TotalRows != 30 || rows.Page != 2 || rows.PageSize != 10 || len(rows.Rows) != 10 {
		t.Fatalf("rows meta = %+v", rows)
	}
	if !equal(rows.Columns, []string{"id", "name", "price", "data", "meta"}) {
		t.Errorf("columns = %v", rows.Columns)
	}
	first := rows.Rows[0]
	if first[0] != float64(11) || first[3] != `\xdead` {
		t.Errorf("first row = %v (want id 11, bytea as hex)", first)
	}
	if meta, ok := first[4].(map[string]any); !ok || meta["n"] != float64(11) {
		t.Errorf("jsonb = %#v", first[4])
	}

	getJSON(t, h, base+"products/rows?pageSize=500", &rows)
	if rows.PageSize != explorer.MaxPageSize || len(rows.Rows) != 30 {
		t.Errorf("clamp = %d rows=%d", rows.PageSize, len(rows.Rows))
	}

	var logs explorer.Rows
	if code := getJSON(t, h, base+"logs/rows", &logs); code != 200 || logs.TotalRows != 2 {
		t.Errorf("table without primary key = %d %+v", code, logs)
	}
}

func TestExplorerAccess(t *testing.T) {
	env := setupExplorer(t)
	base := "/api/databases/" + itoa(env.dbID)

	if code, ec := errorCode(t, env.router(env.outsider), base+"/schemas"); code != 403 || ec != "FORBIDDEN" {
		t.Errorf("outsider = %d %s, want 403", code, ec)
	}
	if code, _ := errorCode(t, env.router(env.outsider), base+"/schemas/public/tables/products/rows"); code != 403 {
		t.Errorf("outsider rows = %d, want 403", code)
	}
	if code, ec := errorCode(t, env.router(env.viewer), "/api/databases/999/schemas"); code != 404 || ec != "NOT_FOUND" {
		t.Errorf("missing database = %d %s", code, ec)
	}
	if code, _ := errorCode(t, env.router(env.viewer), "/api/databases/abc/schemas"); code != 404 {
		t.Errorf("non-numeric id = %d", code)
	}

	noUser := NewRouter(Deps{RequireAuth: func(c *gin.Context) { c.Next() }, Explorer: explorer.New(nil, nil, nil, time.Second)})
	if code, ec := errorCode(t, noUser, base+"/schemas"); code != 401 || ec != "UNAUTHENTICATED" {
		t.Errorf("no user = %d %s", code, ec)
	}
}

func TestExplorerTargetUnavailable(t *testing.T) {
	platform := testutil.PlatformDB(t)
	viewer := testutil.CreateUser(t, platform, "viewer")
	conn := testutil.CreateDatabaseConnection(t, platform, "down")

	dead, _ := sql.Open("pgx", "postgres://nobody:x@127.0.0.1:1/none?connect_timeout=1")
	defer dead.Close()
	svc := explorer.New(platform, testutil.StaticProvider{DB: dead}, fakeAuthz{viewer.ID: authz.View}, 3*time.Second)
	h := NewRouter(Deps{RequireAuth: testutil.AsUser(viewer.ID), Explorer: svc})

	if code, ec := errorCode(t, h, "/api/databases/"+itoa(conn.ID)+"/schemas"); code != 502 || ec != "TARGET_DB_UNAVAILABLE" {
		t.Errorf("dead target = %d %s, want 502", code, ec)
	}
}

func itoa(n uint) string { return strconv.FormatUint(uint64(n), 10) }

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
