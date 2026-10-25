package web

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/AthillaZaidan/datanest/backend/internal/admin"
	"github.com/AthillaZaidan/datanest/backend/internal/dbconn"
	"github.com/AthillaZaidan/datanest/backend/internal/models"
)

func adminCrumbs(label string) []crumb { return []crumb{{Label: "Administrasi"}, {Label: label}} }

// ---- users ----

func (h *Handler) users(c *gin.Context) {
	ctx, a := c.Request.Context(), h.uid(c)
	list, err := h.Admin.Users(ctx, a)
	if err != nil {
		h.fail(c, err)
		return
	}
	admins := 0
	var edit *admin.UserView
	editID := explorerInt(c.Query("edit"))
	for i, u := range list {
		if u.IsAdmin {
			admins++
		}
		if u.ID == editID {
			edit = &list[i]
		}
	}
	h.render(c, http.StatusOK, "users", adminCrumbs("Users"), gin.H{"Title": "Users", "Users": list, "Admins": admins, "Edit": edit})
}

func (h *Handler) saveUser(c *gin.Context) {
	ctx, a := c.Request.Context(), h.uid(c)
	username, password := c.PostForm("username"), c.PostForm("password")
	if uid := formID(c, "id"); uid != 0 {
		p := admin.UserPatch{Username: &username}
		if password != "" {
			p.Password = &password
		}
		_, err := h.Admin.UpdateUser(ctx, a, uid, p)
		back(c, "/admin/users", err)
		return
	}
	_, err := h.Admin.CreateUser(ctx, a, username, password)
	back(c, "/admin/users", err)
}

func (h *Handler) deleteUser(c *gin.Context) {
	back(c, "/admin/users", h.Admin.DeleteUser(c.Request.Context(), h.uid(c), id(c, "id")))
}

func (h *Handler) userRole(c *gin.Context) {
	add := c.PostForm("add") == "1"
	back(c, "/admin/users", h.Admin.Role(c.Request.Context(), h.uid(c), id(c, "id"), models.RolePlatformAdmin, add))
}

// ---- groups ----

func (h *Handler) groups(c *gin.Context) {
	ctx, a := c.Request.Context(), h.uid(c)
	groups, err := h.Admin.Groups(ctx, a)
	if err != nil {
		h.fail(c, err)
		return
	}
	users, err := h.Admin.Users(ctx, a)
	if err != nil {
		h.fail(c, err)
		return
	}
	dbs, err := h.Databases.List(ctx)
	if err != nil {
		h.fail(c, err)
		return
	}
	data := gin.H{"Title": "Groups", "Groups": groups, "Databases": dbs}
	// Open the first group until one is picked, so its members and grants show without a click.
	selected := explorerInt(c.Query("group"))
	if selected == 0 && len(groups) > 0 {
		selected = groups[0].ID
	}
	editID := explorerInt(c.Query("edit"))
	for _, g := range groups {
		if g.ID == selected {
			data["Selected"] = g
		}
		if g.ID == editID {
			data["EditGroup"] = g
		}
	}
	if _, ok := data["Selected"]; ok {
		members, err := h.Admin.Members(ctx, a, selected)
		if err != nil {
			h.fail(c, err)
			return
		}
		grants, err := h.Admin.Grants(ctx, a, selected)
		if err != nil {
			h.fail(c, err)
			return
		}
		in := map[uint]bool{}
		for _, m := range members {
			in[m.ID] = true
		}
		var others []admin.UserView
		for _, u := range users {
			if !in[u.ID] {
				others = append(others, u)
			}
		}
		names := map[uint]dbconn.View{}
		for _, d := range dbs {
			names[d.ID] = d
		}
		type grantRow struct {
			admin.GrantView
			DB dbconn.View
		}
		var rows []grantRow
		for _, g := range grants {
			rows = append(rows, grantRow{g, names[g.DatabaseID]})
		}
		data["Members"], data["Others"], data["Grants"] = members, others, rows
		data["EditGrant"] = explorerInt(c.Query("grant"))
	}
	h.render(c, http.StatusOK, "groups", adminCrumbs("Groups"), data)
}

func groupURL(gid uint) string { return fmt.Sprintf("/admin/groups?group=%d", gid) }

func (h *Handler) saveGroup(c *gin.Context) {
	ctx, a := c.Request.Context(), h.uid(c)
	name, description := c.PostForm("name"), c.PostForm("description")
	if gid := formID(c, "id"); gid != 0 {
		_, err := h.Admin.UpdateGroup(ctx, a, gid, admin.GroupPatch{Name: &name, Description: &description})
		back(c, groupURL(gid), err)
		return
	}
	g, err := h.Admin.CreateGroup(ctx, a, name, description)
	back(c, groupURL(g.ID), err)
}

