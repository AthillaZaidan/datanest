//go:build integration

package httpapi

import (
	"net/http"
	"testing"

	"github.com/AthillaZaidan/datanest/backend/internal/auth"
	"github.com/AthillaZaidan/datanest/backend/internal/session"
	"github.com/AthillaZaidan/datanest/backend/internal/testutil"
)

type outageEnv struct {
	*authEnv
	cut func()
}

func newOutageEnv(t *testing.T) outageEnv {
	t.Helper()
	db := testutil.PlatformDB(t)
	testutil.CreateUser(t, db, "ayu")
	c := newClock()
	store := session.New(db, testSessionCipher(t), defaultSessions(), c.Now)
	t.Cleanup(store.Close)
	s := &auth.Service{DB: db, Sessions: store}
	r := NewRouter(Deps{
		Security: auth.ValidateStateRequest(appOrigin), RequireAuth: s.RequireAuth, RequireAdmin: s.RequireAdmin,
		Login: s.Login, Logout: s.Logout, Me: s.Me,
	})
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	return outageEnv{authEnv: &authEnv{t: t, router: r, clock: c}, cut: func() { sqlDB.Close() }}
}

func TestLoginDuringADatabaseOutageIsAServerErrorNotAWrongPassword(t *testing.T) {
	e := newOutageEnv(t)
	e.cut()

	w, _, _ := e.login("ayu", testutil.Password)

	if w.Code != http.StatusInternalServerError || respCode(t, w) != "INTERNAL_ERROR" {
		t.Errorf("login = %d %s, want 500 INTERNAL_ERROR: the password was right, the database was not reachable", w.Code, respCode(t, w))
	}
}

func TestAWrongPasswordIsStillRejectedAsInvalidCredentials(t *testing.T) {
	e := newOutageEnv(t)

	w, _, _ := e.login("ayu", "not-the-password")

	if w.Code != http.StatusUnauthorized || respCode(t, w) != "INVALID_CREDENTIALS" {
		t.Errorf("login = %d %s, want 401 INVALID_CREDENTIALS", w.Code, respCode(t, w))
	}
	if w, _, _ := e.login("nobody", "whatever"); w.Code != http.StatusUnauthorized {
		t.Errorf("unknown user = %d, want 401", w.Code)
	}
}

func TestASessionCheckDuringADatabaseOutageDoesNotLogTheUserOut(t *testing.T) {
	e := newOutageEnv(t)
	_, sid, _ := e.login("ayu", testutil.Password)
	if sid == "" {
		t.Fatal("login did not return a session")
	}
	e.cut()

	w := e.do(authCall{method: "GET", path: "/api/auth/me", sid: sid})

	if w.Code != http.StatusInternalServerError {
		t.Errorf("GET /api/auth/me = %d %s, want 500: a 401 makes the SPA log the user out", w.Code, respCode(t, w))
	}
	if c := sessionCookie(w); c != nil && c.MaxAge < 0 {
		t.Error("the session cookie was cleared during an outage")
	}
}

func TestAnUnknownSessionIsStillUnauthenticated(t *testing.T) {
	e := newOutageEnv(t)

	w := e.do(authCall{method: "GET", path: "/api/auth/me", sid: "not-a-real-session"})

	if w.Code != http.StatusUnauthorized {
		t.Errorf("GET /api/auth/me = %d, want 401", w.Code)
	}
}
