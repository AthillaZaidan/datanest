// Package httpapi wires the HTTP routes. Handlers stay thin: parse input, call a service, write JSON.
package httpapi

import (
	"github.com/AthillaZaidan/datanest/backend/internal/admin"
	"github.com/AthillaZaidan/datanest/backend/internal/approval"
	"github.com/AthillaZaidan/datanest/backend/internal/dbconn"
	"github.com/AthillaZaidan/datanest/backend/internal/execution"
	"github.com/AthillaZaidan/datanest/backend/internal/obs"
	"github.com/AthillaZaidan/datanest/backend/internal/reqctx"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/AthillaZaidan/datanest/backend/internal/apperr"
	"github.com/AthillaZaidan/datanest/backend/internal/explorer"
	"github.com/AthillaZaidan/datanest/backend/internal/migration"
)

// Deps are what the router needs. Nil middlewares fail closed and nil services leave
// their routes at 501.
type Deps struct {
	Admin        *admin.Service
	Databases    *dbconn.Service
	Security     gin.HandlerFunc
	RequireAuth  gin.HandlerFunc
	RequireAdmin gin.HandlerFunc
	Login        gin.HandlerFunc
	Logout       gin.HandlerFunc
	Me           gin.HandlerFunc
	Explorer     *explorer.Service
	Migrations   *migration.Service
	Executions   *execution.Service
	Approvals    *approval.Service
}

