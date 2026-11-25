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
	return db.Where(Role{Name: RolePlatformAdmin}).FirstOrCreate(&Role{}).Error
}
