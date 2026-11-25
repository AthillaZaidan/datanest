// Package models holds the platform database models.
// No gorm.Model / DeletedAt: every delete is a hard delete.
package models

import "time"

const RolePlatformAdmin = "platform_admin"

type User struct {
	ID           uint   `gorm:"primaryKey"`
	Username     string `gorm:"size:64;not null;uniqueIndex"`
	PasswordHash string `gorm:"not null" json:"-"`
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// Session is one login. Identifiers are stored only as SHA-256 hashes. While a rotation is
// in its grace period the row holds two: the current one and the previous one, and
// SuccessorEnc is the encrypted raw current identifier, so a request that still carries the
// previous one can be handed its replacement again.
type Session struct {
	ID     uint `gorm:"primaryKey"`
	UserID uint `gorm:"not null;index"`
	User   User `gorm:"constraint:OnDelete:CASCADE"`

	CurrentHash []byte `gorm:"type:bytea;not null;uniqueIndex"`
	// All three are NULL outside a grace period.
	PreviousHash []byte     `gorm:"type:bytea;uniqueIndex"`
	GraceUntil   *time.Time `gorm:"index"`
	SuccessorEnc []byte     `gorm:"type:bytea"`

	CSRFToken     string `gorm:"not null"`
	NeedsRotation bool   `gorm:"not null;default:false"`

	CreatedAt     time.Time `gorm:"not null"`
	LastSeen      time.Time `gorm:"not null"`
	LastRotatedAt time.Time `gorm:"not null"`
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
	// A group without a leader blocks approval.
	LeaderID  *uint `gorm:"index"`
	Leader    *User `gorm:"foreignKey:LeaderID;constraint:OnDelete:SET NULL"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

type GroupMember struct {
	GroupID   uint `gorm:"primaryKey"`
	UserID    uint `gorm:"primaryKey"`
	CreatedAt time.Time
	Group     Group `gorm:"constraint:OnDelete:CASCADE"`
	User      User  `gorm:"constraint:OnDelete:CASCADE"`
}

type DatabaseConnection struct {
	ID       uint   `gorm:"primaryKey"`
	Name     string `gorm:"size:64;not null;uniqueIndex"`
	Host     string `gorm:"not null"`
	Port     int    `gorm:"not null"`
	DBName   string `gorm:"column:db_name;not null"`
	Username string `gorm:"not null"`
	// AES-256-GCM: nonce + ciphertext.
	PasswordEnc []byte `gorm:"not null" json:"-"`
	SSLMode     string `gorm:"size:16;not null;default:disable"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type Grant struct {
	GroupID    uint `gorm:"primaryKey"`
	DatabaseID uint `gorm:"primaryKey"`
	// authz.Level: 1 = VIEW, 2 = EDIT.
	Level     int16 `gorm:"not null;check:chk_grants_level,level IN (1, 2)"`
	CreatedAt time.Time
	UpdatedAt time.Time
	Group     Group              `gorm:"constraint:OnDelete:CASCADE"`
	Database  DatabaseConnection `gorm:"foreignKey:DatabaseID;constraint:OnDelete:CASCADE"`
}

type DatabaseLock struct {
	DatabaseID  uint               `gorm:"primaryKey;autoIncrement:false"`
	Database    DatabaseConnection `gorm:"foreignKey:DatabaseID;constraint:OnDelete:CASCADE"`
	ExecutionID uint               `gorm:"not null"`
	Owner       string             `gorm:"size:128;not null"`
	Token       int64              `gorm:"not null"`
	LeaseUntil  time.Time          `gorm:"not null"`
}

type MigrationStatus string

const (
	MigrationDraft   MigrationStatus = "draft"
	MigrationRunning MigrationStatus = "running"
	MigrationSuccess MigrationStatus = "success"
	MigrationFailed  MigrationStatus = "failed"
)

// User references use SET NULL + a username snapshot so history survives user deletion.
type Migration struct {
	ID         uint               `gorm:"primaryKey"`
	DatabaseID uint               `gorm:"not null;uniqueIndex:idx_migrations_database_version,priority:1"`
	Database   DatabaseConnection `gorm:"foreignKey:DatabaseID;constraint:OnDelete:CASCADE"`
	// NULL while draft.
	Version *int `gorm:"uniqueIndex:idx_migrations_database_version,priority:2"`

	// SQL is the up SQL. The column keeps its Milestone 1 name.
	SQL     string `gorm:"column:sql;type:text;not null"`
	SQLDown string `gorm:"column:sql_down;type:text;not null;default:''"`
	// An irreversible migration has no down SQL and can never be rolled back.
	Irreversible       bool          `gorm:"not null;default:false"`
	IrreversibleReason string        `gorm:"type:text;not null;default:''"`
	Mode               ExecutionMode `gorm:"size:8;not null;default:normal;check:chk_migrations_mode,mode IN ('normal', 'batch')"`
	BatchTable         string        `gorm:"not null;default:''"`
	BatchColumn        string        `gorm:"not null;default:''"`
	// FrozenAt is set when the first execution starts running. From then on the SQL
	// is immutable; a fix is a new migration.
	FrozenAt *time.Time

	Status       MigrationStatus `gorm:"size:16;not null;default:draft;index;check:chk_migrations_status,status IN ('draft', 'running', 'success', 'failed')"`
	ErrorMessage string          `gorm:"type:text;not null;default:''"`
	// SQLSTATE.
	ErrorCode string `gorm:"size:5;not null;default:''"`

	BaseVersion *int
	// Optimistic lock for draft edits.
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
