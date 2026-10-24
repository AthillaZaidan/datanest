// Package httpapi wires the HTTP routes. Handlers stay thin: parse input, call a service, write JSON.
package httpapi

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/AthillaZaidan/datanest/backend/internal/apperr"
)

// Deps are the middlewares and handlers the router needs. Nil middlewares fail closed (501).
type Deps struct {
	Security     gin.HandlerFunc
	RequireAuth  gin.HandlerFunc
	RequireAdmin gin.HandlerFunc
	Login        gin.HandlerFunc
	Logout       gin.HandlerFunc
	Me           gin.HandlerFunc
}

func NewRouter(d Deps) *gin.Engine {
	requireAuth := orNotImplemented(d.RequireAuth)
	requireAdmin := orNotImplemented(d.RequireAdmin)

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
	user.GET("/databases/:databaseId/schemas", notImplemented)
	user.GET("/databases/:databaseId/schemas/:schema/tables", notImplemented)
	user.GET("/databases/:databaseId/schemas/:schema/tables/:table", notImplemented)
	user.GET("/databases/:databaseId/schemas/:schema/tables/:table/rows", notImplemented)
	user.GET("/databases/:databaseId/migrations", notImplemented)
	user.POST("/databases/:databaseId/migrations", notImplemented)

	user.GET("/migrations/:migrationId", notImplemented)
	user.PATCH("/migrations/:migrationId", notImplemented)
	user.DELETE("/migrations/:migrationId", notImplemented)
	user.POST("/migrations/:migrationId/execute", notImplemented)
	user.POST("/migrations/:migrationId/duplicate", notImplemented)

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
