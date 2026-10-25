//go:build integration

package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/AthillaZaidan/datanest/backend/internal/admin"
	"github.com/AthillaZaidan/datanest/backend/internal/auth"
	"github.com/AthillaZaidan/datanest/backend/internal/authz"
	"github.com/AthillaZaidan/datanest/backend/internal/dbconn"
	"github.com/AthillaZaidan/datanest/backend/internal/explorer"
	"github.com/AthillaZaidan/datanest/backend/internal/migration"
	"github.com/AthillaZaidan/datanest/backend/internal/session"
	"github.com/AthillaZaidan/datanest/backend/internal/testutil"
)

const testOrigin = "http://localhost:8080"

type env struct {
	t      *testing.T
	router *gin.Engine
	dbID   uint
	admin  uint
}

func setup(t *testing.T) *env {
	t.Helper()
	gin.SetMode(gin.TestMode)
	platform := testutil.PlatformDB(t)
	target := testutil.TargetDB(t)
	if _, err := target.Exec(`CREATE TABLE produk (id int PRIMARY KEY, nama text NOT NULL); INSERT INTO produk VALUES (1, 'Notebook')`); err != nil {
		t.Fatal(err)
	}
	a := testutil.CreateAdmin(t, platform, "boss")
	editor := testutil.CreateUser(t, platform, "editor")
	viewer := testutil.CreateUser(t, platform, "viewer")
	conn := testutil.CreateDatabaseConnection(t, platform, "toko")
	ge, gv := testutil.CreateGroup(t, platform, "editors"), testutil.CreateGroup(t, platform, "viewers")
	testutil.AddMember(t, platform, ge.ID, editor.ID)
	testutil.AddMember(t, platform, gv.ID, viewer.ID)
	testutil.GrantAccess(t, platform, ge.ID, conn.ID, authz.Edit)
	testutil.GrantAccess(t, platform, gv.ID, conn.ID, authz.View)

	store := session.New(session.Config{IdleTimeout: time.Hour, AbsoluteTimeout: 8 * time.Hour, RotationInterval: time.Hour - time.Minute, GracePeriod: time.Second}, nil)
	t.Cleanup(store.Close)
	cipher, _ := dbconn.NewCipher(make([]byte, 32))
	az := authz.New(platform)
	provider := testutil.StaticProvider{DB: target}
	r := gin.New()
	r.Use(auth.ValidateStateRequest(testOrigin))
	Register(r, &Handler{
		Auth:       &auth.Service{DB: platform, Sessions: store},
		Admin:      admin.New(platform, store),
		Databases:  dbconn.New(platform, cipher, dbconn.NewPools(platform, cipher, 5*time.Second), az, 5*time.Second),
		Explorer:   explorer.New(platform, provider, az, 5*time.Second),
		Migrations: migration.New(platform, provider, az, migration.Options{LockTimeout: time.Second, StatementTimeout: 5 * time.Second, ExecTimeout: 10 * time.Second}),
	})
	return &env{t: t, router: r, dbID: conn.ID, admin: a.ID}
}

type client struct {
	e    *env
	sid  string
	csrf string
}

func (e *env) do(method, path, sid string, form url.Values) *httptest.ResponseRecorder {
	e.t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(form.Encode()))
	if method == http.MethodPost {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Origin", testOrigin)
	}
	if sid != "" {
		req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: sid})
	}
	w := httptest.NewRecorder()
	e.router.ServeHTTP(w, req)
	return w
}

var csrfField = regexp.MustCompile(`name="csrf_token" value="([^"]+)"`)

func (e *env) login(username string) *client {
	e.t.Helper()
	w := e.do("POST", "/login", "", url.Values{"username": {username}, "password": {testutil.Password}})
	if w.Code != http.StatusSeeOther {
		e.t.Fatalf("login %s: %d %s", username, w.Code, w.Body.String())
	}
	c := &client{e: e}
	for _, k := range w.Result().Cookies() {
		if k.Name == auth.CookieName {
			c.sid = k.Value
		}
	}
	page := c.get("/databases")
	m := csrfField.FindStringSubmatch(page.Body.String())
	if m == nil {
		e.t.Fatal("no csrf token on page")
	}
	c.csrf = m[1]
	return c
}

func (c *client) get(path string) *httptest.ResponseRecorder { return c.e.do("GET", path, c.sid, nil) }

