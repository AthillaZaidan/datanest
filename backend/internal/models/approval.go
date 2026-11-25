package models

import "time"

// MigrationApproval is append-only: withdrawing fills RevokedAt.
type MigrationApproval struct {
	ID          uint               `gorm:"primaryKey"`
	ExecutionID uint               `gorm:"not null;index"`
	Execution   MigrationExecution `gorm:"foreignKey:ExecutionID;constraint:OnDelete:CASCADE"`
	UserID      *uint              `gorm:"index"`
	User        *User              `gorm:"foreignKey:UserID;constraint:OnDelete:SET NULL"`
	Username    string             `gorm:"size:64;not null"`
	CreatedAt   time.Time          `gorm:"not null"`
	RevokedAt   *time.Time
}
