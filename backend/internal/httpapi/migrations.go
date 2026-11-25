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
	// SQL is the Milestone 1 name of SQLUp; either is accepted.
	SQL                *string `json:"sql"`
	SQLUp              *string `json:"sqlUp"`
	SQLDown            string  `json:"sqlDown"`
	Irreversible       bool    `json:"irreversible"`
	IrreversibleReason string  `json:"irreversibleReason"`
	Revision           *int    `json:"revision"`
}

func (b *migrationBody) recipe() migration.Recipe {
	up := b.SQLUp
	if up == nil {
		up = b.SQL
	}
	return migration.Recipe{SQLUp: *up, SQLDown: b.SQLDown, Irreversible: b.Irreversible, IrreversibleReason: b.IrreversibleReason}
}

var errInvalidBody = apperr.ErrValidation.WithMessage("Body JSON tidak valid.")

func bindMigration(c *gin.Context, needRevision bool) (*migrationBody, bool) {
	var b migrationBody
	if err := c.ShouldBindJSON(&b); err != nil {
		RespondError(c, errInvalidBody)
		return nil, false
	}
	details := map[string]string{}
	if b.SQL == nil && b.SQLUp == nil {
		details["sqlUp"] = "wajib diisi"
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
	v, err := h.svc.Create(c.Request.Context(), userID, dbID, body.recipe())
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
	v, err := h.svc.Update(c.Request.Context(), userID, id, body.recipe(), *body.Revision)
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

// explain plans the saved SQL in a rolled-back transaction. ?analyze=true also runs it, which needs EDIT.
func (h migrationHandlers) explain(c *gin.Context) {
	userID, ok := currentUser(c)
	if !ok {
		return
	}
	id, ok := pathID(c, "migrationId")
	if !ok {
		return
	}
	plans, err := h.svc.Explain(c.Request.Context(), userID, id, c.Query("analyze") == "true")
	if err != nil {
		RespondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"plans": plans})
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

// history is GET /databases/:databaseId/executions?hideRollback=true.
func (h migrationHandlers) history(c *gin.Context) {
	userID, ok := currentUser(c)
	if !ok {
		return
	}
	dbID, ok := pathID(c, "databaseId")
	if !ok {
		return
	}
	items, err := h.svc.History(c.Request.Context(), userID, dbID, c.Query("hideRollback") == "true")
	if err != nil {
		RespondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"executions": items})
}
