// Package web serves the server-rendered HTML interface on top of the same services as the JSON API.
package web

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/AthillaZaidan/datanest/backend/internal/admin"
	"github.com/AthillaZaidan/datanest/backend/internal/apperr"
	"github.com/AthillaZaidan/datanest/backend/internal/auth"
	"github.com/AthillaZaidan/datanest/backend/internal/dbconn"
	"github.com/AthillaZaidan/datanest/backend/internal/demo"
	"github.com/AthillaZaidan/datanest/backend/internal/explorer"
	"github.com/AthillaZaidan/datanest/backend/internal/migration"
	"github.com/AthillaZaidan/datanest/backend/internal/models"
	"github.com/AthillaZaidan/datanest/backend/internal/session"
)

//go:embed templates static
var files embed.FS

type Handler struct {
	Auth       *auth.Service
	Admin      *admin.Service
	Databases  *dbconn.Service
	Explorer   *explorer.Service
	Migrations *migration.Service
	// Demo lists accounts on the login page; leave nil outside local demos.
	Demo []demo.Account

	pages map[string]*template.Template
}

type crumb struct {
	Label, To string
	Mono      bool
}

type user struct {
	ID       uint
	Username string
	IsAdmin  bool
}

// Register mounts the pages and their static files on r.
func Register(r *gin.Engine, h *Handler) {
	h.pages = map[string]*template.Template{}
	names, err := fs.Glob(files, "templates/*.html")
	if err != nil {
		panic(err)
	}
	for _, n := range names {
		if strings.HasSuffix(n, "layout.html") {
			continue
		}
		h.pages[strings.TrimSuffix(strings.TrimPrefix(n, "templates/"), ".html")] = template.Must(
			template.New("").Funcs(funcs).ParseFS(files, "templates/layout.html", n))
	}
	static, _ := fs.Sub(files, "static")
	r.StaticFS("/static", http.FS(static))

	r.GET("/login", h.loginPage)
	r.POST("/login", h.login)
	r.GET("/", func(c *gin.Context) { c.Redirect(http.StatusSeeOther, "/databases") })

	p := r.Group("", h.requireSession)
	p.POST("/logout", h.logout)
	p.GET("/databases", h.databases)
	p.GET("/databases/:id", h.database)
	p.GET("/databases/:id/migrations", h.history)
	p.GET("/databases/:id/migrations/new", h.newMigration)
	p.POST("/databases/:id/migrations", h.createMigration)
	p.GET("/migrations/:id", h.migration)
	p.POST("/migrations/:id", h.saveMigration)
	p.POST("/migrations/:id/execute", h.executeMigration)
	p.POST("/migrations/:id/delete", h.deleteMigration)
	p.POST("/migrations/:id/duplicate", h.duplicateMigration)

	a := p.Group("/admin", h.requireAdmin)
	a.GET("/users", h.users)
	a.POST("/users", h.saveUser)
	a.POST("/users/:id/delete", h.deleteUser)
	a.POST("/users/:id/role", h.userRole)
	a.GET("/groups", h.groups)
	a.POST("/groups", h.saveGroup)
	a.POST("/groups/:id/delete", h.deleteGroup)
	a.POST("/groups/:id/members", h.addMember)
	a.POST("/groups/:id/members/:uid/delete", h.removeMember)
	a.POST("/groups/:id/grants", h.setGrant)
	a.POST("/groups/:id/grants/:did/delete", h.removeGrant)
	a.GET("/databases", h.connections)
	a.POST("/databases", h.saveConnection)
	a.POST("/databases/test", h.testConnection)
	a.POST("/databases/:id/test", h.testSavedConnection)
	a.POST("/databases/:id/delete", h.deleteConnection)

	r.NoRoute(func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/api/") {
			c.JSON(http.StatusNotFound, gin.H{"error": gin.H{"code": "NOT_FOUND", "message": "Resource tidak ditemukan."}})
			return
		}
		h.fail(c, apperr.ErrNotFound)
	})
}

