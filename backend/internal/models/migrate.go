package models

import "gorm.io/gorm"

// All lists every model in dependency order, for AutoMigrate and test resets.
func All() []any {
	return []any{
		&User{},
		&Role{},
		&UserRole{},
		&Group{},
		&GroupMember{},
		&DatabaseConnection{},
		&Grant{},
		&Migration{},
	}
}

// Migrate creates or updates the platform schema and ensures the built-in roles exist.
func Migrate(db *gorm.DB) error {
	if err := db.AutoMigrate(All()...); err != nil {
		return err
	}
	return db.Where(Role{Name: RolePlatformAdmin}).FirstOrCreate(&Role{}).Error
}
