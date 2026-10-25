package web

import (
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/AthillaZaidan/datanest/backend/internal/apperr"
	"github.com/AthillaZaidan/datanest/backend/internal/dbconn"
	"github.com/AthillaZaidan/datanest/backend/internal/explorer"
	"github.com/AthillaZaidan/datanest/backend/internal/migration"
	"github.com/AthillaZaidan/datanest/backend/internal/models"
)

// ---- auth ----

func (h *Handler) loginPage(c *gin.Context) {
	if _, ok := h.Auth.Authenticate(c); ok {
		c.Redirect(http.StatusSeeOther, "/databases")
		return
	}
	h.renderLogin(c, http.StatusOK, "", "")
}

func (h *Handler) renderLogin(c *gin.Context, status int, username, msg string) {
	c.Status(status)
	c.Header("Content-Type", "text/html; charset=utf-8")
	_ = h.pages["login"].ExecuteTemplate(c.Writer, "page", gin.H{"Username": username, "Error": msg, "Demo": h.Demo})
}

func (h *Handler) login(c *gin.Context) {
	username, password := strings.TrimSpace(c.PostForm("username")), c.PostForm("password")
	if username == "" || password == "" {
		h.renderLogin(c, http.StatusBadRequest, username, "Username dan password wajib diisi.")
		return
	}
	u, err := h.Auth.Verify(username, password)
	if err != nil {
		status, msg := describe(err)
		h.renderLogin(c, status, username, msg)
		return
	}
	if _, _, err := h.Auth.StartSession(c, u.ID); err != nil {
		h.renderLogin(c, http.StatusInternalServerError, username, apperr.ErrInternal.Message)
		return
	}
	c.Redirect(http.StatusSeeOther, "/databases")
}

func (h *Handler) logout(c *gin.Context) {
	h.Auth.EndSession(c)
	c.Redirect(http.StatusSeeOther, "/login")
}

// ---- databases ----

type recent struct {
	migration.View
	Database string
}

func (h *Handler) databases(c *gin.Context) {
	ctx, uid := c.Request.Context(), h.uid(c)
	dbs, err := h.Databases.ListForUser(ctx, uid)
	if err != nil {
		h.fail(c, err)
		return
	}
	edit := 0
	var latest []recent
	for _, d := range dbs {
		if d.MyLevel == "EDIT" {
			edit++
		}
		if list, err := h.Migrations.List(ctx, uid, d.ID, ""); err == nil {
			for _, m := range list.Migrations {
				if m.Version != nil {
					latest = append(latest, recent{m, d.Name})
				}
			}
		}
	}
	sort.Slice(latest, func(i, j int) bool { return finishedAt(latest[i].View).After(finishedAt(latest[j].View)) })
	if len(latest) > 6 {
		latest = latest[:6]
	}
	h.render(c, http.StatusOK, "databases", []crumb{{Label: "Databases"}}, gin.H{"Title": "Databases", "Databases": dbs, "Edit": edit, "Recent": latest})
}

func finishedAt(m migration.View) time.Time {
	if m.FinishedAt != nil {
		return *m.FinishedAt
	}
	return m.UpdatedAt
}

// ---- version rail ----

type step struct {
	ID, Version int
	Dot, Label  string
}

type rail struct {
	Steps                   []step
	Hidden, Drafts, Current int
}

const railSteps = 16

func buildRail(list *migration.ListResult) rail {
	r := rail{Current: list.CurrentVersion}
	var done []migration.View
	for _, m := range list.Migrations {
		if m.Status == models.MigrationDraft {
			r.Drafts++
		}
		if m.Version != nil {
			done = append(done, m)
		}
	}
	sort.Slice(done, func(i, j int) bool { return *done[i].Version < *done[j].Version })
	if len(done) > railSteps {
		r.Hidden = len(done) - railSteps
		done = done[r.Hidden:]
	}
	for _, m := range done {
		s := step{ID: int(m.ID), Version: *m.Version, Dot: "size-2.5 bg-[#8d9398]", Label: "sukses"}
		switch {
		case m.Status == models.MigrationFailed:
			s.Dot, s.Label = "size-2.5 border-2 border-negative bg-sheet", "gagal"
		case m.Status == models.MigrationRunning:
			s.Dot, s.Label = "size-2.5 border-2 border-warning bg-sheet", "berjalan"
		case *m.Version == list.CurrentVersion:
			s.Dot, s.Label = "size-3 bg-accent", "versi saat ini"
		}
		r.Steps = append(r.Steps, s)
	}
	return r
}

