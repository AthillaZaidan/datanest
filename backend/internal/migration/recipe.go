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
	Mode               models.ExecutionMode
	BatchTable         string
	BatchColumn        string
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
	switch r.Mode {
	case "", models.ExecModeNormal:
		r.Mode, r.BatchTable, r.BatchColumn = models.ExecModeNormal, "", ""
	case models.ExecModeBatch:
		r.BatchTable, r.BatchColumn = strings.TrimSpace(r.BatchTable), strings.TrimSpace(r.BatchColumn)
		if r.BatchTable == "" {
			details["batchTable"] = "wajib diisi untuk mode per-batch"
		}
		if r.BatchColumn == "" {
			details["batchColumn"] = "wajib diisi untuk mode per-batch"
		}
	default:
		details["mode"] = "harus normal atau batch"
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

func recipeOf(m *models.Migration) Recipe {
	return Recipe{SQLUp: m.SQL, SQLDown: m.SQLDown, Irreversible: m.Irreversible, IrreversibleReason: m.IrreversibleReason,
		Mode: m.Mode, BatchTable: m.BatchTable, BatchColumn: m.BatchColumn}
}