// requireSession sends anonymous visitors to the login page and checks the CSRF form field on every state change.
func (h *Handler) requireSession(c *gin.Context) {
	sess, ok := h.Auth.Authenticate(c)
	if !ok {
		c.Redirect(http.StatusSeeOther, "/login")
		c.Abort()
		return
	}
	c.Set("web.session", sess)
	if c.Request.Method == http.MethodPost && !auth.CSRFMatches(c.PostForm("csrf_token"), sess) {
		h.fail(c, apperr.ErrCSRFInvalid)
		c.Abort()
		return
	}
	c.Next()
}

func (h *Handler) requireAdmin(c *gin.Context) {
	if u := h.user(c); !u.IsAdmin {
		h.fail(c, apperr.ErrForbidden)
		c.Abort()
		return
	}
	c.Next()
}

func (h *Handler) user(c *gin.Context) user {
	if v, ok := c.Get("web.user"); ok {
		return v.(user)
	}
	s := c.MustGet("web.session").(*session.Session)
	var m models.User
	h.Auth.DB.First(&m, s.UserID)
	admin, _ := h.Auth.IsAdmin(s.UserID)
	u := user{ID: m.ID, Username: m.Username, IsAdmin: admin}
	c.Set("web.user", u)
	return u
}

func (h *Handler) uid(c *gin.Context) uint { return c.MustGet("web.session").(*session.Session).UserID }

// render executes a page inside the shell; data carries the page's own fields.
func (h *Handler) render(c *gin.Context, status int, page string, crumbs []crumb, data gin.H) {
	if data == nil {
		data = gin.H{}
	}
	data["Crumbs"] = crumbs
	data["Path"] = c.Request.URL.Path
	if _, ok := c.Get("web.session"); ok {
		data["User"] = h.user(c)
		data["CSRF"] = c.MustGet("web.session").(*session.Session).CSRFToken
	}
	if msg := c.Query("err"); msg != "" && data["Error"] == nil {
		data["Error"] = msg
	}
	if msg := c.Query("msg"); msg != "" {
		data["Notice"] = msg
	}
	c.Status(status)
	c.Header("Content-Type", "text/html; charset=utf-8")
	if err := h.pages[page].ExecuteTemplate(c.Writer, "page", data); err != nil {
		log.Printf("render %s: %v", page, err)
	}
}

// fail renders an error page with the API's status code and a safe message.
func (h *Handler) fail(c *gin.Context, err error) {
	status, msg := describe(err)
	h.render(c, status, "error", []crumb{{Label: "Error"}}, gin.H{"Status": status, "Message": msg})
}

func describe(err error) (int, string) {
	var e *apperr.Error
	if errors.As(err, &e) {
		return e.Status, e.Message
	}
	log.Printf("web: %v", err)
	return http.StatusInternalServerError, apperr.ErrInternal.Message
}

// back redirects after a form post (post/redirect/get), carrying the outcome in the query string.
func back(c *gin.Context, target string, err error) {
	if err != nil {
		_, msg := describe(err)
		target = withQuery(target, "err", msg)
	}
	c.Redirect(http.StatusSeeOther, target)
}

func withQuery(target, key, value string) string {
	sep := "?"
	if strings.Contains(target, "?") {
		sep = "&"
	}
	return target + sep + key + "=" + url.QueryEscape(value)
}

func id(c *gin.Context, name string) uint {
	n, _ := strconv.ParseUint(c.Param(name), 10, 64)
	return uint(n)
}

func formID(c *gin.Context, name string) uint {
	n, _ := strconv.ParseUint(c.PostForm(name), 10, 64)
	return uint(n)
}

var months = []string{"Jan", "Feb", "Mar", "Apr", "Mei", "Jun", "Jul", "Agu", "Sep", "Okt", "Nov", "Des"}

func formatTime(t any) string {
	var v time.Time
	switch x := t.(type) {
	case time.Time:
		v = x
	case *time.Time:
		if x == nil {
			return "-"
		}
		v = *x
	default:
		return "-"
	}
	v = v.Local()
	return fmt.Sprintf("%d %s %d, %02d.%02d", v.Day(), months[v.Month()-1], v.Year(), v.Hour(), v.Minute())
}

