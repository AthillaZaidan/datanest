// Package httpapi wires the HTTP routes. Handlers stay thin: parse input, call a service, write JSON.
package httpapi

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/AthillaZaidan/datanest/backend/internal/apperr"
	"github.com/AthillaZaidan/datanest/backend/internal/explorer"
	"github.com/AthillaZaidan/datanest/backend/internal/migration"
)

// Deps are what the router needs. Nil middlewares fail closed and nil services leave
// their routes at 501.
type Deps struct {
	Security     gin.HandlerFunc
	RequireAuth  gin.HandlerFunc
	RequireAdmin gin.HandlerFunc
	Login        gin.HandlerFunc
	Logout       gin.HandlerFunc
	Me           gin.HandlerFunc
	Explorer     *explorer.Service
	Migrations   *migration.Service
}

func NewRouter(d Deps) *gin.Engine {
	requireAuth := orNotImplemented(d.RequireAuth)
	requireAdmin := orNotImplemented(d.RequireAdmin)

	listSchemas, listTables, getTable, getRows := notImplemented, notImplemented, notImplemented, notImplemented
	if d.Explorer != nil {
		h := explorerHandlers{svc: d.Explorer}
		listSchemas, listTables, getTable, getRows = h.listSchemas, h.listTables, h.getTable, h.getRows
	}

	listMig, createMig, getMig, updateMig, deleteMig, executeMig, duplicateMig := notImplemented,
		notImplemented, notImplemented, notImplemented, notImplemented, notImplemented, notImplemented
	if d.Migrations != nil {
		h := migrationHandlers{svc: d.Migrations}
		listMig, createMig, getMig, updateMig, deleteMig, executeMig, duplicateMig = h.list, h.create,
			h.get, h.update, h.delete, h.execute, h.duplicate
	}

	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(RequestLogger(log.Default()))
	if d.Security != nil {
		r.Use(d.Security)
	}

	api := r.Group("/api")
	api.GET("/healthz", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ok"}) })

	auth := api.Group("/auth")
	auth.POST("/login", orNotImplemented(d.Login))
	auth.POST("/logout", requireAuth, orNotImplemented(d.Logout))
	auth.GET("/me", requireAuth, orNotImplemented(d.Me))

	admin := api.Group("/admin", requireAuth, requireAdmin)
	admin.GET("/users", notImplemented)
	admin.POST("/users", notImplemented)
	admin.GET("/users/:userId", notImplemented)
	admin.PATCH("/users/:userId", notImplemented)
	admin.DELETE("/users/:userId", notImplemented)
	admin.PUT("/users/:userId/roles/:role", notImplemented)
	admin.DELETE("/users/:userId/roles/:role", notImplemented)

	admin.GET("/groups", notImplemented)
	admin.POST("/groups", notImplemented)
	admin.GET("/groups/:groupId", notImplemented)
	admin.PATCH("/groups/:groupId", notImplemented)
	admin.DELETE("/groups/:groupId", notImplemented)
	admin.GET("/groups/:groupId/members", notImplemented)
	admin.PUT("/groups/:groupId/members/:userId", notImplemented)
	admin.DELETE("/groups/:groupId/members/:userId", notImplemented)
	admin.GET("/groups/:groupId/grants", notImplemented)
	admin.PUT("/groups/:groupId/grants/:databaseId", notImplemented)
	admin.DELETE("/groups/:groupId/grants/:databaseId", notImplemented)

	admin.GET("/databases", notImplemented)
	admin.POST("/databases", notImplemented)
	admin.POST("/databases/test", notImplemented)
	admin.GET("/databases/:databaseId", notImplemented)
	admin.PATCH("/databases/:databaseId", notImplemented)
	admin.DELETE("/databases/:databaseId", notImplemented)
	admin.POST("/databases/:databaseId/test", notImplemented)

	user := api.Group("", requireAuth)
	user.GET("/databases", notImplemented)
	user.GET("/databases/:databaseId", notImplemented)
	user.GET("/databases/:databaseId/schemas", listSchemas)
	user.GET("/databases/:databaseId/schemas/:schema/tables", listTables)
	user.GET("/databases/:databaseId/schemas/:schema/tables/:table", getTable)
	user.GET("/databases/:databaseId/schemas/:schema/tables/:table/rows", getRows)
	user.GET("/databases/:databaseId/migrations", listMig)
	user.POST("/databases/:databaseId/migrations", createMig)

	user.GET("/migrations/:migrationId", getMig)
	user.PATCH("/migrations/:migrationId", updateMig)
	user.DELETE("/migrations/:migrationId", deleteMig)
	user.POST("/migrations/:migrationId/execute", executeMig)
	user.POST("/migrations/:migrationId/duplicate", duplicateMig)

	return r
}

// RespondError writes err in the standard error format and aborts the request.
func RespondError(c *gin.Context, err error) {
	status, body := apperr.Response(err)
	c.AbortWithStatusJSON(status, body)
}

func notImplemented(c *gin.Context) {
	RespondError(c, apperr.ErrNotImplemented)
}

func orNotImplemented(h gin.HandlerFunc) gin.HandlerFunc {
	if h == nil {
		return notImplemented
	}
	return h
}
