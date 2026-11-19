package httpapi

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/AthillaZaidan/datanest/backend/internal/apperr"
)

// ServeSPA serves the built frontend from dir. Any GET that is not an /api route and not a
// real file falls back to index.html so client-side routes survive a reload. A missing
// build is not fatal: the API keeps working and the page says how to build it.
func ServeSPA(r *gin.Engine, dir string) {
	index := filepath.Join(dir, "index.html")
	r.NoRoute(func(c *gin.Context) {
		path := c.Request.URL.Path
		if c.Request.Method != http.MethodGet || path == "/api" || strings.HasPrefix(path, "/api/") {
			RespondError(c, apperr.ErrNotFound)
			return
		}
		if file := filepath.Join(dir, filepath.Clean("/"+path)); path != "/" {
			if st, err := os.Stat(file); err == nil && !st.IsDir() {
				c.File(file)
				return
			}
		}
		if _, err := os.Stat(index); err != nil {
			c.String(http.StatusNotFound, "Frontend belum dibangun. Jalankan: cd frontend && bun install && bun run build")
			return
		}
		c.File(index)
	})
}
