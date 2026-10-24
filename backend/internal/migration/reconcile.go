package migration

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/AthillaZaidan/datanest/backend/internal/models"
)

const msgInterrupted = "Eksekusi terputus; perubahan di-rollback."

// Reconcile settles migrations left running by a crash, using each target's ledger.
// Call it at startup, before serving requests. Unreachable targets stay running and
// are reported in the returned error.
func (s *Service) Reconcile(ctx context.Context) error {
	var running []models.Migration
	if err := s.platform.WithContext(ctx).Where("status = ?", models.MigrationRunning).Find(&running).Error; err != nil {
		return err
	}

	var errs []error
	for _, m := range running {
		if err := s.reconcileOne(ctx, &m); err != nil {
			errs = append(errs, fmt.Errorf("migration %d: %w", m.ID, err))
		}
	}
	return errors.Join(errs...)
}

func (s *Service) reconcileOne(ctx context.Context, m *models.Migration) error {
	if m.Version == nil {
		return errors.New("running migration without version")
	}
	db, err := s.targets.Get(ctx, m.DatabaseID)
	if err != nil {
		return err
	}
	applied, err := ledgerHas(ctx, db, *m.Version)
	if err != nil {
		return err
	}

	update := map[string]any{"status": models.MigrationSuccess, "finished_at": time.Now(), "updated_at": time.Now()}
	if !applied {
		update["status"] = models.MigrationFailed
		update["error_message"] = msgInterrupted
	}
	return s.platform.WithContext(ctx).Model(&models.Migration{}).
		Where("id = ? AND status = ?", m.ID, models.MigrationRunning).Updates(update).Error
}

func ledgerHas(ctx context.Context, db *sql.DB, version int) (bool, error) {
	var exists bool
	err := db.QueryRowContext(ctx, `SELECT to_regclass('public.`+LedgerTable+`') IS NOT NULL`).Scan(&exists)
	if err != nil || !exists {
		return false, err
	}
	err = db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM public.`+LedgerTable+` WHERE version = $1)`, version).Scan(&exists)
	return exists, err
}