func ago(t any) string {
	var v time.Time
	switch x := t.(type) {
	case time.Time:
		v = x
	case *time.Time:
		if x == nil {
			return "-"
		}
		v = *x
	}
	d := time.Since(v)
	switch {
	case d < time.Minute:
		return "baru saja"
	case d < time.Hour:
		return fmt.Sprintf("%d menit lalu", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%d jam lalu", int(d.Hours()))
	case d < 7*24*time.Hour:
		return fmt.Sprintf("%d hari lalu", int(d.Hours()/24))
	}
	return formatTime(v)
}

func duration(start, end *time.Time) string {
	if start == nil || end == nil {
		return "-"
	}
	d := end.Sub(*start)
	if d < time.Second {
		return fmt.Sprintf("%d ms", d.Milliseconds())
	}
	return fmt.Sprintf("%.1f s", d.Seconds())
}

// Stable pale identity tints for people; neighbouring ids sit at least 60 degrees apart and skip violet.
var hues = []int{205, 145, 25, 340, 85, 180, 50, 0}

func tint(id uint) template.CSS {
	h := hues[int(id)%len(hues)]
	return template.CSS(fmt.Sprintf("background-color:hsl(%d 55%% 91%%);color:hsl(%d 45%% 26%%)", h, h))
}

func cell(v any) string {
	switch x := v.(type) {
	case json.RawMessage:
		return string(x)
	case map[string]any, []any:
		b, _ := json.Marshal(x)
		return string(b)
	case time.Time:
		return x.Format(time.RFC3339)
	}
	return fmt.Sprint(v)
}

func isNumber(v any) bool {
	switch v.(type) {
	case int, int16, int32, int64, float32, float64:
		return true
	}
	return false
}

var statusPill = map[models.MigrationStatus][2]string{
	models.MigrationDraft:   {"pill-neutral", "Draft"},
	models.MigrationRunning: {"pill-warning", "Berjalan"},
	models.MigrationSuccess: {"pill-positive", "Sukses"},
	models.MigrationFailed:  {"pill-negative", "Gagal"},
}

var funcs = template.FuncMap{
	"time":     formatTime,
	"ago":      ago,
	"duration": duration,
	"tint":     tint,
	"cell":     cell,
	"isNumber": isNumber,
	"icon":     icon,
	"dict": func(kv ...any) map[string]any {
		m := map[string]any{}
		for i := 0; i+1 < len(kv); i += 2 {
			m[kv[i].(string)] = kv[i+1]
		}
		return m
	},
	"initial": func(s string) string {
		if s == "" {
			return "?"
		}
		return strings.ToUpper(s[:1])
	},
	"version": func(v *int) string {
		if v == nil {
			return "-"
		}
		return "v" + strconv.Itoa(*v)
	},
	"status": func(s models.MigrationStatus) template.HTML {
		p := statusPill[s]
		return template.HTML(`<span class="` + p[0] + `">` + p[1] + `</span>`)
	},
	"firstLine": func(s string) string { return strings.SplitN(s, "\n", 2)[0] },
	"lines":     func(s string) int { return strings.Count(s, "\n") + 1 },
	"seq": func(n int) []int {
		out := make([]int, n)
		for i := range out {
			out[i] = i + 1
		}
		return out
	},
	"max":  func(a, b int) int { return max(a, b) },
	"add":  func(a, b int) int { return a + b },
	"join": strings.Join,
	"deref": func(s *string) string {
		if s == nil {
			return "-"
		}
		return *s
	},
	"active": func(path, prefix string) bool { return path == prefix || strings.HasPrefix(path, prefix+"/") },
}

// icon renders a Lucide icon-font glyph (lucide-static, loaded in the layout) by its Lucide name.
func icon(name string, size int) template.HTML {
	return template.HTML(fmt.Sprintf(`<i class="icon-%s flex-none leading-none" style="font-size:%dpx" aria-hidden="true"></i>`, template.HTMLEscapeString(name), size))
}
