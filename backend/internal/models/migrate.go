package models

import "gorm.io/gorm"

// All lists every model in dependency order.
func All() []any {
	return []any{
		&User{},
		&Session{},
		&Role{},
		&UserRole{},
		&Group{},
		&GroupMember{},
		&DatabaseConnection{},
		&Grant{},
		&Migration{},
		&DatabaseLock{},
		&MigrationExecution{},
		&ExecutionAttempt{},
		&MigrationApproval{},
	}
}

// Migrate updates the schema and ensures built-in roles exist.
func Migrate(db *gorm.DB) error {
	if err := db.AutoMigrate(All()...); err != nil {
		return err
	}
	// An execution is undone by at most one successful rollback.
	if err := db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_executions_reverted_once
		ON migration_executions (reverts_execution_id) WHERE kind = 'rollback' AND status = 'success'`).Error; err != nil {
		return err
	}
	// A person has at most one live approval per execution.
	if err := db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_approvals_live
		ON migration_approvals (execution_id, user_id) WHERE revoked_at IS NULL`).Error; err != nil {
		return err
	}
	if err := backfillM1History(db); err != nil {
		return err
	}
	if err := backfillRunSeq(db); err != nil {
		return err
	}
	return db.Where(Role{Name: RolePlatformAdmin}).FirstOrCreate(&Role{}).Error
}

// M1Reason marks migrations created before down SQL existed.
const M1Reason = "Dibuat di Milestone 1, sebelum ada SQL down."

const M1InterruptedReason = "Eksekusi terputus sebelum Milestone 2."

// backfillM1History turns each executed Milestone 1 migration into a frozen,
// irreversible recipe plus one run execution, in version order, so the active position
// can be derived from executions. Rows that already have an execution are skipped, so
// it is safe to run on every start. Migrations still running were interrupted by the
// retired Milestone 1 path, which can no longer settle them, so they are failed first
// and recorded like the others; one without a version is failed and not recorded.
func backfillM1History(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		err := tx.Exec(`UPDATE migrations SET status = 'failed', error_message = ?,
			finished_at = COALESCE(finished_at, now()) WHERE status = 'running'`, M1InterruptedReason).Error
		if err != nil {
			return err
		}
		err = tx.Exec(`UPDATE migrations SET irreversible = true, irreversible_reason = ?,
			frozen_at = COALESCE(frozen_at, started_at, updated_at)
			WHERE sql_down = '' AND `+unrecordedRun, M1Reason).Error
		if err != nil {
			return err
		}
		return insertRunExecutions(tx, "TRUE")
	})
}

// unrecordedRun selects migrations that settled through the Milestone 1 path and
// have no execution yet.
const unrecordedRun = `version IS NOT NULL AND status IN ('success', 'failed')
	AND NOT EXISTS (SELECT 1 FROM migration_executions e WHERE e.migration_id = migrations.id)`

func insertRunExecutions(db *gorm.DB, filter string, args ...any) error {
	return db.Exec(`INSERT INTO migration_executions (migration_id, database_id, kind, status, mode, sql_executed,
			requested_by_id, requested_by_username, error_message, error_code, created_at, updated_at, started_at, finished_at)
		SELECT id, database_id, 'run', status, 'normal', sql,
			executed_by_id, executed_by_username, error_message, error_code,
			COALESCE(started_at, created_at), now(), started_at, finished_at
		FROM migrations WHERE `+unrecordedRun+` AND (`+filter+`)
		ORDER BY database_id, version`, args...).Error
}

func backfillRunSeq(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		return tx.Exec(`UPDATE migration_executions e SET run_seq = n.seq
			FROM (
				SELECT x.id, COALESCE(
						(SELECT MAX(r.run_seq) FROM migration_executions r WHERE r.database_id = x.database_id), 0)
					+ ROW_NUMBER() OVER (PARTITION BY x.database_id ORDER BY COALESCE(x.started_at, x.created_at), x.id) AS seq
				FROM migration_executions x
				WHERE x.run_seq IS NULL
					AND (x.started_at IS NOT NULL OR x.status IN ('running', 'success', 'failed', 'partial', 'uncertain'))
			) n WHERE e.id = n.id`).Error
	})
}
