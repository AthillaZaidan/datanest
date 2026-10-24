// Package explorer reads schema and data from target databases. Every query runs in a
// READ ONLY transaction, so this path can never modify a target database.
package explorer

import (
	"context"
	"database/sql"
	"time"

	"gorm.io/gorm"

	"github.com/AthillaZaidan/datanest/backend/internal/apperr"
	"github.com/AthillaZaidan/datanest/backend/internal/authz"
	"github.com/AthillaZaidan/datanest/backend/internal/dbconn"
	"github.com/AthillaZaidan/datanest/backend/internal/models"
	"github.com/AthillaZaidan/datanest/backend/internal/targeterr"
)

type Authorizer interface {
	Can(ctx context.Context, userID, databaseID uint, action authz.Action) (bool, error)
}

type Service struct {
	platform *gorm.DB
	targets  dbconn.TargetDBProvider
	authz    Authorizer
	timeout  time.Duration
}

func New(platform *gorm.DB, targets dbconn.TargetDBProvider, az Authorizer, timeout time.Duration) *Service {
	return &Service{platform: platform, targets: targets, authz: az, timeout: timeout}
}

var errDatabaseNotFound = apperr.ErrNotFound.WithMessage("Database tidak ditemukan.")

// readOnly checks access, then runs fn in a READ ONLY transaction on the target database.
func (s *Service) readOnly(ctx context.Context, userID, databaseID uint, action authz.Action, fn func(context.Context, *sql.Tx) error) error {
	var count int64
	err := s.platform.WithContext(ctx).Model(&models.DatabaseConnection{}).Where("id = ?", databaseID).Count(&count).Error
	if err != nil {
		return err
	}
	if count == 0 {
		return errDatabaseNotFound
	}

	ok, err := s.authz.Can(ctx, userID, databaseID, action)
	if err != nil {
		return err
	}
	if !ok {
		return apperr.ErrForbidden
	}

	db, err := s.targets.Get(ctx, databaseID)
	if err != nil {
		return targeterr.Map(err)
	}

	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return targeterr.Map(err)
	}
	defer tx.Rollback()
	return targeterr.Map(fn(ctx, tx))
}
