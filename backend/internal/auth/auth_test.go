package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/AthillaZaidan/datanest/backend/internal/session"
	"github.com/gin-gonic/gin"
)

func TestValidateStateRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(ValidateStateRequest("http://localhost:5173"))
	r.POST("/", func(c *gin.Context) { c.Status(204) })
	for _, tt := range []struct {
		origin, contentType string
		status              int
	}{{"http://evil.test", "application/json", 403}, {"http://localhost:5173", "text/plain", 415}, {"http://localhost:5173", "application/json", 204}} {
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		req.Header.Set("Origin", tt.origin)
		req.Header.Set("Content-Type", tt.contentType)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != tt.status {
			t.Errorf("status = %d, want %d", w.Code, tt.status)
		}
	}
}
func TestConstantEqual(t *testing.T) {
	if !constantEqual("abc", "abc") || constantEqual("abc", "abd") || constantEqual("a", "aa") {
		t.Fatal("constant comparison result incorrect")
	}
}
func TestCookieAttributes(t *testing.T) {
	s := &Service{Sessions: session.New(session.Config{}, nil), CookieSecure: true}
	defer s.Sessions.Close()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	s.setCookie(c, "value")
	cookie := c.Writer.Header().Get("Set-Cookie")
	if cookie == "" || !contains(cookie, "HttpOnly") || !contains(cookie, "Secure") || !contains(cookie, "SameSite=Strict") {
		t.Fatalf("cookie = %q", cookie)
	}
}
func contains(s, part string) bool {
	for i := range s {
		if len(s)-i >= len(part) && s[i:i+len(part)] == part {
			return true
		}
	}
	return false
}
