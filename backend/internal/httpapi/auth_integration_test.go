//go:build integration

package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/AthillaZaidan/datanest/backend/internal/auth"
	"github.com/AthillaZaidan/datanest/backend/internal/session"
	"github.com/AthillaZaidan/datanest/backend/internal/testutil"
)

const appOrigin = "http://localhost:5173"

// clock is a manual time source so session timeouts can be crossed without waiting.
type clock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *clock) Now() time.Time      { c.mu.Lock(); defer c.mu.Unlock(); return c.now }
func (c *clock) Add(d time.Duration) { c.mu.Lock(); c.now = c.now.Add(d); c.mu.Unlock() }
func newClock() *clock               { return &clock{now: time.Date(2026, 10, 25, 9, 0, 0, 0, time.UTC)} }

type authEnv struct {
	t      *testing.T
	router http.Handler
	clock  *clock
}

func newAuthEnv(t *testing.T, cfg session.Config) *authEnv {
	t.Helper()
	db := testutil.PlatformDB(t)
	testutil.CreateUser(t, db, "ayu")
	c := newClock()
	store := session.New(cfg, c.Now)
	t.Cleanup(store.Close)
	s := &auth.Service{DB: db, Sessions: store}
	r := NewRouter(Deps{
		Security: auth.ValidateStateRequest(appOrigin), RequireAuth: s.RequireAuth, RequireAdmin: s.RequireAdmin,
		Login: s.Login, Logout: s.Logout, Me: s.Me,
	})
	return &authEnv{t: t, router: r, clock: c}
}

func defaultSessions() session.Config {
	return session.Config{IdleTimeout: 30 * time.Minute, AbsoluteTimeout: 8 * time.Hour, RotationInterval: 15 * time.Minute, GracePeriod: 30 * time.Second}
}

type authCall struct {
	method, path, body, sid, csrf, origin, contentType string
}

func (e *authEnv) do(c authCall) *httptest.ResponseRecorder {
	e.t.Helper()
	req := httptest.NewRequest(c.method, c.path, strings.NewReader(c.body))
	if c.contentType == "" {
		c.contentType = "application/json"
	}
	req.Header.Set("Content-Type", c.contentType)
	if c.origin != "" {
		req.Header.Set("Origin", c.origin)
	}
	if c.sid != "" {
		req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: c.sid})
	}
	if c.csrf != "" {
		req.Header.Set("X-CSRF-Token", c.csrf)
	}
	w := httptest.NewRecorder()
	e.router.ServeHTTP(w, req)
	return w
}

func sessionCookie(w *httptest.ResponseRecorder) *http.Cookie {
	for _, c := range w.Result().Cookies() {
		if c.Name == auth.CookieName {
			return c
		}
	}
	return nil
}

