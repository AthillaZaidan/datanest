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
	if err := backfillM1History(db); err != nil {
		return err
	}
	return db.Where(Role{Name: RolePlatformAdmin}).FirstOrCreate(&Role{}).Error
}

// M1Reason marks migrations created before down SQL existed.
const M1Reason = "Dibuat di Milestone 1, sebelum ada SQL down."

// backfillM1History turns each executed Milestone 1 migration into a frozen,
// irreversible recipe plus one run execution, in version order, so the active position
// can be derived from executions. Rows that already have an execution are skipped, so
// it is safe to run on every start. Migrations still running are left to Reconcile,
// which checks the target ledger before settling them.
func backfillM1History(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		err := tx.Exec(`UPDATE migrations SET irreversible = true, irreversible_reason = ?,
			frozen_at = COALESCE(frozen_at, started_at, updated_at)
			WHERE sql_down = '' AND `+unrecordedRun, M1Reason).Error
		if err != nil {
			return err
		}
		return insertRunExecutions(tx, "TRUE")
	})
}

// RecordLegacyRun records the result of the Milestone 1 direct execution of one
// migration as a run execution. It does nothing until the migration has settled and
// nothing if the run was recorded already.
func RecordLegacyRun(db *gorm.DB, migrationID uint) error {
	return insertRunExecutions(db, "id = ?", migrationID)
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
