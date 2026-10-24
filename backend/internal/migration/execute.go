package migration

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/AthillaZaidan/datanest/backend/internal/apperr"
	"github.com/AthillaZaidan/datanest/backend/internal/authz"
	"github.com/AthillaZaidan/datanest/backend/internal/migration/sqlguard"
	"github.com/AthillaZaidan/datanest/backend/internal/models"
	"github.com/AthillaZaidan/datanest/backend/internal/targeterr"
)

const msgNotInTransaction = "Statement ini tidak dapat dijalankan dalam transaction (misal CREATE INDEX CONCURRENTLY, VACUUM). " +
	"Platform menjalankan migration dalam transaction agar dapat di-rollback."

// Execute runs a draft on its target database. A failing SQL is not an HTTP error: the
// migration is recorded as failed and returned. Errors mean execution never started.
func (s *Service) Execute(ctx context.Context, userID, migrationID uint) (*View, error) {
	m, err := s.loadAuthorized(ctx, userID, migrationID, authz.ExecuteMigration)
	if err != nil {
		return nil, err
	}
	if m.Status != models.MigrationDraft {
		return nil, apperr.ErrMigrationNotDraft
	}
	if err := sqlguard.Check(m.SQL); err != nil {
		return nil, err
	}
	u, err := s.user(ctx, userID)
	if err != nil {
		return nil, err
	}
	db, err := s.targets.Get(ctx, m.DatabaseID)
	if err != nil {
		return nil, targeterr.Map(err)
	}

	// Held until the result is recorded, so version order matches commit order.
	unlock, ok := s.locks.TryLock(m.DatabaseID)
	if !ok {
		return nil, apperr.ErrMigrationRunning
	}
	defer unlock()

	// Finish and record the result even if the client goes away.
	ctx = context.WithoutCancel(ctx)

	version, err := s.start(ctx, m.ID, m.DatabaseID, u)
	if err != nil {
		return nil, err
	}

	execCtx, cancel := context.WithTimeout(ctx, s.opts.ExecTimeout)
	runErr := s.runOnTarget(execCtx, db, version, m.SQL)
	cancel()

	if err := s.finish(ctx, m.ID, runErr); err != nil {
		return nil, err
	}
	if err := s.platform.WithContext(ctx).First(m, m.ID).Error; err != nil {
		return nil, err
	}
	return s.view(ctx, m)
}

// start claims the draft and assigns the next version in one statement.
func (s *Service) start(ctx context.Context, migrationID, databaseID uint, u *models.User) (int, error) {
	var versions []int
	now := time.Now()
	err := s.platform.WithContext(ctx).Raw(`
		UPDATE migrations SET
			status = ?,
			version = (SELECT COALESCE(MAX(version), 0) + 1 FROM migrations WHERE database_id = ?),
			started_at = ?, updated_at = ?,
			executed_by_id = ?, executed_by_username = ?
		WHERE id = ? AND status = ?
		RETURNING version`,
		models.MigrationRunning, databaseID, now, now, u.ID, u.Username, migrationID, models.MigrationDraft,
	).Scan(&versions).Error
	if err != nil {
		return 0, err
	}
	if len(versions) == 0 {
		return 0, apperr.ErrMigrationNotDraft
	}
	return versions[0], nil
}

// runOnTarget applies sql and the ledger row in one transaction: both commit or neither does.
func (s *Service) runOnTarget(ctx context.Context, db *sql.DB, version int, migrationSQL string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	setup := []string{
		`CREATE TABLE IF NOT EXISTS public.` + LedgerTable + ` (version integer PRIMARY KEY, applied_at timestamptz NOT NULL DEFAULT now())`,
	}
	if s.opts.LockTimeout > 0 {
		setup = append(setup, fmt.Sprintf("SET LOCAL lock_timeout = %d", s.opts.LockTimeout.Milliseconds()))
	}
	if s.opts.StatementTimeout > 0 {
		setup = append(setup, fmt.Sprintf("SET LOCAL statement_timeout = %d", s.opts.StatementTimeout.Milliseconds()))
	}
	for _, q := range setup {
		if _, err := tx.ExecContext(ctx, q); err != nil {
			return err
		}
	}

	// No arguments: sent with the simple protocol, which accepts multiple statements.
	if _, err := tx.ExecContext(ctx, migrationSQL); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO public.`+LedgerTable+` (version) VALUES ($1)`, version); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Service) finish(ctx context.Context, migrationID uint, runErr error) error {
	status, msg, code := models.MigrationSuccess, "", ""
	if runErr != nil {
		status = models.MigrationFailed
		msg, code = failure(runErr)
	}
	now := time.Now()
	return s.platform.WithContext(ctx).Model(&models.Migration{}).Where("id = ?", migrationID).
		Updates(map[string]any{
			"status":        status,
			"error_message": msg,
			"error_code":    code,
			"finished_at":   now,
			"updated_at":    now,
		}).Error
}

// failure turns an execution error into a message and SQLSTATE for the history.
func failure(err error) (message, code string) {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		if pgErr.Code == "25001" {
			return msgNotInTransaction, pgErr.Code
		}
		message = "ERROR: " + pgErr.Message
		if pgErr.Detail != "" {
			message += "\nDETAIL: " + pgErr.Detail
		}
		if pgErr.Hint != "" {
			message += "\nHINT: " + pgErr.Hint
		}
		return message, pgErr.Code
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "Eksekusi melebihi batas waktu; perubahan di-rollback.", ""
	}
	return "Koneksi ke database target bermasalah; perubahan di-rollback.", ""
}
