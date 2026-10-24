package httpapi

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/AthillaZaidan/datanest/backend/internal/explorer"
)

type explorerHandlers struct {
	svc *explorer.Service
}

func (h explorerHandlers) listSchemas(c *gin.Context) {
	userID, ok := currentUser(c)
	if !ok {
		return
	}
	dbID, ok := pathID(c, "databaseId")
	if !ok {
		return
	}
	schemas, err := h.svc.ListSchemas(c.Request.Context(), userID, dbID)
	if err != nil {
		RespondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"schemas": schemas})
}

func (h explorerHandlers) listTables(c *gin.Context) {
	userID, ok := currentUser(c)
	if !ok {
		return
	}
	dbID, ok := pathID(c, "databaseId")
	if !ok {
		return
	}
	tables, err := h.svc.ListTables(c.Request.Context(), userID, dbID, c.Param("schema"))
	if err != nil {
		RespondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"tables": tables})
}

func (h explorerHandlers) getTable(c *gin.Context) {
	userID, ok := currentUser(c)
	if !ok {
		return
	}
	dbID, ok := pathID(c, "databaseId")
	if !ok {
		return
	}
	detail, err := h.svc.GetTable(c.Request.Context(), userID, dbID, c.Param("schema"), c.Param("table"))
	if err != nil {
		RespondError(c, err)
		return
	}
	c.JSON(http.StatusOK, detail)
}

func (h explorerHandlers) getRows(c *gin.Context) {
	userID, ok := currentUser(c)
	if !ok {
		return
	}
	dbID, ok := pathID(c, "databaseId")
	if !ok {
		return
	}
	page := explorer.ParseInt(c.Query("page"), 1)
	pageSize := explorer.ParseInt(c.Query("pageSize"), explorer.DefaultPageSize)
	rows, err := h.svc.GetRows(c.Request.Context(), userID, dbID, c.Param("schema"), c.Param("table"), page, pageSize)
	if err != nil {
		RespondError(c, err)
		return
	}
	c.JSON(http.StatusOK, rows)
}
