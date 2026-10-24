package httpapi

import (
	"log"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/AthillaZaidan/datanest/backend/internal/reqctx"
)

// RequestLogger records only routing metadata. It never records headers or
// bodies, which can contain passwords, session IDs, and CSRF tokens.
func RequestLogger(logger *log.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		started := time.Now()
		c.Next()
		if userID, ok := reqctx.UserID(c); ok {
			logger.Printf("method=%s path=%s status=%d duration=%s user_id=%d", c.Request.Method, c.Request.URL.Path, c.Writer.Status(), time.Since(started), userID)
			return
		}
		logger.Printf("method=%s path=%s status=%d duration=%s", c.Request.Method, c.Request.URL.Path, c.Writer.Status(), time.Since(started))
	}
}
