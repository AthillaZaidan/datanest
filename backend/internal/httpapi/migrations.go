package httpapi

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/AthillaZaidan/datanest/backend/internal/apperr"
	"github.com/AthillaZaidan/datanest/backend/internal/migration"
)

type migrationHandlers struct {
	svc *migration.Service
}

type migrationBody struct {
	SQL      *string `json:"sql"`
	Revision *int    `json:"revision"`
}

var errInvalidBody = apperr.ErrValidation.WithMessage("Body JSON tidak valid.")

func bindMigration(c *gin.Context, needRevision bool) (*migrationBody, bool) {
	var b migrationBody
	if err := c.ShouldBindJSON(&b); err != nil {
		RespondError(c, errInvalidBody)
		return nil, false
	}
	details := map[string]string{}
	if b.SQL == nil {
		details["sql"] = "wajib diisi"
	}
	if needRevision && b.Revision == nil {
		details["revision"] = "wajib diisi"
	}
	if len(details) > 0 {
		RespondError(c, apperr.ErrValidation.WithDetails(details))
		return nil, false
	}
	return &b, true
}

func (h migrationHandlers) list(c *gin.Context) {
	userID, ok := currentUser(c)
	if !ok {
		return
	}
	dbID, ok := pathID(c, "databaseId")
	if !ok {
		return
	}
	res, err := h.svc.List(c.Request.Context(), userID, dbID, c.Query("status"))
	if err != nil {
		RespondError(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}

func (h migrationHandlers) create(c *gin.Context) {
	userID, ok := currentUser(c)
	if !ok {
		return
	}
	dbID, ok := pathID(c, "databaseId")
	if !ok {
		return
	}
	body, ok := bindMigration(c, false)
	if !ok {
		return
	}
	v, err := h.svc.Create(c.Request.Context(), userID, dbID, *body.SQL)
	if err != nil {
		RespondError(c, err)
		return
	}
	c.JSON(http.StatusCreated, v)
}

func (h migrationHandlers) get(c *gin.Context) {
	h.withMigration(c, http.StatusOK, h.svc.Get)
}

func (h migrationHandlers) update(c *gin.Context) {
	userID, ok := currentUser(c)
	if !ok {
		return
	}
	id, ok := pathID(c, "migrationId")
	if !ok {
		return
	}
	body, ok := bindMigration(c, true)
	if !ok {
		return
	}
	v, err := h.svc.Update(c.Request.Context(), userID, id, *body.SQL, *body.Revision)
	if err != nil {
		RespondError(c, err)
		return
	}
	c.JSON(http.StatusOK, v)
}

func (h migrationHandlers) delete(c *gin.Context) {
	userID, ok := currentUser(c)
	if !ok {
		return
	}
	id, ok := pathID(c, "migrationId")
	if !ok {
		return
	}
	if err := h.svc.Delete(c.Request.Context(), userID, id); err != nil {
		RespondError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h migrationHandlers) execute(c *gin.Context) {
	h.withMigration(c, http.StatusOK, h.svc.Execute)
}

func (h migrationHandlers) duplicate(c *gin.Context) {
	h.withMigration(c, http.StatusCreated, h.svc.Duplicate)
}

func (h migrationHandlers) withMigration(c *gin.Context, status int, fn func(context.Context, uint, uint) (*migration.View, error)) {
	userID, ok := currentUser(c)
	if !ok {
		return
	}
	id, ok := pathID(c, "migrationId")
	if !ok {
		return
	}
	v, err := fn(c.Request.Context(), userID, id)
	if err != nil {
		RespondError(c, err)
		return
	}
	c.JSON(status, v)
}