// ---- explorer ----

type pager struct {
	From, To, Page, Last, PageSize int
	Total                          int64
}

func (h *Handler) database(c *gin.Context) {
	ctx, uid, dbID := c.Request.Context(), h.uid(c), id(c, "id")
	db, err := h.Databases.GetForUser(ctx, uid, dbID)
	if err != nil {
		h.fail(c, err)
		return
	}
	schema := c.DefaultQuery("schema", "public")
	table := c.Query("table")
	tab := "structure"
	if c.Query("tab") == "data" {
		tab = "data"
	}
	data := gin.H{"Title": db.Name, "DB": db, "Schema": schema, "Table": table, "Tab": tab, "PageSizes": []int{10, 25, 50, 100}}
	crumbs := []crumb{{Label: "Databases", To: "/databases"}, {Label: db.Name, To: fmt.Sprintf("/databases/%d", db.ID)}}
	if table != "" {
		crumbs = append(crumbs, crumb{Label: schema + "." + table, Mono: true})
	}
	if list, err := h.Migrations.List(ctx, uid, dbID, ""); err == nil {
		data["Rail"] = buildRail(list)
	}
	schemas, err := h.Explorer.ListSchemas(ctx, uid, dbID)
	if err != nil {
		data["Error"] = msgOf(err)
		h.render(c, http.StatusOK, "database", crumbs, data)
		return
	}
	data["Schemas"] = schemas
	if tables, err := h.Explorer.ListTables(ctx, uid, dbID, schema); err != nil {
		data["TablesError"] = msgOf(err)
	} else {
		data["Tables"] = tables
	}
	if table != "" {
		if tab == "data" {
			h.tableData(c, uid, dbID, schema, table, data)
		} else if detail, err := h.Explorer.GetTable(ctx, uid, dbID, schema, table); err != nil {
			data["TableError"] = msgOf(err)
		} else {
			data["Detail"] = detail
			data["PK"], data["FK"] = keyColumns(detail)
		}
	}
	h.render(c, http.StatusOK, "database", crumbs, data)
}

func (h *Handler) tableData(c *gin.Context, uid, dbID uint, schema, table string, data gin.H) {
	page, size := explorer.ClampPage(explorer.ParseInt(c.Query("page"), 1), explorer.ParseInt(c.Query("pageSize"), 25))
	rows, err := h.Explorer.GetRows(c.Request.Context(), uid, dbID, schema, table, page, size)
	if err != nil {
		data["TableError"] = msgOf(err)
		return
	}
	p := pager{Page: rows.Page, PageSize: rows.PageSize, Total: rows.TotalRows, Last: max(1, int((rows.TotalRows+int64(rows.PageSize)-1)/int64(rows.PageSize)))}
	if rows.TotalRows > 0 {
		p.From = (rows.Page-1)*rows.PageSize + 1
		p.To = min(rows.Page*rows.PageSize, int(rows.TotalRows))
	}
	// A column whose every value on this page is a number is right-aligned in tabular figures.
	numeric := make([]bool, len(rows.Columns))
	for j := range rows.Columns {
		numeric[j] = len(rows.Rows) > 0
		for _, r := range rows.Rows {
			if r[j] != nil && !isNumber(r[j]) {
				numeric[j] = false
			}
		}
	}
	data["Rows"], data["Pager"], data["Numeric"] = rows, p, numeric
}

func keyColumns(d *explorer.TableDetail) (map[string]bool, map[string]bool) {
	pk, fk := map[string]bool{}, map[string]bool{}
	for _, c := range d.Constraints {
		for _, col := range c.Columns {
			switch c.Type {
			case "PRIMARY KEY":
				pk[col] = true
			case "FOREIGN KEY":
				fk[col] = true
			}
		}
	}
	return pk, fk
}

func msgOf(err error) string {
	_, msg := describe(err)
	return msg
}

// ---- migrations ----

var statusTabs = []struct{ Key, Label string }{{"", "Semua"}, {"draft", "Draft"}, {"running", "Berjalan"}, {"success", "Sukses"}, {"failed", "Gagal"}}

type tab struct {
	Key, Label string
	Count      int
}

