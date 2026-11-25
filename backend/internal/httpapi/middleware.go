package httpapi

import (
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/AthillaZaidan/datanest/backend/internal/obs"
	"github.com/AthillaZaidan/datanest/backend/internal/reqctx"
)

const RequestIDHeader = "X-Request-ID"

func RequestLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		// The ID is made by the server, never taken from the client.
		id := obs.NewCorrelationID()
		c.Header(RequestIDHeader, id)
		c.Request = c.Request.WithContext(obs.WithCorrelationID(c.Request.Context(), id))

		started := time.Now()
		c.Next()

		attrs := []any{
			obs.AttrMethod, c.Request.Method, obs.AttrPath, c.Request.URL.Path,
			obs.AttrStatus, c.Writer.Status(), obs.AttrDurationMS, time.Since(started).Milliseconds(),
		}
		if userID, ok := reqctx.UserID(c); ok {
			attrs = append(attrs, obs.AttrActorID, userID)
		}
		slog.InfoContext(c.Request.Context(), obs.EventHTTPRequest, attrs...)
	}
}