func (e *authEnv) login(username, password string) (*httptest.ResponseRecorder, string, string) {
	e.t.Helper()
	w := e.do(authCall{method: "POST", path: "/api/auth/login", body: `{"username":"` + username + `","password":"` + password + `"}`, origin: appOrigin})
	var body struct {
		CSRFToken string `json:"csrfToken"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	sid := ""
	if c := sessionCookie(w); c != nil {
		sid = c.Value
	}
	return w, sid, body.CSRFToken
}

func respCode(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	return body.Error.Code
}

func TestLoginSuccess(t *testing.T) {
	e := newAuthEnv(t, defaultSessions())
	w, sid, csrf := e.login("ayu", testutil.Password)
	if w.Code != 200 || sid == "" || csrf == "" {
		t.Fatalf("login: %d %s", w.Code, w.Body.String())
	}
	c := sessionCookie(w)
	if !c.HttpOnly || c.SameSite != http.SameSiteStrictMode || c.Path != "/" || c.Secure {
		t.Fatalf("cookie attributes %+v", c)
	}
	if strings.Contains(w.Body.String(), sid) || strings.Contains(w.Body.String(), testutil.Password) {
		t.Fatal("response body leaks the session identifier or password")
	}
	if !strings.Contains(w.Body.String(), `"username":"ayu"`) || !strings.Contains(w.Body.String(), `"isAdmin":false`) {
		t.Fatal(w.Body.String())
	}
	me := e.do(authCall{method: "GET", path: "/api/auth/me", sid: sid})
	if me.Code != 200 || !strings.Contains(me.Body.String(), csrf) {
		t.Fatalf("me: %d %s", me.Code, me.Body.String())
	}
}

func TestLoginFailureLooksTheSame(t *testing.T) {
	e := newAuthEnv(t, defaultSessions())
	wrong, _, _ := e.login("ayu", "not-the-password")
	unknown, _, _ := e.login("nobody", "not-the-password")
	if wrong.Code != 401 || unknown.Code != 401 || wrong.Body.String() != unknown.Body.String() || respCode(t, wrong) != "INVALID_CREDENTIALS" {
		t.Fatalf("wrong %d %s, unknown %d %s", wrong.Code, wrong.Body.String(), unknown.Code, unknown.Body.String())
	}
	if sessionCookie(wrong) != nil || sessionCookie(unknown) != nil {
		t.Fatal("failed login set a session cookie")
	}
	if w := e.do(authCall{method: "POST", path: "/api/auth/login", body: `{"username":""}`, origin: appOrigin}); w.Code != 400 || respCode(t, w) != "VALIDATION_ERROR" {
		t.Fatalf("empty login: %d %s", w.Code, w.Body.String())
	}
}

func TestLoginRejectsCrossSiteRequests(t *testing.T) {
	e := newAuthEnv(t, defaultSessions())
	body := `{"username":"ayu","password":"` + testutil.Password + `"}`
	if w := e.do(authCall{method: "POST", path: "/api/auth/login", body: body, origin: "https://evil.example"}); w.Code != 403 || respCode(t, w) != "ORIGIN_NOT_ALLOWED" {
		t.Fatalf("foreign origin: %d %s", w.Code, w.Body.String())
	}
	if w := e.do(authCall{method: "POST", path: "/api/auth/login", body: body, origin: appOrigin, contentType: "text/plain"}); w.Code != 415 {
		t.Fatalf("form post: %d %s", w.Code, w.Body.String())
	}
}

func TestMeRequiresSession(t *testing.T) {
	e := newAuthEnv(t, defaultSessions())
	for _, sid := range []string{"", "forged-session-identifier"} {
		if w := e.do(authCall{method: "GET", path: "/api/auth/me", sid: sid}); w.Code != 401 || respCode(t, w) != "UNAUTHENTICATED" {
			t.Fatalf("sid %q: %d %s", sid, w.Code, w.Body.String())
		}
	}
}

func TestLogoutRevokesSession(t *testing.T) {
	e := newAuthEnv(t, defaultSessions())
	_, sid, csrf := e.login("ayu", testutil.Password)
	if w := e.do(authCall{method: "POST", path: "/api/auth/logout", sid: sid, origin: appOrigin}); w.Code != 403 || respCode(t, w) != "CSRF_INVALID" {
		t.Fatalf("logout without token: %d %s", w.Code, w.Body.String())
	}
	if w := e.do(authCall{method: "POST", path: "/api/auth/logout", sid: sid, csrf: "wrong", origin: appOrigin}); w.Code != 403 {
		t.Fatalf("logout with wrong token: %d", w.Code)
	}
	w := e.do(authCall{method: "POST", path: "/api/auth/logout", sid: sid, csrf: csrf, origin: appOrigin})
	if w.Code != 204 {
		t.Fatalf("logout: %d %s", w.Code, w.Body.String())
	}
	if c := sessionCookie(w); c == nil || c.MaxAge >= 0 {
		t.Fatal("logout did not clear the cookie")
	}
	if w := e.do(authCall{method: "GET", path: "/api/auth/me", sid: sid}); w.Code != 401 {
		t.Fatalf("old cookie after logout: %d", w.Code)
	}
}

func TestSessionTimeouts(t *testing.T) {
	e := newAuthEnv(t, defaultSessions())
	_, sid, _ := e.login("ayu", testutil.Password)
	e.clock.Add(31 * time.Minute)
	if w := e.do(authCall{method: "GET", path: "/api/auth/me", sid: sid}); w.Code != 401 {
		t.Fatalf("idle session accepted: %d", w.Code)
	}

	// Stay active every 10 minutes: rotation keeps the session alive but never past the absolute limit.
	_, sid, _ = e.login("ayu", testutil.Password)
	for i := 1; i <= 48; i++ {
		e.clock.Add(10 * time.Minute)
		w := e.do(authCall{method: "GET", path: "/api/auth/me", sid: sid})
		if w.Code != 200 {
			t.Fatalf("active session rejected after %d minutes: %d", i*10, w.Code)
		}
		if c := sessionCookie(w); c != nil {
			sid = c.Value
		}
	}
	e.clock.Add(10 * time.Minute)
	if w := e.do(authCall{method: "GET", path: "/api/auth/me", sid: sid}); w.Code != 401 {
		t.Fatalf("session outlived absolute expiration: %d", w.Code)
	}
}

func TestRotationGracePeriod(t *testing.T) {
	e := newAuthEnv(t, defaultSessions())
	_, old, csrf := e.login("ayu", testutil.Password)
	e.clock.Add(16 * time.Minute)
	w := e.do(authCall{method: "GET", path: "/api/auth/me", sid: old})
	next := sessionCookie(w)
	if w.Code != 200 || next == nil || next.Value == old {
		t.Fatalf("rotation: %d %v", w.Code, next)
	}
	// Within the grace period the old identifier still works and is pointed at its successor.
	e.clock.Add(10 * time.Second)
	if w := e.do(authCall{method: "GET", path: "/api/auth/me", sid: old}); w.Code != 200 || sessionCookie(w) == nil || sessionCookie(w).Value != next.Value {
		t.Fatalf("old identifier in grace: %d", w.Code)
	}
	e.clock.Add(30 * time.Second)
	if w := e.do(authCall{method: "GET", path: "/api/auth/me", sid: old}); w.Code != 401 {
		t.Fatalf("old identifier after grace: %d", w.Code)
	}
	if w := e.do(authCall{method: "GET", path: "/api/auth/me", sid: next.Value}); w.Code != 200 || !strings.Contains(w.Body.String(), csrf) {
		t.Fatalf("new identifier: %d", w.Code)
	}
}