func (h *Handler) history(c *gin.Context) {
	ctx, uid, dbID := c.Request.Context(), h.uid(c), id(c, "id")
	db, err := h.Databases.GetForUser(ctx, uid, dbID)
	if err != nil {
		h.fail(c, err)
		return
	}
	list, err := h.Migrations.List(ctx, uid, dbID, "")
	if err != nil {
		h.fail(c, err)
		return
	}
	status := c.Query("status")
	var rows []migration.View
	tabs := make([]tab, len(statusTabs))
	for i, t := range statusTabs {
		tabs[i] = tab{Key: t.Key, Label: t.Label}
		for _, m := range list.Migrations {
			if t.Key == "" || string(m.Status) == t.Key {
				tabs[i].Count++
			}
		}
	}
	for _, m := range list.Migrations {
		if status == "" || string(m.Status) == status {
			rows = append(rows, m)
		}
	}
	h.render(c, http.StatusOK, "history", []crumb{{Label: "Databases", To: "/databases"}, {Label: db.Name, To: fmt.Sprintf("/databases/%d", db.ID)}, {Label: "Riwayat migration"}},
		gin.H{"Title": "Riwayat migration", "DB": db, "Rows": rows, "Tabs": tabs, "Status": status, "Rail": buildRail(list), "Current": list.CurrentVersion, "Finding": finding(list.Migrations)})
}

func finding(ms []migration.View) string {
	var last *migration.View
	drafts := 0
	for i, m := range ms {
		if m.Status == models.MigrationDraft {
			drafts++
		}
		if m.Version != nil && (last == nil || *m.Version > *last.Version) {
			last = &ms[i]
		}
	}
	waiting := ""
	if drafts > 0 {
		waiting = fmt.Sprintf(" %d draft menunggu dijalankan.", drafts)
	}
	switch {
	case last == nil:
		return "Belum ada migration yang dijalankan." + waiting
	case last.Status == models.MigrationFailed:
		return fmt.Sprintf("v%d gagal %s: %s.%s", *last.Version, ago(last.FinishedAt), strings.SplitN(last.ErrorMessage, "\n", 2)[0], waiting)
	case last.Status == models.MigrationRunning:
		return fmt.Sprintf("v%d sedang berjalan.%s", *last.Version, waiting)
	}
	by := last.ExecutedBy
	if by == "" {
		by = last.CreatedBy
	}
	return fmt.Sprintf("v%d dijalankan %s %s.%s", *last.Version, by, ago(last.FinishedAt), waiting)
}

func migrationCrumbs(db dbconn.View, last string) []crumb {
	base := fmt.Sprintf("/databases/%d", db.ID)
	return []crumb{{Label: "Databases", To: "/databases"}, {Label: db.Name, To: base}, {Label: "Riwayat migration", To: base + "/migrations"}, {Label: last}}
}

func (h *Handler) newMigration(c *gin.Context) {
	db, err := h.Databases.GetForUser(c.Request.Context(), h.uid(c), id(c, "id"))
	if err != nil {
		h.fail(c, err)
		return
	}
	h.render(c, http.StatusOK, "migration", migrationCrumbs(db, "Migration baru"), gin.H{"Title": "Migration baru", "DB": db, "New": true, "SQL": ""})
}

func (h *Handler) createMigration(c *gin.Context) {
	ctx, uid, dbID := c.Request.Context(), h.uid(c), id(c, "id")
	sql := c.PostForm("sql")
	m, err := h.Migrations.Create(ctx, uid, dbID, sql)
	if err != nil {
		db, dbErr := h.Databases.GetForUser(ctx, uid, dbID)
		if dbErr != nil {
			h.fail(c, dbErr)
			return
		}
		h.render(c, http.StatusOK, "migration", migrationCrumbs(db, "Migration baru"), gin.H{"Title": "Migration baru", "DB": db, "New": true, "SQL": sql, "Error": msgOf(err)})
		return
	}
	c.Redirect(http.StatusSeeOther, fmt.Sprintf("/migrations/%d", m.ID))
}

func (h *Handler) migration(c *gin.Context) {
	h.showMigration(c, id(c, "id"), "", "")
}

