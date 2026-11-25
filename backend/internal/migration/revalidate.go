package migration

import (
	"context"
	"errors"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/AthillaZaidan/datanest/backend/internal/apperr"
	"github.com/AthillaZaidan/datanest/backend/internal/models"
	"github.com/AthillaZaidan/datanest/backend/internal/position"
)

var (
	errSQLChanged       = errors.New("SQL eksekusi tidak lagi sama dengan migration yang disetujui")
	errReferenceChanged = errors.New("Eksekusi yang dibatalkan atau dilanjutkan sudah berubah sejak permintaan ini disetujui.")
)

// Revalidate re-checks an execution once the worker holds the database lock (design D3)
// and, on the first attempt that starts, assigns the migration its version and freezes
// its SQL (design D2/D4). It is meant for worker.Worker.Revalidate; an error fails the
// execution with the reason.
func (s *Service) Revalidate(ctx context.Context, e *models.MigrationExecution) error {
	return s.platform.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var m models.Migration
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&m, e.MigrationID).Error; err != nil {
			return err
		}
		st, versions, err := position.Load(ctx, tx, e.DatabaseID, e.ID)
		if err != nil {
			return err
		}
		plan, err := position.Decide(st, versions, &m, e.Kind)
		if err != nil {
			var ae *apperr.Error
			if errors.As(err, &ae) {
				return errors.New(ae.Message)
			}
			return err
		}
		if plan.SQL != e.SQLExecuted {
			return errSQLChanged
		}
		if !sameID(plan.Reverts, e.RevertsExecutionID) || !sameID(plan.Continues, e.ContinuesExecutionID) {
			return errReferenceChanged
		}
		if m.Version == nil {
			err := tx.Exec(`UPDATE migrations SET version = (SELECT COALESCE(MAX(version), 0) + 1 FROM migrations WHERE database_id = ?)
				WHERE id = ? AND version IS NULL`, m.DatabaseID, m.ID).Error
			if err != nil {
				return err
			}
		}
		return tx.Exec(`UPDATE migrations SET frozen_at = now() WHERE id = ? AND frozen_at IS NULL`, m.ID).Error
	})
}

func sameID(a, b *uint) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}
