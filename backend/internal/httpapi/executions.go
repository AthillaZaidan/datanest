package httpapi

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/AthillaZaidan/datanest/backend/internal/execution"
)

type executionHandlers struct {
	svc *execution.Service
}

func (h executionHandlers) get(c *gin.Context) {
	userID, ok := currentUser(c)
	if !ok {
		return
	}
	id, ok := pathID(c, "executionId")
	if !ok {
		return
	}
	v, err := h.svc.Get(c.Request.Context(), userID, id)
	if err != nil {
		RespondError(c, err)
		return
	}
	c.JSON(http.StatusOK, v)
}