// showMigration renders one migration; draftSQL keeps unsaved edits on screen after a failed save.
func (h *Handler) showMigration(c *gin.Context, migrationID uint, draftSQL, msg string) {
	ctx, uid := c.Request.Context(), h.uid(c)
	m, err := h.Migrations.Get(ctx, uid, migrationID)
	if err != nil {
		h.fail(c, err)
		return
	}
	db, err := h.Databases.GetForUser(ctx, uid, m.DatabaseID)
	if err != nil {
		h.fail(c, err)
		return
	}
	title, last := fmt.Sprintf("Draft #%d", m.ID), fmt.Sprintf("Draft #%d", m.ID)
	if m.Version != nil {
		title, last = fmt.Sprintf("Migration v%d", *m.Version), fmt.Sprintf("v%d", *m.Version)
	}
	sql := m.SQL
	if draftSQL != "" {
		sql = draftSQL
	}
	data := gin.H{"Title": title, "DB": db, "M": m, "SQL": sql, "CanEdit": db.MyLevel == "EDIT", "Subtitle": subtitle(m)}
	if msg != "" {
		data["Error"] = msg
	}
	if errors.Is(errFromQuery(c), apperr.ErrRevisionConflict) {
		data["Conflict"] = true
	}
	h.render(c, http.StatusOK, "migration", migrationCrumbs(db, last), data)
}

func errFromQuery(c *gin.Context) error {
	if c.Query("conflict") != "" {
		return apperr.ErrRevisionConflict
	}
	return nil
}

func subtitle(m *migration.View) string {
	switch m.Status {
	case models.MigrationSuccess:
		return fmt.Sprintf("Dijalankan %s pada %s, selesai dalam %s.", m.ExecutedBy, formatTime(m.StartedAt), duration(m.StartedAt, m.FinishedAt))
	case models.MigrationFailed:
		return fmt.Sprintf("Gagal saat dijalankan %s pada %s. Perubahan di-rollback, schema tetap di versi sebelumnya.", m.ExecutedBy, formatTime(m.StartedAt))
	case models.MigrationRunning:
		return fmt.Sprintf("Sedang dijalankan %s sejak %s.", m.ExecutedBy, formatTime(m.StartedAt))
	}
	base := "awal"
	if m.BaseVersion != nil {
		base = fmt.Sprintf("v%d", *m.BaseVersion)
	}
	return fmt.Sprintf("Draft dari %s, dibuat di atas versi %s.", m.CreatedBy, base)
}

func (h *Handler) saveMigration(c *gin.Context) {
	ctx, uid, mID := c.Request.Context(), h.uid(c), id(c, "id")
	sql := c.PostForm("sql")
	revision := explorer.ParseInt(c.PostForm("revision"), 0)
	if _, err := h.Migrations.Update(ctx, uid, mID, sql, revision); err != nil {
		if errors.Is(err, apperr.ErrRevisionConflict) {
			c.Redirect(http.StatusSeeOther, withQuery(fmt.Sprintf("/migrations/%d?conflict=1", mID), "err", msgOf(err)))
			return
		}
		h.showMigration(c, mID, sql, msgOf(err))
		return
	}
	c.Redirect(http.StatusSeeOther, fmt.Sprintf("/migrations/%d", mID))
}

func (h *Handler) executeMigration(c *gin.Context) {
	mID := id(c, "id")
	_, err := h.Migrations.Execute(c.Request.Context(), h.uid(c), mID)
	back(c, fmt.Sprintf("/migrations/%d", mID), err)
}

func (h *Handler) deleteMigration(c *gin.Context) {
	ctx, uid, mID := c.Request.Context(), h.uid(c), id(c, "id")
	m, err := h.Migrations.Get(ctx, uid, mID)
	if err != nil {
		h.fail(c, err)
		return
	}
	if err := h.Migrations.Delete(ctx, uid, mID); err != nil {
		back(c, fmt.Sprintf("/migrations/%d", mID), err)
		return
	}
	c.Redirect(http.StatusSeeOther, fmt.Sprintf("/databases/%d/migrations", m.DatabaseID))
}

func (h *Handler) duplicateMigration(c *gin.Context) {
	mID := id(c, "id")
	m, err := h.Migrations.Duplicate(c.Request.Context(), h.uid(c), mID)
	if err != nil {
		back(c, fmt.Sprintf("/migrations/%d", mID), err)
		return
	}
	c.Redirect(http.StatusSeeOther, fmt.Sprintf("/migrations/%d", m.ID))
}