func NewRouter(d Deps) *gin.Engine {
	requireAuth := orNotImplemented(d.RequireAuth)
	requireAdmin := orNotImplemented(d.RequireAdmin)

	listSchemas, listTables, getTable, getRows := notImplemented, notImplemented, notImplemented, notImplemented
	if d.Explorer != nil {
		h := explorerHandlers{svc: d.Explorer}
		listSchemas, listTables, getTable, getRows = h.listSchemas, h.listTables, h.getTable, h.getRows
	}

	listMig, createMig, getMig, updateMig, deleteMig, duplicateMig, explainMig := notImplemented,
		notImplemented, notImplemented, notImplemented, notImplemented, notImplemented, notImplemented
	if d.Migrations != nil {
		h := migrationHandlers{svc: d.Migrations}
		listMig, createMig, getMig, updateMig, deleteMig, duplicateMig, explainMig = h.list, h.create,
			h.get, h.update, h.delete, h.duplicate, h.explain
	}

	getExec := notImplemented
	if d.Executions != nil {
		getExec = executionHandlers{svc: d.Executions}.get
	}
	historyExec := notImplemented
	if d.Migrations != nil {
		historyExec = migrationHandlers{svc: d.Migrations}.history
	}
	submitExec := notImplemented
	if d.Migrations != nil && d.Executions != nil {
		submitExec = submissionHandlers{migrations: d.Migrations, executions: d.Executions}.submit
	}

	approve, reject, cancelExec, withdraw, pending := notImplemented, notImplemented, notImplemented, notImplemented, notImplemented
	if d.Approvals != nil && d.Executions != nil {
		h := approvalHandlers{approvals: d.Approvals, executions: d.Executions}
		approve, reject, cancelExec, withdraw = h.act(h.approve), h.act(h.reject), h.act(h.cancel), h.act(h.withdraw)
		pending = h.pending
	}

	ah := adminHandlers{svc: d.Admin}
	dh := databaseHandlers{svc: d.Databases}
	adminHandler := func(h gin.HandlerFunc) gin.HandlerFunc {
		if d.Admin == nil {
			return notImplemented
		}
		return h
	}
	databaseHandler := func(h gin.HandlerFunc) gin.HandlerFunc {
		if d.Databases == nil {
			return notImplemented
		}
		return h
	}
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(RequestLogger())
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
	admin.GET("/users", adminHandler(ah.users))
	admin.POST("/users", adminHandler(ah.createUser))
	admin.GET("/users/:userId", adminHandler(ah.user))
	admin.PATCH("/users/:userId", adminHandler(ah.updateUser))
	admin.DELETE("/users/:userId", adminHandler(ah.deleteUser))
	admin.PUT("/users/:userId/roles/:role", adminHandler(ah.role))
	admin.DELETE("/users/:userId/roles/:role", adminHandler(ah.role))

	admin.GET("/groups", adminHandler(ah.groups))
	admin.POST("/groups", adminHandler(ah.createGroup))
	admin.GET("/groups/:groupId", adminHandler(ah.group))
	admin.PATCH("/groups/:groupId", adminHandler(ah.updateGroup))
	admin.DELETE("/groups/:groupId", adminHandler(ah.deleteGroup))
	admin.GET("/groups/:groupId/members", adminHandler(ah.members))
	admin.PUT("/groups/:groupId/members/:userId", adminHandler(ah.member))
	admin.DELETE("/groups/:groupId/members/:userId", adminHandler(ah.member))
	admin.PUT("/groups/:groupId/leader", adminHandler(ah.leader))
	admin.DELETE("/groups/:groupId/leader", adminHandler(ah.leader))
	admin.GET("/groups/:groupId/grants", adminHandler(ah.grants))
	admin.PUT("/groups/:groupId/grants/:databaseId", adminHandler(ah.grant))
	admin.DELETE("/groups/:groupId/grants/:databaseId", adminHandler(ah.grant))

	admin.GET("/databases", databaseHandler(dh.list))
	admin.POST("/databases", databaseHandler(dh.create))
	admin.POST("/databases/test", databaseHandler(dh.test))
	admin.GET("/databases/:databaseId", databaseHandler(dh.get))
	admin.PATCH("/databases/:databaseId", databaseHandler(dh.update))
	admin.DELETE("/databases/:databaseId", databaseHandler(dh.delete))
	admin.POST("/databases/:databaseId/test", databaseHandler(dh.testSaved))

	user := api.Group("", requireAuth)
	user.GET("/databases", databaseHandler(dh.listForUser))
	user.GET("/databases/:databaseId", databaseHandler(dh.getForUser))
	user.GET("/databases/:databaseId/schemas", listSchemas)
	user.GET("/databases/:databaseId/schemas/:schema/tables", listTables)
	user.GET("/databases/:databaseId/schemas/:schema/tables/:table", getTable)
	user.GET("/databases/:databaseId/schemas/:schema/tables/:table/rows", getRows)
	user.GET("/databases/:databaseId/migrations", listMig)
	user.POST("/databases/:databaseId/migrations", createMig)

	user.GET("/migrations/:migrationId", getMig)
	user.PATCH("/migrations/:migrationId", updateMig)
	user.DELETE("/migrations/:migrationId", deleteMig)
	user.POST("/migrations/:migrationId/duplicate", duplicateMig)
	user.POST("/migrations/:migrationId/explain", explainMig)

	user.GET("/databases/:databaseId/executions", historyExec)
	user.POST("/migrations/:migrationId/executions", submitExec)
	user.GET("/executions/:executionId", getExec)
	user.POST("/executions/:executionId/approve", approve)
	user.POST("/executions/:executionId/reject", reject)
	user.POST("/executions/:executionId/cancel", cancelExec)
	user.POST("/executions/:executionId/withdraw", withdraw)
	user.GET("/approvals/pending", pending)

	return r
}

// RespondError writes err in the standard error format and aborts the request.
func RespondError(c *gin.Context, err error) {
	status, body := apperr.Response(err)
	if body.Error.Code == apperr.ErrForbidden.Code || body.Error.Code == apperr.ErrNotAnApprover.Code {
		attrs := []any{obs.AttrMethod, c.Request.Method, obs.AttrPath, c.Request.URL.Path, "code", body.Error.Code}
		if userID, ok := reqctx.UserID(c); ok {
			attrs = append(attrs, obs.AttrActorID, userID)
		}
		slog.WarnContext(c.Request.Context(), obs.EventAuthzDenied, attrs...)
	}
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
