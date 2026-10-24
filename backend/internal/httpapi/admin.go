package httpapi

import (
	"github.com/AthillaZaidan/datanest/backend/internal/admin"
	"github.com/AthillaZaidan/datanest/backend/internal/apperr"
	"github.com/AthillaZaidan/datanest/backend/internal/dbconn"
	"github.com/gin-gonic/gin"
	"net/http"
)

type adminHandlers struct{ svc *admin.Service }

func decode(c *gin.Context, dst any) bool {
	if e := c.ShouldBindJSON(dst); e != nil {
		RespondError(c, apperr.ErrValidation.WithMessage("Body JSON tidak valid."))
		return false
	}
	return true
}
func respond(c *gin.Context, value any, e error) {
	if e != nil {
		RespondError(c, e)
		return
	}
	c.JSON(http.StatusOK, value)
}
func empty(c *gin.Context, e error) {
	if e != nil {
		RespondError(c, e)
		return
	}
	c.Status(http.StatusNoContent)
}
func (h adminHandlers) users(c *gin.Context) {
	a, ok := currentUser(c)
	if !ok {
		return
	}
	v, e := h.svc.Users(c.Request.Context(), a)
	respond(c, v, e)
}
func (h adminHandlers) createUser(c *gin.Context) {
	a, ok := currentUser(c)
	if !ok {
		return
	}
	var i struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if !decode(c, &i) {
		return
	}
	v, e := h.svc.CreateUser(c.Request.Context(), a, i.Username, i.Password)
	if e != nil {
		RespondError(c, e)
		return
	}
	c.JSON(201, v)
}
func (h adminHandlers) user(c *gin.Context) {
	a, ok := currentUser(c)
	if !ok {
		return
	}
	id, ok := pathID(c, "userId")
	if !ok {
		return
	}
	v, e := h.svc.User(c.Request.Context(), a, id)
	respond(c, v, e)
}
func (h adminHandlers) updateUser(c *gin.Context) {
	a, ok := currentUser(c)
	if !ok {
		return
	}
	id, ok := pathID(c, "userId")
	if !ok {
		return
	}
	var p admin.UserPatch
	if !decode(c, &p) {
		return
	}
	v, e := h.svc.UpdateUser(c.Request.Context(), a, id, p)
	respond(c, v, e)
}
func (h adminHandlers) deleteUser(c *gin.Context) {
	a, ok := currentUser(c)
	if !ok {
		return
	}
	id, ok := pathID(c, "userId")
	if !ok {
		return
	}
	empty(c, h.svc.DeleteUser(c.Request.Context(), a, id))
}
func (h adminHandlers) role(c *gin.Context) {
	a, ok := currentUser(c)
	if !ok {
		return
	}
	id, ok := pathID(c, "userId")
	if !ok {
		return
	}
	empty(c, h.svc.Role(c.Request.Context(), a, id, c.Param("role"), c.Request.Method == "PUT"))
}
func (h adminHandlers) groups(c *gin.Context) {
	a, ok := currentUser(c)
	if !ok {
		return
	}
	v, e := h.svc.Groups(c.Request.Context(), a)
	respond(c, v, e)
}
func (h adminHandlers) createGroup(c *gin.Context) {
	a, ok := currentUser(c)
	if !ok {
		return
	}
	var i struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if !decode(c, &i) {
		return
	}
	v, e := h.svc.CreateGroup(c.Request.Context(), a, i.Name, i.Description)
	if e != nil {
		RespondError(c, e)
		return
	}
	c.JSON(201, v)
}
func (h adminHandlers) group(c *gin.Context) {
	a, ok := currentUser(c)
	if !ok {
		return
	}
	id, ok := pathID(c, "groupId")
	if !ok {
		return
	}
	v, e := h.svc.Group(c.Request.Context(), a, id)
	respond(c, v, e)
}
func (h adminHandlers) updateGroup(c *gin.Context) {
	a, ok := currentUser(c)
	if !ok {
		return
	}
	id, ok := pathID(c, "groupId")
	if !ok {
		return
	}
	var p admin.GroupPatch
	if !decode(c, &p) {
		return
	}
	v, e := h.svc.UpdateGroup(c.Request.Context(), a, id, p)
	respond(c, v, e)
}
func (h adminHandlers) deleteGroup(c *gin.Context) {
	a, ok := currentUser(c)
	if !ok {
		return
	}
	id, ok := pathID(c, "groupId")
	if !ok {
		return
	}
	empty(c, h.svc.DeleteGroup(c.Request.Context(), a, id))
}
func (h adminHandlers) members(c *gin.Context) {
	a, ok := currentUser(c)
	if !ok {
		return
	}
	g, ok := pathID(c, "groupId")
	if !ok {
		return
	}
	v, e := h.svc.Members(c.Request.Context(), a, g)
	respond(c, v, e)
}
func (h adminHandlers) member(c *gin.Context) {
	a, ok := currentUser(c)
	if !ok {
		return
	}
	g, ok := pathID(c, "groupId")
	if !ok {
		return
	}
	u, ok := pathID(c, "userId")
	if !ok {
		return
	}
	empty(c, h.svc.Member(c.Request.Context(), a, g, u, c.Request.Method == "PUT"))
}
func (h adminHandlers) grants(c *gin.Context) {
	a, ok := currentUser(c)
	if !ok {
		return
	}
	g, ok := pathID(c, "groupId")
	if !ok {
		return
	}
	v, e := h.svc.Grants(c.Request.Context(), a, g)
	respond(c, v, e)
}
func (h adminHandlers) grant(c *gin.Context) {
	a, ok := currentUser(c)
	if !ok {
		return
	}
	g, ok := pathID(c, "groupId")
	if !ok {
		return
	}
	d, ok := pathID(c, "databaseId")
	if !ok {
		return
	}
	var i struct {
		Level string `json:"level"`
	}
	if c.Request.Method == "PUT" && !decode(c, &i) {
		return
	}
	empty(c, h.svc.Grant(c.Request.Context(), a, g, d, i.Level, c.Request.Method == "PUT"))
}

