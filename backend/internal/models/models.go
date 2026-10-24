// Package models holds the GORM models of the platform database.
//
// Rules for this package:
//   - Never embed gorm.Model or gorm.DeletedAt: every delete is a hard delete.
//   - Foreign-key behaviour is declared explicitly with constraint tags.
//   - Secret fields carry json:"-" so they can never be serialised by accident.
package models

import "time"

// RolePlatformAdmin is the only platform role for now.
const RolePlatformAdmin = "platform_admin"

type User struct {
	ID           uint   `gorm:"primaryKey"`
	Username     string `gorm:"size:64;not null;uniqueIndex"`
	PasswordHash string `gorm:"not null" json:"-"`
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type Role struct {
	ID   uint   `gorm:"primaryKey"`
	Name string `gorm:"size:64;not null;uniqueIndex"`
}

type UserRole struct {
	UserID uint `gorm:"primaryKey"`
	RoleID uint `gorm:"primaryKey"`
	User   User `gorm:"constraint:OnDelete:CASCADE"`
	Role   Role `gorm:"constraint:OnDelete:CASCADE"`
}

type Group struct {
	ID          uint   `gorm:"primaryKey"`
	Name        string `gorm:"size:64;not null;uniqueIndex"`
	Description string `gorm:"not null;default:''"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type GroupMember struct {
	GroupID   uint `gorm:"primaryKey"`
	UserID    uint `gorm:"primaryKey"`
	CreatedAt time.Time
	Group     Group `gorm:"constraint:OnDelete:CASCADE"`
	User      User  `gorm:"constraint:OnDelete:CASCADE"`
}

// DatabaseConnection is a target PostgreSQL database managed by the platform.
type DatabaseConnection struct {
	ID       uint   `gorm:"primaryKey"`
	Name     string `gorm:"size:64;not null;uniqueIndex"`
	Host     string `gorm:"not null"`
	Port     int    `gorm:"not null"`
	DBName   string `gorm:"column:db_name;not null"`
	Username string `gorm:"not null"`
	// PasswordEnc is the AES-256-GCM encrypted password (nonce + ciphertext).
	PasswordEnc []byte `gorm:"not null" json:"-"`
	SSLMode     string `gorm:"size:16;not null;default:disable"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Grant gives a group an access level on one database. One row per (group, database).
type Grant struct {
	GroupID    uint `gorm:"primaryKey"`
	DatabaseID uint `gorm:"primaryKey"`
	// Level is authz.Level stored as an integer: 1 = VIEW, 2 = EDIT.
	Level     int16 `gorm:"not null;check:chk_grants_level,level IN (1, 2)"`
	CreatedAt time.Time
	UpdatedAt time.Time
	Group     Group              `gorm:"constraint:OnDelete:CASCADE"`
	Database  DatabaseConnection `gorm:"foreignKey:DatabaseID;constraint:OnDelete:CASCADE"`
}

type MigrationStatus string

const (
	MigrationDraft   MigrationStatus = "draft"
	MigrationRunning MigrationStatus = "running"
	MigrationSuccess MigrationStatus = "success"
	MigrationFailed  MigrationStatus = "failed"
)

// Migration is a draft or an executed migration of one target database.
//
// References to users use ON DELETE SET NULL plus a username snapshot, so history
// stays linear and still names the person after the user is deleted.
type Migration struct {
	ID         uint               `gorm:"primaryKey"`
	DatabaseID uint               `gorm:"not null;uniqueIndex:idx_migrations_database_version,priority:1"`
	Database   DatabaseConnection `gorm:"foreignKey:DatabaseID;constraint:OnDelete:CASCADE"`
	// Version is NULL while draft; assigned when execution starts.
	Version *int `gorm:"uniqueIndex:idx_migrations_database_version,priority:2"`

	SQL          string          `gorm:"column:sql;type:text;not null"`
	Status       MigrationStatus `gorm:"size:16;not null;default:draft;index;check:chk_migrations_status,status IN ('draft', 'running', 'success', 'failed')"`
	ErrorMessage string          `gorm:"type:text;not null;default:''"`
	// ErrorCode is the PostgreSQL SQLSTATE of a failed execution.
	ErrorCode string `gorm:"size:5;not null;default:''"`

	// BaseVersion is the latest successful version when the draft was created.
	BaseVersion *int
	// Revision is bumped on every draft save (optimistic locking).
	Revision int `gorm:"not null;default:1"`

	CreatedByID        *uint
	CreatedBy          *User  `gorm:"foreignKey:CreatedByID;constraint:OnDelete:SET NULL"`
	CreatedByUsername  string `gorm:"size:64;not null"`
	UpdatedByID        *uint
	UpdatedBy          *User  `gorm:"foreignKey:UpdatedByID;constraint:OnDelete:SET NULL"`
	UpdatedByUsername  string `gorm:"size:64;not null;default:''"`
	ExecutedByID       *uint
	ExecutedBy         *User  `gorm:"foreignKey:ExecutedByID;constraint:OnDelete:SET NULL"`
	ExecutedByUsername string `gorm:"size:64;not null;default:''"`

	CreatedAt  time.Time
	UpdatedAt  time.Time
	StartedAt  *time.Time
	FinishedAt *time.Time
}
