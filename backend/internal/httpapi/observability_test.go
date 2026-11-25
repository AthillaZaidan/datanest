package httpapi

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/AthillaZaidan/datanest/backend/internal/apperr"
	"github.com/AthillaZaidan/datanest/backend/internal/obs"
	"github.com/AthillaZaidan/datanest/backend/internal/reqctx"
)

func captureLogs(t *testing.T) func() []map[string]any {
	t.Helper()
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(obs.NewHandler(&buf, slog.LevelDebug)))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return func() []map[string]any {
		var out []map[string]any
		dec := json.NewDecoder(bytes.NewReader(buf.Bytes()))
		for dec.More() {
			var line map[string]any
			if err := dec.Decode(&line); err != nil {
				t.Fatalf("log is not JSON: %v", err)
			}
			out = append(out, line)
		}
		return out
	}
}

func firstEvent(lines []map[string]any, event string) map[string]any {
	for _, l := range lines {
		if l[obs.AttrEvent] == event {
			return l
		}
	}
	return nil
}

func TestRequestGetsAServerMadeCorrelationIDAndOneStructuredLine(t *testing.T) {
	lines := captureLogs(t)
	r := NewRouter(Deps{RequireAuth: passthrough, RequireAdmin: passthrough})

	req := httptest.NewRequest("GET", "/api/healthz?password=hunter2&sid=abc", nil)
	req.Header.Set(RequestIDHeader, "forged-by-client")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	id := w.Header().Get(RequestIDHeader)
	if !regexp.MustCompile(`^[0-9a-f]{16}$`).MatchString(id) || id == "forged-by-client" {
		t.Fatalf("X-Request-ID = %q, want a server-made id", id)
	}
	line := firstEvent(lines(), obs.EventHTTPRequest)
	if line == nil {
		t.Fatal("no http.request line")
	}
	if line[obs.AttrCorrelationID] != id || line[obs.AttrMethod] != "GET" || line[obs.AttrPath] != "/api/healthz" || line[obs.AttrStatus] != float64(200) {
		t.Errorf("line = %v", line)
	}
	if _, ok := line[obs.AttrDurationMS]; !ok {
		t.Error("no duration_ms")
	}
	if raw, _ := json.Marshal(line); strings.Contains(string(raw), "hunter2") || strings.Contains(string(raw), "abc") {
		t.Errorf("the query string reached the log: %s", raw)
	}

	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, httptest.NewRequest("GET", "/api/healthz", nil))
	if w2.Header().Get(RequestIDHeader) == id {
		t.Error("two requests got the same correlation id")
	}
}

func TestRequestLineCarriesTheActor(t *testing.T) {
	lines := captureLogs(t)
	r := NewRouter(Deps{RequireAuth: func(c *gin.Context) { reqctx.SetUserID(c, 7); c.Next() }})
	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/api/databases", nil))
	line := firstEvent(lines(), obs.EventHTTPRequest)
	if line == nil || line[obs.AttrActorID] != float64(7) {
		t.Errorf("line = %v, want actor_id 7", line)
	}
}

func TestDeniedAccessIsLogged(t *testing.T) {
	lines := captureLogs(t)
	r := NewRouter(Deps{
		RequireAuth:  func(c *gin.Context) { reqctx.SetUserID(c, 7); c.Next() },
		RequireAdmin: func(c *gin.Context) { RespondError(c, apperr.ErrForbidden) },
	})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/api/admin/users", nil))
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d", w.Code)
	}
	line := firstEvent(lines(), obs.EventAuthzDenied)
	if line == nil || line[obs.AttrActorID] != float64(7) || line[obs.AttrPath] != "/api/admin/users" || line["code"] != "FORBIDDEN" {
		t.Errorf("authz.denied = %v", line)
	}
	if line[obs.AttrCorrelationID] == nil {
		t.Error("authz.denied has no correlation_id")
	}
}

func TestNotAnApproverIsLoggedAsDenied(t *testing.T) {
	lines := captureLogs(t)
	r := NewRouter(Deps{RequireAuth: func(c *gin.Context) { RespondError(c, apperr.ErrNotAnApprover) }})
	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/api/databases", nil))
	if firstEvent(lines(), obs.EventAuthzDenied) == nil {
		t.Error("NOT_AN_APPROVER was not logged as authz.denied")
	}
}

func TestAPanicIsAnsweredAndLoggedAsJSON(t *testing.T) {
	lines := captureLogs(t)
	r := NewRouter(Deps{RequireAuth: passthrough, Me: func(*gin.Context) { panic("rusak") }})

	w := do(r, "GET", "/api/auth/me")

	var body apperr.Body
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || w.Code != http.StatusInternalServerError || body.Error.Code != "INTERNAL_ERROR" {
		t.Errorf("response = %d %s", w.Code, w.Body)
	}
	if p := firstEvent(lines(), "http.panic"); p == nil || p["level"] != "ERROR" || p[obs.AttrError] != "rusak" || p["stack"] == nil || p[obs.AttrCorrelationID] == nil {
		t.Errorf("panic line = %v", p)
	}
	if q := firstEvent(lines(), obs.EventHTTPRequest); q == nil || q[obs.AttrStatus] != float64(500) {
		t.Errorf("request line = %v", q)
	}
}