// Database administration is gated by RequireAdmin. No credentials reach JSON views.
type databaseHandlers struct{ svc *dbconn.Service }

func (h databaseHandlers) list(c *gin.Context) {
	v, e := h.svc.List(c.Request.Context())
	respond(c, v, e)
}
func (h databaseHandlers) get(c *gin.Context) {
	id, ok := pathID(c, "databaseId")
	if !ok {
		return
	}
	v, e := h.svc.Get(c.Request.Context(), id)
	respond(c, v, e)
}
func (h databaseHandlers) create(c *gin.Context) {
	var i dbconn.Input
	if !decode(c, &i) {
		return
	}
	v, e := h.svc.Create(c.Request.Context(), i)
	if e != nil {
		RespondError(c, e)
		return
	}
	c.JSON(201, v)
}
func (h databaseHandlers) update(c *gin.Context) {
	id, ok := pathID(c, "databaseId")
	if !ok {
		return
	}
	var p dbconn.Patch
	if !decode(c, &p) {
		return
	}
	v, e := h.svc.Update(c.Request.Context(), id, p)
	respond(c, v, e)
}
func (h databaseHandlers) delete(c *gin.Context) {
	id, ok := pathID(c, "databaseId")
	if !ok {
		return
	}
	empty(c, h.svc.Delete(c.Request.Context(), id))
}
func (h databaseHandlers) test(c *gin.Context) {
	var i dbconn.Input
	if !decode(c, &i) {
		return
	}
	v, e := h.svc.Test(c.Request.Context(), i)
	respond(c, v, e)
}
func (h databaseHandlers) testSaved(c *gin.Context) {
	id, ok := pathID(c, "databaseId")
	if !ok {
		return
	}
	v, e := h.svc.TestSaved(c.Request.Context(), id)
	respond(c, v, e)
}
func (h databaseHandlers) listForUser(c *gin.Context) {
	u, ok := currentUser(c)
	if !ok {
		return
	}
	v, e := h.svc.ListForUser(c.Request.Context(), u)
	respond(c, v, e)
}
func (h databaseHandlers) getForUser(c *gin.Context) {
	u, ok := currentUser(c)
	if !ok {
		return
	}
	id, ok := pathID(c, "databaseId")
	if !ok {
		return
	}
	v, e := h.svc.GetForUser(c.Request.Context(), u, id)
	respond(c, v, e)
}
