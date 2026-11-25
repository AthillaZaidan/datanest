package migration

import (
	"context"
	"log/slog"

	"gorm.io/gorm"

	"github.com/AthillaZaidan/datanest/backend/internal/authz"
	"github.com/AthillaZaidan/datanest/backend/internal/migration/sqlguard"
	"github.com/AthillaZaidan/datanest/backend/internal/models"
	"github.com/AthillaZaidan/datanest/backend/internal/obs"
	"github.com/AthillaZaidan/datanest/backend/internal/position"
)

// Submit asks for an execution of a migration (design D3/D9): run for the first attempt
// that really starts, forward to apply it again or to continue a partial one, rollback to
// undo it (partial or not) or to continue a partial rollback. The version rules are checked
// now and again by the worker (Revalidate). The request starts as pending_review; approval
// and the queue take it from there.
func (s *Service) Submit(ctx context.Context, userID, migrationID uint, kind models.ExecutionKind) (*models.MigrationExecution, error) {
	m, err := s.loadAuthorized(ctx, userID, migrationID, authz.ExecuteMigration)
	if err != nil {
		return nil, err
	}
	u, err := s.user(ctx, userID)
	if err != nil {
		return nil, err
	}

	var out models.MigrationExecution
	err = s.platform.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// One request per database at a time, so two cannot pass the rules together.
		var locked []uint
		if err := tx.Raw(`SELECT id FROM database_connections WHERE id = ? FOR UPDATE`, m.DatabaseID).Scan(&locked).Error; err != nil {
			return err
		}
		if err := tx.First(m, m.ID).Error; err != nil {
			return err
		}
		st, versions, err := position.Load(ctx, tx, m.DatabaseID, 0)
		if err != nil {
			return err
		}
		plan, err := position.Decide(st, versions, m, kind)
		if err != nil {
			return err
		}
		if err := sqlguard.Check(plan.SQL); err != nil {
			return err
		}
		out = models.MigrationExecution{
			MigrationID: m.ID, DatabaseID: m.DatabaseID, Version: m.Version, Revision: m.Revision,
			Kind: plan.Kind, Status: models.ExecPendingReview, Mode: models.ExecModeNormal,
			SQLExecuted: plan.SQL, RevertsExecutionID: plan.Reverts, ContinuesExecutionID: plan.Continues,
			RequestedByID: &u.ID, RequestedByUsername: u.Username,
		}
		return tx.Create(&out).Error
	})
	if err != nil {
		return nil, err
	}
	slog.InfoContext(ctx, obs.EventExecutionRequested, append([]any{obs.AttrActorID, u.ID, obs.AttrActor, u.Username, obs.AttrMigrationID, m.ID,
		obs.AttrExecutionID, out.ID, obs.AttrDatabaseID, out.DatabaseID, "kind", string(out.Kind), obs.AttrStatus, string(out.Status)},
		obs.MigrationVersionAttrs(out.Version, out.Revision)...)...)
	return &out, nil
}
