package migration

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/AthillaZaidan/datanest/backend/internal/apperr"
	"github.com/AthillaZaidan/datanest/backend/internal/authz"
	"github.com/AthillaZaidan/datanest/backend/internal/dbconn"
	"github.com/AthillaZaidan/datanest/backend/internal/models"
	"github.com/AthillaZaidan/datanest/backend/internal/position"
)

type Authorizer interface {
	Can(ctx context.Context, userID, databaseID uint, action authz.Action) (bool, error)
}

type Options struct {
	LockTimeout      time.Duration
	StatementTimeout time.Duration
	ExecTimeout      time.Duration
}

type Service struct {
	platform *gorm.DB
	targets  dbconn.TargetDBProvider
	authz    Authorizer
	opts     Options
}

func New(platform *gorm.DB, targets dbconn.TargetDBProvider, az Authorizer, opts Options) *Service {
	return &Service{platform: platform, targets: targets, authz: az, opts: opts}
}

var (
	errDatabaseNotFound  = apperr.ErrNotFound.WithMessage("Database tidak ditemukan.")
	errMigrationNotFound = apperr.ErrNotFound.WithMessage("Migration tidak ditemukan.")
)

type ListResult struct {
	Migrations     []View `json:"migrations"`
	CurrentVersion int    `json:"currentVersion"`
}

func (s *Service) List(ctx context.Context, userID, databaseID uint, status string) (*ListResult, error) {
	if err := s.requireDatabase(ctx, databaseID); err != nil {
		return nil, err
	}
	if err := s.authorize(ctx, userID, databaseID, authz.ViewMigrations); err != nil {
		return nil, err
	}
	q := s.platform.WithContext(ctx).Where("database_id = ?", databaseID)
	if status != "" {
		switch models.MigrationStatus(status) {
		case models.MigrationDraft, models.MigrationRunning, models.MigrationSuccess, models.MigrationFailed:
			q = q.Where("status = ?", status)
		default:
			return nil, apperr.ErrValidation.WithDetails(map[string]string{"status": "status tidak dikenal"})
		}
	}
	var rows []models.Migration
	if err := q.Order("version DESC NULLS FIRST, id DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	p, err := s.position(ctx, databaseID)
	if err != nil {
		return nil, err
	}
	out := &ListResult{Migrations: make([]View, len(rows)), CurrentVersion: p.current}
	for i := range rows {
		out.Migrations[i] = toView(&rows[i], p)
	}
	return out, nil
}

func (s *Service) Get(ctx context.Context, userID, migrationID uint) (*View, error) {
	m, err := s.loadAuthorized(ctx, userID, migrationID, authz.ViewMigrations)
	if err != nil {
		return nil, err
	}
	return s.view(ctx, m)
}

func (s *Service) Create(ctx context.Context, userID, databaseID uint, r Recipe) (*View, error) {
	if err := s.requireDatabase(ctx, databaseID); err != nil {
		return nil, err
	}
	if err := s.authorize(ctx, userID, databaseID, authz.CreateMigration); err != nil {
		return nil, err
	}
	if err := r.validate(); err != nil {
		return nil, err
	}
	return s.createDraft(ctx, userID, databaseID, r)
}

func (s *Service) Update(ctx context.Context, userID, migrationID uint, r Recipe, revision int) (*View, error) {
	m, err := s.loadAuthorized(ctx, userID, migrationID, authz.UpdateMigration)
	if err != nil {
		return nil, err
	}
	if m.Status != models.MigrationDraft || m.FrozenAt != nil {
		return nil, apperr.ErrMigrationNotDraft
	}
	if err := r.validate(); err != nil {
		return nil, err
	}
	u, err := s.user(ctx, userID)
	if err != nil {
		return nil, err
	}

	res := s.platform.WithContext(ctx).Model(&models.Migration{}).
		Where("id = ? AND status = ? AND revision = ? AND frozen_at IS NULL AND NOT EXISTS (?)",
			migrationID, models.MigrationDraft, revision, openExecutions(s.platform, migrationID).Select("1")).
		Updates(map[string]any{
			"sql":                 r.SQLUp,
			"sql_down":            r.SQLDown,
			"irreversible":        r.Irreversible,
			"irreversible_reason": r.IrreversibleReason,
			"revision":            gorm.Expr("revision + 1"),
			"updated_by_id":       u.ID,
			"updated_by_username": u.Username,
		})
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		if err := s.platform.WithContext(ctx).First(m, migrationID).Error; err != nil {
			return nil, err
		}
		if m.Status != models.MigrationDraft || m.FrozenAt != nil {
			return nil, apperr.ErrMigrationNotDraft
		}
		open, err := s.hasOpenExecution(ctx, migrationID)
		if err != nil {
			return nil, err
		}
		if open {
			return nil, errOpenExecution
		}
		return nil, apperr.ErrRevisionConflict
	}
	return s.Get(ctx, userID, migrationID)
}