func (c *client) post(path string, form url.Values) *httptest.ResponseRecorder {
	if form == nil {
		form = url.Values{}
	}
	form.Set("csrf_token", c.csrf)
	return c.e.do("POST", path, c.sid, form)
}

func itoa(n uint) string { return strconv.FormatUint(uint64(n), 10) }

func contains(t *testing.T, w *httptest.ResponseRecorder, want ...string) {
	t.Helper()
	for _, s := range want {
		if !strings.Contains(w.Body.String(), s) {
			t.Fatalf("page (%d) lacks %q", w.Code, s)
		}
	}
}

func TestAnonymousIsSentToLogin(t *testing.T) {
	e := setup(t)
	if w := e.do("GET", "/databases", "", nil); w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/login" {
		t.Fatalf("%d %s", w.Code, w.Header().Get("Location"))
	}
}

func TestLoginForm(t *testing.T) {
	e := setup(t)
	w := e.do("POST", "/login", "", url.Values{"username": {"editor"}, "password": {"wrong-password"}})
	if w.Code != http.StatusUnauthorized || !strings.Contains(w.Body.String(), "Username atau password salah.") {
		t.Fatalf("wrong password: %d", w.Code)
	}
	if w := e.do("POST", "/login", "", url.Values{"username": {"editor"}, "password": {testutil.Password}}); w.Code != http.StatusSeeOther {
		t.Fatalf("login: %d", w.Code)
	}
}

func TestFormWithoutCSRFIsRejected(t *testing.T) {
	e := setup(t)
	c := e.login("editor")
	w := e.do("POST", "/databases/"+itoa(e.dbID)+"/migrations", c.sid, url.Values{"sql": {"SELECT 1"}})
	if w.Code != http.StatusForbidden {
		t.Fatalf("missing token: %d", w.Code)
	}
}

func TestExplorerPages(t *testing.T) {
	e := setup(t)
	c := e.login("viewer")
	contains(t, c.get("/databases"), "toko", "VIEW")
	base := "/databases/" + itoa(e.dbID)
	contains(t, c.get(base+"?schema=public&table=produk"), "produk", "nama", "PK")
	contains(t, c.get(base+"?schema=public&table=produk&tab=data"), "Notebook", "1–1")
}

func TestViewerCannotWriteMigrations(t *testing.T) {
	e := setup(t)
	c := e.login("viewer")
	base := "/databases/" + itoa(e.dbID)
	if strings.Contains(c.get(base).Body.String(), "Migration baru") {
		t.Fatal("viewer sees the new migration button")
	}
	if w := c.post(base+"/migrations", url.Values{"sql": {"CREATE TABLE x (id int)"}}); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "Anda tidak memiliki akses.") {
		t.Fatalf("viewer created a migration: %d", w.Code)
	}
}

