package migration

import (
	"context"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/AthillaZaidan/datanest/backend/internal/apperr"
	"github.com/AthillaZaidan/datanest/backend/internal/authz"
	"github.com/AthillaZaidan/datanest/backend/internal/dbconn"
	"github.com/AthillaZaidan/datanest/backend/internal/models"
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
	locks    *DBLocks
	opts     Options
}

func New(platform *gorm.DB, targets dbconn.TargetDBProvider, az Authorizer, opts Options) *Service {
	return &Service{platform: platform, targets: targets, authz: az, locks: NewDBLocks(), opts: opts}
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
	current, err := s.currentVersion(ctx, databaseID)
	if err != nil {
		return nil, err
	}
	out := &ListResult{Migrations: make([]View, len(rows)), CurrentVersion: current}
	for i := range rows {
		out.Migrations[i] = toView(&rows[i], current)
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

func (s *Service) Create(ctx context.Context, userID, databaseID uint, sql string) (*View, error) {
	if err := s.requireDatabase(ctx, databaseID); err != nil {
		return nil, err
	}
	if err := s.authorize(ctx, userID, databaseID, authz.CreateMigration); err != nil {
		return nil, err
	}
	if err := validateSQL(sql); err != nil {
		return nil, err
	}
	return s.createDraft(ctx, userID, databaseID, sql)
}

func (s *Service) Update(ctx context.Context, userID, migrationID uint, sql string, revision int) (*View, error) {
	m, err := s.loadAuthorized(ctx, userID, migrationID, authz.UpdateMigration)
	if err != nil {
		return nil, err
	}
	if m.Status != models.MigrationDraft {
		return nil, apperr.ErrMigrationNotDraft
	}
	if err := validateSQL(sql); err != nil {
		return nil, err
	}
	u, err := s.user(ctx, userID)
	if err != nil {
		return nil, err
	}

	res := s.platform.WithContext(ctx).Model(&models.Migration{}).
		Where("id = ? AND status = ? AND revision = ?", migrationID, models.MigrationDraft, revision).
		Updates(map[string]any{
			"sql":                 sql,
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
		if m.Status != models.MigrationDraft {
			return nil, apperr.ErrMigrationNotDraft
		}
		return nil, apperr.ErrRevisionConflict
	}
	return s.Get(ctx, userID, migrationID)
}

func (s *Service) Delete(ctx context.Context, userID, migrationID uint) error {
	if _, err := s.loadAuthorized(ctx, userID, migrationID, authz.DeleteMigration); err != nil {
		return err
	}
	res := s.platform.WithContext(ctx).Where("id = ? AND status = ?", migrationID, models.MigrationDraft).
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
	return s.createDraft(ctx, userID, m.DatabaseID, m.SQL)
}

func (s *Service) createDraft(ctx context.Context, userID, databaseID uint, sql string) (*View, error) {
	u, err := s.user(ctx, userID)
	if err != nil {
		return nil, err
	}
	current, err := s.currentVersion(ctx, databaseID)
	if err != nil {
		return nil, err
	}
	m := models.Migration{
		DatabaseID:        databaseID,
		SQL:               sql,
		Status:            models.MigrationDraft,
		Revision:          1,
		CreatedByID:       &u.ID,
		CreatedByUsername: u.Username,
	}
	if current > 0 {
		m.BaseVersion = &current
	}
	if err := s.platform.WithContext(ctx).Create(&m).Error; err != nil {
		return nil, err
	}
	v := toView(&m, current)
	return &v, nil
}

func (s *Service) view(ctx context.Context, m *models.Migration) (*View, error) {
	current, err := s.currentVersion(ctx, m.DatabaseID)
	if err != nil {
		return nil, err
	}
	v := toView(m, current)
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

// currentVersion is the latest successful version, or 0.
func (s *Service) currentVersion(ctx context.Context, databaseID uint) (int, error) {
	var v int
	err := s.platform.WithContext(ctx).Model(&models.Migration{}).
		Where("database_id = ? AND status = ?", databaseID, models.MigrationSuccess).
		Select("COALESCE(MAX(version), 0)").Scan(&v).Error
	return v, err
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

func validateSQL(sql string) error {
	if strings.TrimSpace(sql) == "" {
		return apperr.ErrValidation.WithMessage("SQL migration wajib diisi.").
			WithDetails(map[string]string{"sql": "wajib diisi"})
	}
	return nil
}
