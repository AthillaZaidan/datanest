package httpapi

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/AthillaZaidan/datanest/backend/internal/apperr"
	"github.com/AthillaZaidan/datanest/backend/internal/execution"
	"github.com/AthillaZaidan/datanest/backend/internal/migration"
	"github.com/AthillaZaidan/datanest/backend/internal/models"
)

type submissionHandlers struct {
	migrations *migration.Service
	executions *execution.Service
}

type submissionBody struct {
	Kind      models.ExecutionKind `json:"kind"`
	BatchSize *int                 `json:"batchSize"`
}

// submit is POST /migrations/:migrationId/executions: request a run, forward or rollback.
func (h submissionHandlers) submit(c *gin.Context) {
	userID, ok := currentUser(c)
	if !ok {
		return
	}
	id, ok := pathID(c, "migrationId")
	if !ok {
		return
	}
	var b submissionBody
	if err := c.ShouldBindJSON(&b); err != nil {
		RespondError(c, errInvalidBody)
		return
	}
	if b.BatchSize != nil {
		RespondError(c, apperr.ErrValidation.WithMessage("Mode per-batch belum dapat diajukan dari endpoint ini.").
			WithDetails(map[string]string{"batchSize": "belum didukung"}))
		return
	}
	e, err := h.migrations.Submit(c.Request.Context(), userID, id, b.Kind)
	if err != nil {
		RespondError(c, err)
		return
	}
	v, err := h.executions.Get(c.Request.Context(), userID, e.ID)
	if err != nil {
		RespondError(c, err)
		return
	}
	c.JSON(http.StatusCreated, v)
}
