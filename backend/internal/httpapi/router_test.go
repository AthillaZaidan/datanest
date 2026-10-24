package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/AthillaZaidan/datanest/backend/internal/apperr"
)

func init() { gin.SetMode(gin.TestMode) }

var passthrough = func(c *gin.Context) { c.Next() }

// contract mirrors the API table in the PRD (§15).
var contract = []struct{ method, path string }{
	{"POST", "/api/auth/login"},
	{"POST", "/api/auth/logout"},
	{"GET", "/api/auth/me"},

	{"GET", "/api/admin/users"},
	{"POST", "/api/admin/users"},
	{"GET", "/api/admin/users/1"},
	{"PATCH", "/api/admin/users/1"},
	{"DELETE", "/api/admin/users/1"},
	{"PUT", "/api/admin/users/1/roles/platform_admin"},
	{"DELETE", "/api/admin/users/1/roles/platform_admin"},
	{"GET", "/api/admin/groups"},
	{"POST", "/api/admin/groups"},
	{"GET", "/api/admin/groups/1"},
	{"PATCH", "/api/admin/groups/1"},
	{"DELETE", "/api/admin/groups/1"},
	{"GET", "/api/admin/groups/1/members"},
	{"PUT", "/api/admin/groups/1/members/2"},
	{"DELETE", "/api/admin/groups/1/members/2"},
	{"GET", "/api/admin/groups/1/grants"},
	{"PUT", "/api/admin/groups/1/grants/3"},
	{"DELETE", "/api/admin/groups/1/grants/3"},
	{"GET", "/api/admin/databases"},
	{"POST", "/api/admin/databases"},
	{"POST", "/api/admin/databases/test"},
	{"GET", "/api/admin/databases/3"},
	{"PATCH", "/api/admin/databases/3"},
	{"DELETE", "/api/admin/databases/3"},
	{"POST", "/api/admin/databases/3/test"},

	{"GET", "/api/databases"},
	{"GET", "/api/databases/3"},
	{"GET", "/api/databases/3/schemas"},
	{"GET", "/api/databases/3/schemas/public/tables"},
	{"GET", "/api/databases/3/schemas/public/tables/products"},
	{"GET", "/api/databases/3/schemas/public/tables/products/rows"},
	{"GET", "/api/databases/3/migrations"},
	{"POST", "/api/databases/3/migrations"},
	{"GET", "/api/migrations/7"},
	{"PATCH", "/api/migrations/7"},
	{"DELETE", "/api/migrations/7"},
	{"POST", "/api/migrations/7/execute"},
	{"POST", "/api/migrations/7/duplicate"},
}

func do(r http.Handler, method, path string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(method, path, nil))
	return w
}

func TestContractRoutesRegistered(t *testing.T) {
	r := NewRouter(Deps{RequireAuth: passthrough, RequireAdmin: passthrough})

	for _, rt := range contract {
		w := do(r, rt.method, rt.path)
		if w.Code != http.StatusNotImplemented {
			t.Errorf("%s %s = %d, want 501", rt.method, rt.path, w.Code)
			continue
		}
		var body apperr.Body
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || body.Error.Code != "NOT_IMPLEMENTED" {
			t.Errorf("%s %s body = %s", rt.method, rt.path, w.Body.String())
		}
	}

	// contract + healthz; catches routes added without updating the contract.
	if got, want := len(r.Routes()), len(contract)+1; got != want {
		t.Errorf("router has %d routes, contract lists %d", got, want)
	}
}

func TestHealthz(t *testing.T) {
	w := do(NewRouter(Deps{}), "GET", "/api/healthz")
	if w.Code != http.StatusOK {
		t.Fatalf("healthz = %d", w.Code)
	}
}

func TestMiddlewareGuardsRoutes(t *testing.T) {
	deny := func(c *gin.Context) { RespondError(c, apperr.ErrUnauthenticated) }
	forbid := func(c *gin.Context) { RespondError(c, apperr.ErrForbidden) }
	r := NewRouter(Deps{RequireAuth: deny, RequireAdmin: forbid})

	if w := do(r, "POST", "/api/auth/login"); w.Code != http.StatusNotImplemented {
		t.Errorf("login must stay public, got %d", w.Code)
	}
	for _, p := range []string{"/api/auth/me", "/api/databases", "/api/migrations/7", "/api/admin/users"} {
		if w := do(r, "GET", p); w.Code != http.StatusUnauthorized {
			t.Errorf("GET %s = %d, want 401", p, w.Code)
		}
	}

	r = NewRouter(Deps{RequireAuth: passthrough, RequireAdmin: forbid})
	if w := do(r, "GET", "/api/admin/users"); w.Code != http.StatusForbidden {
		t.Errorf("admin route without admin = %d, want 403", w.Code)
	}
	if w := do(r, "GET", "/api/databases"); w.Code != http.StatusNotImplemented {
		t.Errorf("user route must not require admin, got %d", w.Code)
	}
}
