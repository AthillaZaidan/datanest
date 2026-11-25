package migration

import (
	"strings"

	"gorm.io/gorm"

	"github.com/AthillaZaidan/datanest/backend/internal/apperr"
	"github.com/AthillaZaidan/datanest/backend/internal/models"
)

// Recipe is the editable part of a migration (design D1).
type Recipe struct {
	SQLUp              string
	SQLDown            string
	Irreversible       bool
	IrreversibleReason string
}

// validate checks what a draft must always satisfy. A reversible draft may still lack
// its down SQL; that is checked when it is submitted.
func (r *Recipe) validate() error {
	details := map[string]string{}
	if strings.TrimSpace(r.SQLUp) == "" {
		details["sqlUp"] = "wajib diisi"
	}
	if r.Irreversible {
		if strings.TrimSpace(r.IrreversibleReason) == "" {
			details["irreversibleReason"] = "wajib diisi untuk migration irreversible"
		}
		r.SQLDown = "" // an irreversible migration has no down SQL
	} else {
		r.IrreversibleReason = ""
	}
	if len(details) > 0 {
		return apperr.ErrValidation.WithMessage("Migration tidak valid.").WithDetails(details)
	}
	return nil
}

var errOpenExecution = apperr.ErrExecutionStateConflict.WithMessage(
	"Migration ini masih punya eksekusi yang belum selesai, sehingga SQL-nya tidak dapat diubah.")

// openStatuses are the execution statuses that keep a recipe locked.
var openStatuses = []models.ExecutionStatus{models.ExecPendingReview, models.ExecApproved, models.ExecQueued,
	models.ExecRunning, models.ExecUncertain, models.ExecPartial}

func openExecutions(db *gorm.DB, migrationID uint) *gorm.DB {
	return db.Model(&models.MigrationExecution{}).Where("migration_id = ? AND status IN ?", migrationID, openStatuses)
}
