// Package authz is the single place that decides who may do what on a target database
// (the Policy Decision Point). Other packages only call Can; they never compare levels themselves.
//
// Effective access = the highest level among all of the user's groups that have a grant on the
// database. No grant means no access (deny by default). Platform Admin gives no database access.
package authz

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"
)

// Level is an access level on a database. Stored as an integer so that MAX() orders correctly.
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

// MarshalText makes levels appear as "VIEW" / "EDIT" in JSON.
func (l Level) MarshalText() ([]byte, error) { return []byte(l.String()), nil }

// UnmarshalText accepts only grantable levels ("VIEW", "EDIT").
func (l *Level) UnmarshalText(b []byte) error {
	v, err := ParseLevel(string(b))
	if err != nil {
		return err
	}
	*l = v
	return nil
}

// ParseLevel parses a grantable level. NONE is not grantable: revoking is deleting the grant.
func ParseLevel(s string) (Level, error) {
	switch s {
	case "VIEW":
		return View, nil
	case "EDIT":
		return Edit, nil
	}
	return None, fmt.Errorf("authz: invalid level %q (want VIEW or EDIT)", s)
}

// Action is something a user can do on a target database.
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

// RequiredLevel returns the minimum level for action; ok is false for unknown actions.
func RequiredLevel(action Action) (level Level, ok bool) {
	level, ok = required[action]
	return level, ok
}

// ErrNotImplemented is returned until EffectiveLevel is implemented.
var ErrNotImplemented = errors.New("authz: EffectiveLevel not implemented")

// Service answers access questions from the platform database.
type Service struct {
	db *gorm.DB
}

func New(db *gorm.DB) *Service {
	return &Service{db: db}
}

// EffectiveLevel returns the highest level the user holds on the database through any group,
// or None when no group of the user has a grant on it.
//
// TODO(B): SELECT MAX(grants.level) FROM grants JOIN group_members ... (PRD §9).
func (s *Service) EffectiveLevel(ctx context.Context, userID, databaseID uint) (Level, error) {
	return None, ErrNotImplemented
}

// Can reports whether the user may perform action on the database.
// Unknown actions and lookup errors are denied.
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