func TestEditorRunsMigration(t *testing.T) {
	e := setup(t)
	c := e.login("editor")
	base := "/databases/" + itoa(e.dbID)
	contains(t, c.get(base+"/migrations/new"), "Migration baru", `name="sql"`)
	w := c.post(base+"/migrations", url.Values{"sql": {"ALTER TABLE produk ADD COLUMN sku text;"}})
	if w.Code != http.StatusSeeOther {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	loc := w.Header().Get("Location")
	contains(t, c.get(loc), "Draft", "Jalankan", "ALTER TABLE produk")
	if w := c.post(loc+"/execute", nil); w.Code != http.StatusSeeOther {
		t.Fatalf("execute: %d", w.Code)
	}
	contains(t, c.get(loc), "Migration v1", "Sukses")
	contains(t, c.get(base+"/migrations"), "v1", "Sukses", "Riwayat migration")
	contains(t, c.get(base+"?schema=public&table=produk"), "sku")
}

func TestAdminPagesRequireAdmin(t *testing.T) {
	e := setup(t)
	c := e.login("editor")
	for _, p := range []string{"/admin/users", "/admin/groups", "/admin/databases"} {
		if w := c.get(p); w.Code != http.StatusForbidden {
			t.Fatalf("%s: %d", p, w.Code)
		}
	}
}

func TestAdminManagesUsersGroupsAndGrants(t *testing.T) {
	e := setup(t)
	c := e.login("boss")
	if w := c.post("/admin/users", url.Values{"username": {"citra"}, "password": {"password123"}}); w.Code != http.StatusSeeOther {
		t.Fatalf("create user: %d", w.Code)
	}
	contains(t, c.get("/admin/users"), "citra")
	if w := c.post("/admin/groups", url.Values{"name": {"auditors"}, "description": {"Tim audit"}}); w.Code != http.StatusSeeOther {
		t.Fatalf("create group: %d", w.Code)
	}
	page := c.get("/admin/groups")
	contains(t, page, "auditors", "Tim audit")
	gid := regexp.MustCompile(`/admin/groups/(\d+)/delete`).FindAllStringSubmatch(page.Body.String(), -1)
	var auditors string
	for _, m := range gid {
		if strings.Contains(c.get("/admin/groups?group="+m[1]).Body.String(), "Anggota auditors") {
			auditors = m[1]
		}
	}
	if auditors == "" {
		t.Fatal("group detail not found")
	}
	users := c.get("/admin/users").Body.String()
	uid := regexp.MustCompile(`/admin/users/(\d+)/delete" [^>]*data-name="citra"`).FindStringSubmatch(users)
	if uid == nil {
		t.Fatal("user row not found")
	}
	if w := c.post("/admin/groups/"+auditors+"/members", url.Values{"uid": {uid[1]}}); w.Code != http.StatusSeeOther {
		t.Fatalf("add member: %d", w.Code)
	}
	if w := c.post("/admin/groups/"+auditors+"/grants", url.Values{"did": {itoa(e.dbID)}, "level": {"VIEW"}}); w.Code != http.StatusSeeOther {
		t.Fatalf("grant: %d", w.Code)
	}
	contains(t, c.get("/admin/groups?group="+auditors), "citra", "toko", "VIEW")
	contains(t, c.get("/admin/databases"), "toko", "Tidak ada") // testutil stores the connection without a password
}

func TestLastAdminIsKept(t *testing.T) {
	e := setup(t)
	c := e.login("boss")
	w := c.post("/admin/users/"+itoa(e.admin)+"/role", url.Values{"add": {"0"}})
	if w.Code != http.StatusSeeOther || !strings.Contains(w.Header().Get("Location"), url.QueryEscape("minimal satu Platform Admin")) {
		t.Fatalf("%d %s", w.Code, w.Header().Get("Location"))
	}
}

func TestLogoutEndsSession(t *testing.T) {
	e := setup(t)
	c := e.login("editor")
	if w := c.post("/logout", nil); w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/login" {
		t.Fatalf("logout: %d", w.Code)
	}
	if w := c.get("/databases"); w.Code != http.StatusSeeOther {
		t.Fatalf("old session still works: %d", w.Code)
	}
}

func TestExplainFromMigrationPage(t *testing.T) {
	e := setup(t)
	editor := e.login("editor")
	w := editor.post("/databases/"+itoa(e.dbID)+"/migrations", url.Values{"sql": {"CREATE TABLE extra (id int);\nSELECT * FROM produk WHERE id = 1;"}})
	loc := w.Header().Get("Location")
	contains(t, editor.get(loc), "Analisis")

	viewer := e.login("viewer")
	plain := viewer.post(loc+"/explain", nil)
	if plain.Code != http.StatusOK {
		t.Fatalf("explain: %d", plain.Code)
	}
	contains(t, plain, "Hasil analisis", "tidak dapat dianalisis", "produk", "Cost")
	if strings.Contains(plain.Body.String(), "Waktu aktual") {
		t.Fatal("plain EXPLAIN shows actual times")
	}
	contains(t, viewer.post(loc+"/explain", url.Values{"analyze": {"1"}}), "Anda tidak memiliki akses.")
	contains(t, editor.post(loc+"/explain", url.Values{"analyze": {"1"}}), "Hasil analisis", "Waktu aktual", "di-rollback")
	contains(t, editor.get("/databases/"+itoa(e.dbID)), "produk")
	if strings.Contains(editor.get("/databases/"+itoa(e.dbID)).Body.String(), ">extra<") {
		t.Fatal("explain kept the DDL")
	}
}

func TestUnknownPageRendersFully(t *testing.T) {
	e := setup(t)
	anon := e.do("GET", "/nope", "", nil)
	if anon.Code != http.StatusNotFound || !strings.Contains(anon.Body.String(), "</html>") {
		t.Fatalf("anonymous 404 cut short: %d", anon.Code)
	}
	c := e.login("editor")
	w := c.get("/nope")
	if w.Code != http.StatusNotFound || !strings.Contains(w.Body.String(), "</html>") || !strings.Contains(w.Body.String(), "editor") {
		t.Fatalf("signed-in 404: %d", w.Code)
	}
}
