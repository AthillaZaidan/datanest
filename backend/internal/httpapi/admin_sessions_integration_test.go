//go:build integration

package httpapi

import (
	"context"
	"github.com/AthillaZaidan/datanest/backend/internal/admin"
	"github.com/AthillaZaidan/datanest/backend/internal/auth"
	"github.com/AthillaZaidan/datanest/backend/internal/models"
	"github.com/AthillaZaidan/datanest/backend/internal/session"
	"github.com/AthillaZaidan/datanest/backend/internal/testutil"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAdminEffectsOnRealSessions(t *testing.T) {
	db := testutil.PlatformDB(t)
	a := testutil.CreateAdmin(t, db, "admin")
	u := testutil.CreateUser(t, db, "member")
	store := session.New(session.Config{IdleTimeout: time.Hour, AbsoluteTimeout: 8 * time.Hour, RotationInterval: 15 * time.Minute, GracePeriod: 30 * time.Second}, nil)
	defer store.Close()
	as := &auth.Service{DB: db, Sessions: store}
	svc := admin.New(db, store)
	r := NewRouter(Deps{Security: auth.ValidateStateRequest("http://localhost:5173"), RequireAuth: as.RequireAuth, RequireAdmin: as.RequireAdmin, Admin: svc, Me: as.Me})
	raw, sess, e := store.Create(a.ID)
	if e != nil {
		t.Fatal(e)
	}
	call := func(token bool, origin string, want int) {
		t.Helper()
		req := httptest.NewRequest("POST", "/api/admin/groups", strings.NewReader(`{"name":"group"}`))
		req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: raw})
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", origin)
		if token {
			req.Header.Set("X-CSRF-Token", sess.CSRFToken)
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != want {
			t.Fatalf("status %d: %s", w.Code, w.Body.String())
		}
	}
	call(false, "http://localhost:5173", 403)
	call(true, "https://evil.example", 403)
	call(true, "http://localhost:5173", 201)
	uraw, _, e := store.Create(u.ID)
	if e != nil {
		t.Fatal(e)
	}
	g := testutil.CreateGroup(t, db, "dev")
	if e = svc.Member(context.Background(), a.ID, g.ID, u.ID, true); e != nil {
		t.Fatal(e)
	}
	_, next, e := store.Validate(uraw)
	if e != nil || next == "" {
		t.Fatal("membership did not rotate session", e)
	}
	if e = svc.Role(context.Background(), a.ID, u.ID, models.RolePlatformAdmin, true); e != nil {
		t.Fatal(e)
	}
	_, newer, e := store.Validate(next)
	if e != nil || newer == "" {
		t.Fatal("role did not rotate session", e)
	}
	if e = svc.Role(context.Background(), a.ID, u.ID, models.RolePlatformAdmin, false); e != nil {
		t.Fatal(e)
	}
	for _, id := range []string{uraw, next, newer} {
		if _, _, e = store.Validate(id); e == nil {
			t.Fatal("revoked identifier accepted")
		}
	}
	uraw, _, _ = store.Create(u.ID)
	if e = svc.DeleteUser(context.Background(), a.ID, u.ID); e != nil {
		t.Fatal(e)
	}
	if _, _, e = store.Validate(uraw); e == nil {
		t.Fatal("deleted user session remains")
	}
}
