// Package reqctx stores per-request values, such as the authenticated user, on the Gin context.
package reqctx

import "github.com/gin-gonic/gin"

const userIDKey = "datanest.userID"

func SetUserID(c *gin.Context, id uint) {
	c.Set(userIDKey, id)
}

func UserID(c *gin.Context) (uint, bool) {
	v, ok := c.Get(userIDKey)
	if !ok {
		return 0, false
	}
	id, ok := v.(uint)
	return id, ok
}
