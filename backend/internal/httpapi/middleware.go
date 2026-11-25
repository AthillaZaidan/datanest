package httpapi

import (
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/AthillaZaidan/datanest/backend/internal/apperr"
	"github.com/AthillaZaidan/datanest/backend/internal/obs"
	"github.com/AthillaZaidan/datanest/backend/internal/reqctx"
)

const RequestIDHeader = "X-Request-ID"

func RequestLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		// The ID is made by the server, never taken from the client.
		id := obs.NewCorrelationID()
		c.Header(RequestIDHeader, id)
		ctx := obs.WithCorrelationID(c.Request.Context(), id)
		// Only requests that change state get a span; polled reads would flood the output.
		var span trace.Span
		if changesState(c.Request.Method) {
			ctx, span = obs.Start(ctx, c.Request.Method+" "+routeOf(c), attribute.String(obs.AttrMethod, c.Request.Method))
		}
		c.Request = c.Request.WithContext(ctx)

		started := time.Now()
		c.Next()

		attrs := []any{
			obs.AttrMethod, c.Request.Method, obs.AttrPath, c.Request.URL.Path,
			obs.AttrStatus, c.Writer.Status(), obs.AttrDurationMS, time.Since(started).Milliseconds(),
		}
		if userID, ok := reqctx.UserID(c); ok {
			attrs = append(attrs, obs.AttrActorID, userID)
		}
		slog.InfoContext(ctx, obs.EventHTTPRequest, attrs...)
		if span != nil {
			span.SetAttributes(attribute.Int("http.status_code", c.Writer.Status()))
			if c.Writer.Status() >= 500 {
				span.SetStatus(codes.Error, http.StatusText(c.Writer.Status()))
			}
			span.End()
		}
	}
}

func Recovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if p := recover(); p != nil {
				slog.ErrorContext(c.Request.Context(), obs.EventHTTPPanic, obs.AttrMethod, c.Request.Method, obs.AttrPath, c.Request.URL.Path,
					obs.AttrError, fmt.Sprint(p), "stack", string(debug.Stack()))
				RespondError(c, apperr.ErrInternal)
			}
		}()
		c.Next()
	}
}

func changesState(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	}
	return false
}

func routeOf(c *gin.Context) string {
	if p := c.FullPath(); p != "" {
		return p
	}
	return "(no route)"
}