func (h *Handler) deleteGroup(c *gin.Context) {
	back(c, "/admin/groups", h.Admin.DeleteGroup(c.Request.Context(), h.uid(c), id(c, "id")))
}

func (h *Handler) addMember(c *gin.Context) {
	gid := id(c, "id")
	back(c, groupURL(gid), h.Admin.Member(c.Request.Context(), h.uid(c), gid, formID(c, "uid"), true))
}

func (h *Handler) removeMember(c *gin.Context) {
	gid := id(c, "id")
	back(c, groupURL(gid), h.Admin.Member(c.Request.Context(), h.uid(c), gid, id(c, "uid"), false))
}

func (h *Handler) setGrant(c *gin.Context) {
	gid := id(c, "id")
	back(c, groupURL(gid), h.Admin.Grant(c.Request.Context(), h.uid(c), gid, formID(c, "did"), c.PostForm("level"), true))
}

func (h *Handler) removeGrant(c *gin.Context) {
	gid := id(c, "id")
	back(c, groupURL(gid), h.Admin.Grant(c.Request.Context(), h.uid(c), gid, id(c, "did"), "", false))
}

// ---- connections ----

var sslModes = []string{"disable", "allow", "prefer", "require", "verify-ca", "verify-full"}

func defaultConnection() dbconn.Input {
	return dbconn.Input{Host: "localhost", Port: 15433, Username: "platform_app", SSLMode: "disable"}
}

func (h *Handler) connections(c *gin.Context) {
	form := defaultConnection()
	var editID uint
	if e := explorerInt(c.Query("edit")); e != 0 {
		if v, err := h.Databases.Get(c.Request.Context(), e); err == nil {
			editID = v.ID
			form = dbconn.Input{Name: v.Name, Host: v.Host, Port: v.Port, DBName: v.DBName, Username: v.Username, SSLMode: v.SSLMode}
		}
	}
	h.renderConnections(c, editID, form, nil)
}

func (h *Handler) renderConnections(c *gin.Context, editID uint, form dbconn.Input, extra gin.H) {
	list, err := h.Databases.List(c.Request.Context())
	if err != nil {
		h.fail(c, err)
		return
	}
	data := gin.H{"Title": "Koneksi database", "Connections": list, "Form": form, "EditID": editID, "SSLModes": sslModes}
	for k, v := range extra {
		data[k] = v
	}
	h.render(c, http.StatusOK, "connections", adminCrumbs("Koneksi database"), data)
}

func connectionForm(c *gin.Context) dbconn.Input {
	port, _ := strconv.Atoi(c.PostForm("port"))
	return dbconn.Input{
		Name: strings.TrimSpace(c.PostForm("name")), Host: strings.TrimSpace(c.PostForm("host")), Port: port,
		DBName: strings.TrimSpace(c.PostForm("dbName")), Username: strings.TrimSpace(c.PostForm("username")),
		Password: c.PostForm("password"), SSLMode: c.PostForm("sslMode"),
	}
}

func (h *Handler) saveConnection(c *gin.Context) {
	ctx, f := c.Request.Context(), connectionForm(c)
	editID := formID(c, "id")
	var err error
	if editID != 0 {
		_, err = h.Databases.Update(ctx, editID, dbconn.Patch{Name: &f.Name, Host: &f.Host, Port: &f.Port, DBName: &f.DBName, Username: &f.Username, Password: &f.Password, SSLMode: &f.SSLMode})
	} else {
		_, err = h.Databases.Create(ctx, f)
	}
	if err != nil {
		f.Password = ""
		h.renderConnections(c, editID, f, gin.H{"Error": msgOf(err)})
		return
	}
	c.Redirect(http.StatusSeeOther, "/admin/databases")
}

// testConnection checks the form as typed; the typed password is never echoed back.
func (h *Handler) testConnection(c *gin.Context) {
	f := connectionForm(c)
	res, err := h.Databases.Test(c.Request.Context(), f)
	f.Password = ""
	extra := gin.H{"Result": res}
	if err != nil {
		extra = gin.H{"Error": msgOf(err)}
	}
	h.renderConnections(c, formID(c, "id"), f, extra)
}

func (h *Handler) testSavedConnection(c *gin.Context) {
	res, err := h.Databases.TestSaved(c.Request.Context(), id(c, "id"))
	extra := gin.H{"Result": res}
	if err != nil {
		extra = gin.H{"Error": msgOf(err)}
	}
	h.renderConnections(c, 0, defaultConnection(), extra)
}

func (h *Handler) deleteConnection(c *gin.Context) {
	back(c, "/admin/databases", h.Databases.Delete(c.Request.Context(), id(c, "id")))
}

func explorerInt(s string) uint {
	n, _ := strconv.ParseUint(s, 10, 64)
	return uint(n)
}
