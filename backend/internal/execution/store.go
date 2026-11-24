package execution

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/AthillaZaidan/datanest/backend/internal/models"
)

var ErrNotFound = errors.New("execution: not found")

type Store struct {
	db *gorm.DB
}

func NewStore(db *gorm.DB) *Store {
	return &Store{db: db}
}

func seconds(d time.Duration) float64 { return d.Seconds() }

func (s *Store) Get(ctx context.Context, id uint) (*models.MigrationExecution, error) {
	var e models.MigrationExecution
	err := s.db.WithContext(ctx).First(&e, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &e, nil
}

func (s *Store) Claim(ctx context.Context, id uint, from models.ExecutionStatus, owner string) (*models.MigrationExecution, error) {
	var rows []models.MigrationExecution
	err := s.db.WithContext(ctx).Raw(`
		UPDATE migration_executions SET
			status = ?, worker_id = ?, attempts = attempts + 1, not_before = NULL,
			started_at = COALESCE(started_at, now()), updated_at = now()
		WHERE id = ? AND status = ? AND (not_before IS NULL OR not_before <= now())
		RETURNING *`,
		models.ExecRunning, owner, id, from,
	).Scan(&rows).Error
	if err != nil || len(rows) == 0 {
		return nil, err
	}
	return &rows[0], nil
}

func (s *Store) Postpone(ctx context.Context, id uint, delay time.Duration) error {
	return s.db.WithContext(ctx).Exec(`
		UPDATE migration_executions SET not_before = now() + make_interval(secs => ?::float8), published_at = NULL, updated_at = now()
		WHERE id = ? AND status IN (?, ?)`,
		seconds(delay), id, models.ExecQueued, models.ExecUncertain,
	).Error
}

func (s *Store) MarkPublished(ctx context.Context, id uint) error {
	return s.db.WithContext(ctx).Exec(
		`UPDATE migration_executions SET published_at = now(), updated_at = now() WHERE id = ?`, id,
	).Error
}

func (s *Store) Requeue(ctx context.Context, id uint, owner string, delay time.Duration, code, message string) (bool, error) {
	res := s.db.WithContext(ctx).Exec(`
		UPDATE migration_executions SET
			status = ?, not_before = now() + make_interval(secs => ?::float8), published_at = NULL,
			error_code = ?, error_message = ?, updated_at = now()
		WHERE id = ? AND status = ? AND worker_id = ?`,
		models.ExecQueued, seconds(delay), code, message, id, models.ExecRunning, owner,
	)
	return res.RowsAffected == 1, res.Error
}

func (s *Store) MarkUncertain(ctx context.Context, id uint, owner string, retryIn time.Duration, code, message string) (bool, error) {
	res := s.db.WithContext(ctx).Exec(`
		UPDATE migration_executions SET
			status = ?, not_before = now() + make_interval(secs => ?::float8), published_at = NULL,
			error_code = ?, error_message = ?, updated_at = now()
		WHERE id = ? AND status = ? AND worker_id = ?`,
		models.ExecUncertain, seconds(retryIn), code, message, id, models.ExecRunning, owner,
	)
	return res.RowsAffected == 1, res.Error
}

func (s *Store) Finish(ctx context.Context, id uint, owner string, status models.ExecutionStatus, code, message string, batches int, rows int64) (bool, error) {
	res := s.db.WithContext(ctx).Exec(`
		UPDATE migration_executions SET
			status = ?, error_code = ?, error_message = ?, committed_batches = ?, committed_rows = ?,
			finished_at = now(), not_before = NULL, updated_at = now()
		WHERE id = ? AND status = ? AND worker_id = ?`,
		status, code, message, batches, rows, id, models.ExecRunning, owner,
	)
	return res.RowsAffected == 1, res.Error
}

func (s *Store) SetProgress(ctx context.Context, id uint, owner string, batches int, rows int64, cursor string) (bool, error) {
	res := s.db.WithContext(ctx).Exec(`
		UPDATE migration_executions SET committed_batches = ?, committed_rows = ?, last_cursor = ?, updated_at = now()
		WHERE id = ? AND status = ? AND worker_id = ?`,
		batches, rows, cursor, id, models.ExecRunning, owner,
	)
	return res.RowsAffected == 1, res.Error
}

func (s *Store) SetScope(ctx context.Context, id uint, owner string, scopeMax string, estimatedRows int64) error {
	return s.db.WithContext(ctx).Exec(`
		UPDATE migration_executions SET scope_max = ?, estimated_rows = ?, updated_at = now()
		WHERE id = ? AND status = ? AND worker_id = ? AND scope_max IS NULL`,
		scopeMax, estimatedRows, id, models.ExecRunning, owner,
	).Error
}

func (s *Store) StartAttempt(ctx context.Context, e *models.MigrationExecution, owner string) error {
	return s.db.WithContext(ctx).Exec(`
		INSERT INTO execution_attempts (execution_id, number, status, worker_id, sql_executed, started_at)
		VALUES (?, ?, ?, ?, ?, now())`,
		e.ID, e.Attempts, models.AttemptRunning, owner, e.SQLExecuted,
	).Error
}

func (s *Store) FinishAttempt(ctx context.Context, executionID uint, number int, status models.AttemptStatus, code, message string) error {
	return s.db.WithContext(ctx).Exec(`
		UPDATE execution_attempts SET status = ?, error_code = ?, error_message = ?, finished_at = now()
		WHERE execution_id = ? AND number = ?`,
		status, code, message, executionID, number,
	).Error
}

func (s *Store) Attempts(ctx context.Context, executionID uint) ([]models.ExecutionAttempt, error) {
	var out []models.ExecutionAttempt
	err := s.db.WithContext(ctx).Where("execution_id = ?", executionID).Order("number").Find(&out).Error
	return out, err
}

func (s *Store) Blockers(ctx context.Context, e *models.MigrationExecution) ([]models.MigrationExecution, error) {
	var out []models.MigrationExecution
	err := s.db.WithContext(ctx).Raw(`
		SELECT x.* FROM migration_executions x
		WHERE x.database_id = ? AND x.id <> ?
			AND (
				x.status IN (?, ?)
				OR (
					x.status = ?
					AND x.id <> COALESCE(?::bigint, 0)
					AND x.id <> COALESCE(?::bigint, 0)
					AND NOT EXISTS (
						SELECT 1 FROM migration_executions c
						WHERE (c.continues_execution_id = x.id OR c.reverts_execution_id = x.id)
							AND c.status IN (?, ?)
					)
				)
			)
		ORDER BY x.id`,
		e.DatabaseID, e.ID, models.ExecRunning, models.ExecUncertain,
		models.ExecPartial, e.ContinuesExecutionID, e.RevertsExecutionID,
		models.ExecSuccess, models.ExecPartial,
	).Scan(&out).Error
	return out, err
}

func (s *Store) DueForPublish(ctx context.Context, republishAfter time.Duration, limit int) ([]models.MigrationExecution, error) {
	var out []models.MigrationExecution
	err := s.db.WithContext(ctx).Raw(`
		SELECT e.* FROM migration_executions e
		WHERE e.status IN (?, ?)
			AND (e.not_before IS NULL OR e.not_before <= now())
			AND (e.published_at IS NULL OR e.published_at < now() - make_interval(secs => ?::float8))
			AND NOT EXISTS (SELECT 1 FROM database_locks l WHERE l.execution_id = e.id AND l.lease_until > now())
		ORDER BY e.id LIMIT ?`,
		models.ExecQueued, models.ExecUncertain, seconds(republishAfter), limit,
	).Scan(&out).Error
	return out, err
}

func (s *Store) MarkDeadRunning(ctx context.Context) ([]uint, error) {
	var ids []uint
	err := s.db.WithContext(ctx).Raw(`
		UPDATE migration_executions e SET
			status = ?, not_before = now(), published_at = NULL, error_code = 'WORKER_LOST',
			error_message = 'Worker berhenti tanpa melaporkan hasil; menunggu pemeriksaan ledger.', updated_at = now()
		WHERE e.status = ?
			AND NOT EXISTS (SELECT 1 FROM database_locks l WHERE l.execution_id = e.id AND l.lease_until > now())
		RETURNING e.id`,
		models.ExecUncertain, models.ExecRunning,
	).Scan(&ids).Error
	if err != nil || len(ids) == 0 {
		return ids, err
	}
	err = s.db.WithContext(ctx).Exec(`
		UPDATE execution_attempts SET status = ?, error_code = 'WORKER_LOST', finished_at = now()
		WHERE execution_id IN ? AND status = ?`,
		models.AttemptUncertain, ids, models.AttemptRunning,
	).Error
	return ids, err
}
