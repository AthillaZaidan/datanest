package httpapi

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/AthillaZaidan/datanest/backend/internal/apperr"
	"github.com/AthillaZaidan/datanest/backend/internal/reqctx"
)

// currentUser returns the authenticated user ID, or writes 401 and returns false.
func currentUser(c *gin.Context) (uint, bool) {
	id, ok := reqctx.UserID(c)
	if !ok {
		RespondError(c, apperr.ErrUnauthenticated)
	}
	return id, ok
}

// pathID parses a numeric path parameter, or writes 404 and returns false.
func pathID(c *gin.Context, name string) (uint, bool) {
	id, err := strconv.ParseUint(c.Param(name), 10, 64)
	if err != nil || id == 0 {
		RespondError(c, apperr.ErrNotFound)
		return 0, false
	}
	return uint(id), true
}
