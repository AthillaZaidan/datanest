// Package authz decides who may do what on a target database. Other packages only call Can.
package authz

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"
)

// Level is stored as an integer so MAX() orders correctly.
type Level int16

const (
	None Level = 0
	View Level = 1
	Edit Level = 2
)

func (l Level) String() string {
	switch l {
	case None:
		return "NONE"
	case View:
		return "VIEW"
	case Edit:
		return "EDIT"
	}
	return fmt.Sprintf("Level(%d)", int16(l))
}

func (l Level) MarshalText() ([]byte, error) { return []byte(l.String()), nil }

func (l *Level) UnmarshalText(b []byte) error {
	v, err := ParseLevel(string(b))
	if err != nil {
		return err
	}
	*l = v
	return nil
}

// ParseLevel accepts only grantable levels; NONE is expressed by deleting the grant.
func ParseLevel(s string) (Level, error) {
	switch s {
	case "VIEW":
		return View, nil
	case "EDIT":
		return Edit, nil
	}
	return None, fmt.Errorf("authz: invalid level %q (want VIEW or EDIT)", s)
}

type Action string

const (
	ViewDatabase       Action = "view_database"
	ViewSchema         Action = "view_schema"
	ViewData           Action = "view_data"
	ViewMigrations     Action = "view_migrations"
	CreateMigration    Action = "create_migration"
	UpdateMigration    Action = "update_migration"
	DeleteMigration    Action = "delete_migration"
	DuplicateMigration Action = "duplicate_migration"
	ExecuteMigration   Action = "execute_migration"
)

var required = map[Action]Level{
	ViewDatabase:       View,
	ViewSchema:         View,
	ViewData:           View,
	ViewMigrations:     View,
	CreateMigration:    Edit,
	UpdateMigration:    Edit,
	DeleteMigration:    Edit,
	DuplicateMigration: Edit,
	ExecuteMigration:   Edit,
}

func RequiredLevel(action Action) (level Level, ok bool) {
	level, ok = required[action]
	return level, ok
}

var ErrNotImplemented = errors.New("authz: EffectiveLevel not implemented")

type Service struct {
	db *gorm.DB
}

func New(db *gorm.DB) *Service {
	return &Service{db: db}
}

// EffectiveLevel is the highest level among the user's groups; None without a grant.
// TODO(B): implement (PRD §9).
func (s *Service) EffectiveLevel(ctx context.Context, userID, databaseID uint) (Level, error) {
	return None, ErrNotImplemented
}

// Can denies unknown actions and lookup errors.
func (s *Service) Can(ctx context.Context, userID, databaseID uint, action Action) (bool, error) {
	need, ok := RequiredLevel(action)
	if !ok {
		return false, nil
	}
	level, err := s.EffectiveLevel(ctx, userID, databaseID)
	if err != nil {
		return false, err
	}
	return level >= need, nil
}
