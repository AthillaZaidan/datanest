package httpapi

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/AthillaZaidan/datanest/backend/internal/approval"
	"github.com/AthillaZaidan/datanest/backend/internal/execution"
	"github.com/AthillaZaidan/datanest/backend/internal/models"
)

type approvalHandlers struct {
	approvals  *approval.Service
	executions *execution.Service
}

type reviewAction func(c *gin.Context, userID, executionID uint) (*models.MigrationExecution, error)

func (h approvalHandlers) act(do reviewAction) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, ok := currentUser(c)
		if !ok {
			return
		}
		id, ok := pathID(c, "executionId")
		if !ok {
			return
		}
		e, err := do(c, userID, id)
		if err != nil {
			RespondError(c, err)
			return
		}
		v, err := h.executions.Get(c.Request.Context(), userID, e.ID)
		if err != nil {
			RespondError(c, err)
			return
		}
		c.JSON(http.StatusOK, v)
	}
}

func (h approvalHandlers) approve(c *gin.Context, userID, id uint) (*models.MigrationExecution, error) {
	return h.approvals.Approve(c.Request.Context(), userID, id)
}

func (h approvalHandlers) reject(c *gin.Context, userID, id uint) (*models.MigrationExecution, error) {
	var b struct {
		Reason string `json:"reason"`
	}
	if c.Request.ContentLength != 0 {
		if err := c.ShouldBindJSON(&b); err != nil {
			return nil, errInvalidBody
		}
	}
	return h.approvals.Reject(c.Request.Context(), userID, id, b.Reason)
}

func (h approvalHandlers) cancel(c *gin.Context, userID, id uint) (*models.MigrationExecution, error) {
	return h.approvals.Cancel(c.Request.Context(), userID, id)
}

func (h approvalHandlers) withdraw(c *gin.Context, userID, id uint) (*models.MigrationExecution, error) {
	return h.approvals.Withdraw(c.Request.Context(), userID, id)
}

func (h approvalHandlers) pending(c *gin.Context) {
	userID, ok := currentUser(c)
	if !ok {
		return
	}
	items, err := h.approvals.Pending(c.Request.Context(), userID)
	if err != nil {
		RespondError(c, err)
		return
	}
	c.JSON(http.StatusOK, items)
}
