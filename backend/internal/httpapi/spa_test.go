package httpapi

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestServeSPA(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "index.html"), []byte("<div id=root></div>"), 0o644)
	os.MkdirAll(filepath.Join(dir, "assets"), 0o755)
	os.WriteFile(filepath.Join(dir, "assets", "a.js"), []byte("console.log(1)"), 0o644)
	secret := filepath.Join(filepath.Dir(dir), "secret.txt")
	os.WriteFile(secret, []byte("secret"), 0o644)

	r := gin.New()
	ServeSPA(r, dir)
	get := func(method, path string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(method, path, nil))
		return w
	}

	for path, want := range map[string]string{
		"/":             "<div id=root>",
		"/migrations/3": "<div id=root>", // client-side route survives a reload
		"/assets/a.js":  "console.log(1)",
		"/assets":       "<div id=root>", // a directory is not a file
	} {
		if w := get("GET", path); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), want) {
			t.Errorf("GET %s = %d %q, want %q", path, w.Code, w.Body, want)
		}
	}
	// Traversal never reaches files outside the build directory.
	for _, path := range []string{"/../secret.txt", "/%2e%2e/secret.txt", "/assets/../../secret.txt"} {
		if w := get("GET", path); w.Code == http.StatusOK && strings.Contains(w.Body.String(), "secret") {
			t.Errorf("GET %s leaked a file outside the build directory", path)
		}
	}
	// Unknown API paths and non-GET requests stay errors; they must not return HTML.
	for _, c := range [][2]string{{"GET", "/api/nope"}, {"GET", "/api"}, {"POST", "/anything"}} {
		if w := get(c[0], c[1]); w.Code != http.StatusNotFound || strings.Contains(w.Body.String(), "<div") {
			t.Errorf("%s %s = %d %q", c[0], c[1], w.Code, w.Body)
		}
	}
}

func TestServeSPAWithoutBuild(t *testing.T) {
	r := gin.New()
	ServeSPA(r, t.TempDir())
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/databases", nil))
	if w.Code != http.StatusNotFound || !strings.Contains(w.Body.String(), "bun run build") {
		t.Fatalf("missing build = %d %q", w.Code, w.Body)
	}
}