func (s *Service) Delete(ctx context.Context, userID, migrationID uint) error {
	if _, err := s.loadAuthorized(ctx, userID, migrationID, authz.DeleteMigration); err != nil {
		return err
	}
	// Executions cascade with their migration, so a migration with any history is kept.
	res := s.platform.WithContext(ctx).
		Where("id = ? AND status = ? AND NOT EXISTS (?)", migrationID, models.MigrationDraft,
			s.platform.Model(&models.MigrationExecution{}).Select("1").Where("migration_id = ?", migrationID)).
		Delete(&models.Migration{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return apperr.ErrMigrationNotDraft
	}
	return nil
}

// Duplicate copies a migration's SQL into a new draft, e.g. to fix a failed one.
func (s *Service) Duplicate(ctx context.Context, userID, migrationID uint) (*View, error) {
	m, err := s.loadAuthorized(ctx, userID, migrationID, authz.DuplicateMigration)
	if err != nil {
		return nil, err
	}
	return s.createDraft(ctx, userID, m.DatabaseID, Recipe{SQLUp: m.SQL, SQLDown: m.SQLDown,
		Irreversible: m.Irreversible, IrreversibleReason: m.IrreversibleReason})
}

func (s *Service) createDraft(ctx context.Context, userID, databaseID uint, r Recipe) (*View, error) {
	u, err := s.user(ctx, userID)
	if err != nil {
		return nil, err
	}
	p, err := s.position(ctx, databaseID)
	if err != nil {
		return nil, err
	}
	current := p.current
	m := models.Migration{
		DatabaseID:         databaseID,
		SQL:                r.SQLUp,
		SQLDown:            r.SQLDown,
		Irreversible:       r.Irreversible,
		IrreversibleReason: r.IrreversibleReason,
		Status:             models.MigrationDraft,
		Revision:           1,
		CreatedByID:        &u.ID,
		CreatedByUsername:  u.Username,
	}
	if current > 0 {
		m.BaseVersion = &current
	}
	if err := s.platform.WithContext(ctx).Create(&m).Error; err != nil {
		return nil, err
	}
	v := toView(&m, p)
	return &v, nil
}

func (s *Service) view(ctx context.Context, m *models.Migration) (*View, error) {
	p, err := s.position(ctx, m.DatabaseID)
	if err != nil {
		return nil, err
	}
	v := toView(m, p)
	return &v, nil
}

// loadAuthorized loads the migration first, so access is checked against the database it
// really belongs to (not one claimed by the caller).
func (s *Service) loadAuthorized(ctx context.Context, userID, migrationID uint, action authz.Action) (*models.Migration, error) {
	var m models.Migration
	err := s.platform.WithContext(ctx).First(&m, migrationID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errMigrationNotFound
	}
	if err != nil {
		return nil, err
	}
	if err := s.authorize(ctx, userID, m.DatabaseID, action); err != nil {
		return nil, err
	}
	return &m, nil
}

func (s *Service) authorize(ctx context.Context, userID, databaseID uint, action authz.Action) error {
	ok, err := s.authz.Can(ctx, userID, databaseID, action)
	if err != nil {
		return err
	}
	if !ok {
		return apperr.ErrForbidden
	}
	return nil
}

func (s *Service) requireDatabase(ctx context.Context, databaseID uint) error {
	var count int64
	err := s.platform.WithContext(ctx).Model(&models.DatabaseConnection{}).Where("id = ?", databaseID).Count(&count).Error
	if err != nil {
		return err
	}
	if count == 0 {
		return errDatabaseNotFound
	}
	return nil
}

// snapshot is a database's derived position as the views need it.
type snapshot struct {
	state    position.State
	versions map[uint]int
	current  int
	last     map[uint]LastExecution
}

func (s *Service) position(ctx context.Context, databaseID uint) (snapshot, error) {
	st, versions, err := position.Load(ctx, s.platform, databaseID, 0)
	if err != nil {
		return snapshot{}, err
	}
	last, err := s.lastExecutions(ctx, databaseID)
	if err != nil {
		return snapshot{}, err
	}
	return snapshot{state: st, versions: versions, current: position.ActiveVersion(st, versions), last: last}, nil
}

func (s *Service) lastExecutions(ctx context.Context, databaseID uint) (map[uint]LastExecution, error) {
	var rows []struct {
		MigrationID uint
		LastExecution
	}
	err := s.platform.WithContext(ctx).Raw(`SELECT DISTINCT ON (migration_id) migration_id, id, kind, status
		FROM migration_executions WHERE database_id = ? ORDER BY migration_id, id DESC`, databaseID).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	last := make(map[uint]LastExecution, len(rows))
	for _, r := range rows {
		last[r.MigrationID] = r.LastExecution
	}
	return last, nil
}

func (s *Service) user(ctx context.Context, userID uint) (*models.User, error) {
	var u models.User
	err := s.platform.WithContext(ctx).First(&u, userID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, apperr.ErrUnauthenticated
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (s *Service) hasOpenExecution(ctx context.Context, migrationID uint) (bool, error) {
	var n int64
	err := openExecutions(s.platform.WithContext(ctx), migrationID).Count(&n).Error
	return n > 0, err
}
