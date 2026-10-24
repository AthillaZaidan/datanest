//go:build integration

package httpapi

import (
	"context"
	"encoding/json"
	"github.com/AthillaZaidan/datanest/backend/internal/admin"
	"github.com/AthillaZaidan/datanest/backend/internal/auth"
	"github.com/AthillaZaidan/datanest/backend/internal/authz"
	"github.com/AthillaZaidan/datanest/backend/internal/dbconn"
	"github.com/AthillaZaidan/datanest/backend/internal/testutil"
	"github.com/gin-gonic/gin"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type fakeSessions struct{}

func (fakeSessions) MarkNeedsRotation(uint) {}
func (fakeSessions) RevokeAllForUser(uint)  {}
func TestAdminHTTP(t *testing.T) {
	db := testutil.PlatformDB(t)
	a := testutil.CreateAdmin(t, db, "admin")
	u := testutil.CreateUser(t, db, "user")
	svc := admin.New(db, fakeSessions{})
	ci, _ := dbconn.NewCipher(make([]byte, 32))
	p := dbconn.NewPools(db, ci, time.Second)
	defer p.Close()
	ds := dbconn.New(db, ci, p, authz.New(db), time.Second)
	as := &auth.Service{DB: db}
	router := func(id uint) *gin.Engine {
		return NewRouter(Deps{RequireAuth: testutil.AsUser(id), RequireAdmin: as.RequireAdmin, Admin: svc, Databases: ds})
	}
	// Every registered admin method must reject a non-admin before handler processing.
	for _, rt := range contract {
		if !strings.HasPrefix(rt.path, "/api/admin/") {
			continue
		}
		w := httptest.NewRecorder()
		router(u.ID).ServeHTTP(w, httptest.NewRequest(rt.method, rt.path, nil))
		if w.Code != 403 {
			t.Fatalf("%s %s: %d", rt.method, rt.path, w.Code)
		}
	}
	r := router(a.ID)
	request := func(method, path, body string, want int) string {
		t.Helper()
		w := httptest.NewRecorder()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		if w.Code != want {
			t.Fatalf("%s %s: %d %s", method, path, w.Code, w.Body.String())
		}
		return w.Body.String()
	}
	body := request("POST", "/api/admin/users", `{"username":"created","password":"password123"}`, 201)
	if strings.Contains(body, "password") {
		t.Fatal("secret exposed")
	}
	request("GET", "/api/admin/users", "", 200)
	request("POST", "/api/admin/users", `{"username":"created","password":"password123"}`, 409)
	request("POST", "/api/admin/users", `{"username":"short","password":"x"}`, 400)
	request("POST", "/api/admin/groups", `{"name":"dev","description":"x"}`, 201)
	request("GET", "/api/admin/groups", "", 200)
	request("GET", "/api/admin/groups/1", "", 200)
	request("PATCH", "/api/admin/groups/1", `{"description":""}`, 200)
	request("PUT", "/api/admin/groups/1/members/2", "", 204)
	request("GET", "/api/admin/groups/1/members", "", 200)
	request("POST", "/api/admin/databases", `{"name":"demo","host":"127.0.0.1","port":1,"dbName":"test","username":"test","password":"secret","sslMode":"disable"}`, 201)
	request("GET", "/api/admin/databases", "", 200)
	request("GET", "/api/admin/databases/1", "", 200)
	request("PATCH", "/api/admin/databases/1", `{"password":"","name":"newname"}`, 200)
	request("PUT", "/api/admin/groups/1/grants/1", `{"level":"VIEW"}`, 204)
	request("PUT", "/api/admin/groups/1/grants/1", `{"level":"EDIT"}`, 204)
	request("GET", "/api/admin/groups/1/grants", "", 200)
	request("PUT", "/api/admin/groups/1/grants/1", `{"level":"ADMIN"}`, 400)
	rr := router(u.ID)
	w := httptest.NewRecorder()
	rr.ServeHTTP(w, httptest.NewRequest("GET", "/api/databases", nil))
	var list []dbconn.View
	if e := json.Unmarshal(w.Body.Bytes(), &list); e != nil || len(list) != 1 || list[0].MyLevel != "EDIT" {
		t.Fatal(w.Body.String(), e)
	}
	request("DELETE", "/api/admin/groups/1/grants/1", "", 204)
	request("DELETE", "/api/admin/groups/1/members/2", "", 204)
	if ok, e := authz.New(db).Can(context.Background(), u.ID, 1, authz.ViewData); e != nil || ok {
		t.Fatal("revocation ineffective", e)
	}
	request("DELETE", "/api/admin/databases/1", "", 204)
	request("DELETE", "/api/admin/groups/1", "", 204)
	request("GET", "/api/admin/groups/1", "", 404)
	request("POST", "/api/admin/groups", "{", 400)
}
