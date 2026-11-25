package approval

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/AthillaZaidan/datanest/backend/internal/apperr"
	"github.com/AthillaZaidan/datanest/backend/internal/authz"
	"github.com/AthillaZaidan/datanest/backend/internal/models"
	"github.com/AthillaZaidan/datanest/backend/internal/obs"
	"github.com/AthillaZaidan/datanest/backend/internal/queue"
)

type Authorizer interface {
	Can(ctx context.Context, userID, databaseID uint, action authz.Action) (bool, error)
}

type Publisher interface {
	Publish(ctx context.Context, job queue.Job) error
}

type Service struct {
	db        *gorm.DB
	authz     Authorizer
	Publisher Publisher
}

func NewService(db *gorm.DB, az Authorizer) *Service {
	return &Service{db: db, authz: az}
}

var errExecutionNotFound = apperr.ErrNotFound.WithMessage("Eksekusi tidak ditemukan.")

func conflict(msg string) error { return apperr.ErrExecutionStateConflict.WithMessage(msg) }

// lockExecution takes the row lock so concurrent reviews of one execution queue up.
func lockExecution(tx *gorm.DB, id uint) (*models.MigrationExecution, error) {
	var e models.MigrationExecution
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&e, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errExecutionNotFound
	}
	return &e, err
}

// transition is a conditional update; it reports whether this caller won.
func transition(tx *gorm.DB, id uint, from, to models.ExecutionStatus, set map[string]any) (bool, error) {
	set["status"] = to
	set["updated_at"] = gorm.Expr("now()")
	res := tx.Model(&models.MigrationExecution{}).Where("id = ? AND status = ?", id, from).Updates(set)
	return res.RowsAffected == 1, res.Error
}

func (s *Service) user(ctx context.Context, id uint) (*models.User, error) {
	var u models.User
	if err := s.db.WithContext(ctx).First(&u, id).Error; err != nil {
		return nil, err
	}
	return &u, nil
}

func (s *Service) requireApprover(ctx context.Context, tx *gorm.DB, userID uint, e *models.MigrationExecution) ([]Person, []GroupRef, error) {
	ok, err := s.authz.Can(ctx, userID, e.DatabaseID, authz.ApproveMigration)
	if err != nil {
		return nil, nil, err
	}
	if !ok {
		return nil, nil, apperr.ErrForbidden
	}
	approvers, blocking, err := Approvers(ctx, tx, e.DatabaseID)
	if err != nil {
		return nil, nil, err
	}
	if !isApprover(approvers, userID) {
		return nil, nil, apperr.ErrNotAnApprover
	}
	return approvers, blocking, nil
}

func (s *Service) Approve(ctx context.Context, userID, executionID uint) (*models.MigrationExecution, error) {
	ctx, span := obs.Start(ctx, obs.SpanAPIApprove,
		attribute.Int("execution_id", int(executionID)), attribute.Int("actor_id", int(userID)))
	defer span.End()

	u, err := s.user(ctx, userID)
	if err != nil {
		return nil, err
	}
	var out *models.MigrationExecution
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		e, err := lockExecution(tx, executionID)
		if err != nil {
			return err
		}
		approvers, blocking, err := s.requireApprover(ctx, tx, userID, e)
		if err != nil {
			return err
		}
		if e.Status != models.ExecPendingReview {
			return conflict("Eksekusi ini tidak sedang menunggu persetujuan.")
		}
		if len(blocking) > 0 {
			return groupsWithoutLeader(blocking)
		}
		approved, err := liveApprovals(ctx, tx, e.ID)
		if err != nil {
			return err
		}
		if approved[userID] {
			return conflict("Anda sudah menyetujui eksekusi ini.")
		}
		if err := tx.Create(&models.MigrationApproval{ExecutionID: e.ID, UserID: &u.ID, Username: u.Username, CreatedAt: time.Now()}).Error; err != nil {
			return err
		}
		approved[userID] = true
		if complete(approvers, blocking, approved) {
			won, err := transition(tx, e.ID, models.ExecPendingReview, models.ExecApproved, map[string]any{})
			if err != nil {
				return err
			}
			if won {
				e.Status = models.ExecApproved
			}
		}
		out = e
		return nil
	})
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	slog.InfoContext(ctx, obs.EventExecutionApproved, obs.AttrActorID, userID, obs.AttrActor, u.Username,
		obs.AttrMigrationID, out.MigrationID, obs.AttrExecutionID, out.ID, obs.AttrJobID, out.ID, obs.AttrDatabaseID, out.DatabaseID,
		obs.AttrStatus, string(out.Status))

	if out.Status == models.ExecApproved {
		return s.Promote(ctx, out.ID)
	}
	return out, nil
}

func groupsWithoutLeader(blocking []GroupRef) error {
	names := make([]string, len(blocking))
	for i, g := range blocking {
		names[i] = g.Name
	}
	return apperr.ErrGroupWithoutLeader.
		WithMessage("Group " + strings.Join(names, ", ") + " belum punya leader. Minta admin menetapkan leader terlebih dahulu.").
		WithDetails(map[string]any{"groups": blocking})
}

// Promote re-checks the approver list, then queues the execution and stores the trace context and correlation ID on the row.
func (s *Service) Promote(ctx context.Context, executionID uint) (*models.MigrationExecution, error) {
	correlationID := obs.CorrelationID(ctx)
	if correlationID == "" {
		correlationID = obs.NewCorrelationID()
		ctx = obs.WithCorrelationID(ctx, correlationID)
	}
	traceparent := obs.Traceparent(ctx)

	var out *models.MigrationExecution
	queued := false
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		e, err := lockExecution(tx, executionID)
		if err != nil {
			return err
		}
		out = e
		if e.Status != models.ExecApproved {
			return nil
		}
		approvers, blocking, err := Approvers(ctx, tx, e.DatabaseID)
		if err != nil {
			return err
		}
		approved, err := liveApprovals(ctx, tx, e.ID)
		if err != nil {
			return err
		}
		if !complete(approvers, blocking, approved) {
			won, err := transition(tx, e.ID, models.ExecApproved, models.ExecPendingReview, map[string]any{})
			if won {
				e.Status = models.ExecPendingReview
			}
			return err
		}
		won, err := transition(tx, e.ID, models.ExecApproved, models.ExecQueued, map[string]any{
			"queued_at": gorm.Expr("now()"), "published_at": nil, "not_before": nil,
			"correlation_id": correlationID, "traceparent": traceparent,
		})
		if err != nil {
			return err
		}
		if won {
			e.Status, e.CorrelationID, e.Traceparent = models.ExecQueued, correlationID, traceparent
			queued = true
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if queued {
		s.publish(ctx, out)
	}
	return out, nil
}

// publish is best effort: the sweeper republishes anything queued that was never published.
func (s *Service) publish(ctx context.Context, e *models.MigrationExecution) {
	if s.Publisher == nil {
		return
	}
	if err := s.Publisher.Publish(ctx, queue.Job{ExecutionID: e.ID, Traceparent: e.Traceparent, CorrelationID: e.CorrelationID}); err != nil {
		slog.WarnContext(ctx, "queue.publish_failed", obs.AttrExecutionID, e.ID, obs.AttrError, err)
		return
	}
	err := s.db.WithContext(ctx).Model(&models.MigrationExecution{}).Where("id = ?", e.ID).
		Updates(map[string]any{"published_at": gorm.Expr("now()")}).Error
	if err != nil {
		slog.WarnContext(ctx, "queue.mark_published_failed", obs.AttrExecutionID, e.ID, obs.AttrError, err)
	}
}

func (s *Service) PromoteStale(ctx context.Context, olderThan time.Duration) (int, error) {
	var ids []uint
	err := s.db.WithContext(ctx).Model(&models.MigrationExecution{}).
		Where("status = ? AND updated_at < now() - make_interval(secs => ?::float8)", models.ExecApproved, olderThan.Seconds()).
		Order("id").Limit(100).Pluck("id", &ids).Error
	if err != nil {
		return 0, err
	}
	n := 0
	for _, id := range ids {
		if e, err := s.Promote(ctx, id); err != nil {
			slog.WarnContext(ctx, "approval.promote_failed", obs.AttrExecutionID, id, obs.AttrError, err)
		} else if e.Status == models.ExecQueued {
			n++
		}
	}
	return n, nil
}

func (s *Service) Reject(ctx context.Context, userID, executionID uint, reason string) (*models.MigrationExecution, error) {
	u, err := s.user(ctx, userID)
	if err != nil {
		return nil, err
	}
	msg := "Ditolak oleh " + u.Username + "."
	if r := strings.TrimSpace(reason); r != "" {
		msg += " Alasan: " + r
	}
	return s.finishReview(ctx, userID, executionID, models.ExecRejected, "REJECTED", msg, obs.EventExecutionRejected, true)
}

func (s *Service) Cancel(ctx context.Context, userID, executionID uint) (*models.MigrationExecution, error) {
	return s.finishReview(ctx, userID, executionID, models.ExecCanceled, "CANCELED", "Ditarik oleh pengaju.", obs.EventExecutionCanceled, false)
}

func (s *Service) finishReview(ctx context.Context, userID, executionID uint, to models.ExecutionStatus, code, message, event string, asApprover bool) (*models.MigrationExecution, error) {
	var out *models.MigrationExecution
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		e, err := lockExecution(tx, executionID)
		if err != nil {
			return err
		}
		if asApprover {
			if _, _, err := s.requireApprover(ctx, tx, userID, e); err != nil {
				return err
			}
		} else {
			ok, err := s.authz.Can(ctx, userID, e.DatabaseID, authz.ViewMigrations)
			if err != nil {
				return err
			}
			if !ok || e.RequestedByID == nil || *e.RequestedByID != userID {
				return apperr.ErrForbidden
			}
		}
		if !open(e.Status) {
			return conflict("Eksekusi ini sudah tidak dalam tahap review.")
		}
		won, err := transition(tx, e.ID, e.Status, to, map[string]any{
			"error_code": code, "error_message": message, "finished_at": gorm.Expr("now()"),
		})
		if err != nil {
			return err
		}
		if !won {
			return conflict("Status eksekusi berubah. Muat ulang lalu coba lagi.")
		}
		e.Status = to
		out = e
		return nil
	})
	if err != nil {
		return nil, err
	}
	slog.InfoContext(ctx, event, obs.AttrActorID, userID, obs.AttrMigrationID, out.MigrationID, obs.AttrExecutionID, out.ID,
		obs.AttrJobID, out.ID, obs.AttrDatabaseID, out.DatabaseID, obs.AttrStatus, string(out.Status))
	return out, nil
}

func (s *Service) Withdraw(ctx context.Context, userID, executionID uint) (*models.MigrationExecution, error) {
	var out *models.MigrationExecution
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		e, err := lockExecution(tx, executionID)
		if err != nil {
			return err
		}
		ok, err := s.authz.Can(ctx, userID, e.DatabaseID, authz.ViewMigrations)
		if err != nil {
			return err
		}
		if !ok {
			return apperr.ErrForbidden
		}
		if !open(e.Status) {
			return conflict("Persetujuan tidak dapat dicabut setelah eksekusi masuk antrean.")
		}
		res := tx.Model(&models.MigrationApproval{}).
			Where("execution_id = ? AND user_id = ? AND revoked_at IS NULL", e.ID, userID).
			Update("revoked_at", gorm.Expr("now()"))
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return conflict("Anda belum menyetujui eksekusi ini.")
		}
		if e.Status == models.ExecApproved {
			won, err := transition(tx, e.ID, models.ExecApproved, models.ExecPendingReview, map[string]any{})
			if err != nil {
				return err
			}
			if won {
				e.Status = models.ExecPendingReview
			}
		}
		out = e
		return nil
	})
	if err != nil {
		return nil, err
	}
	slog.InfoContext(ctx, obs.EventApprovalWithdrawn, obs.AttrActorID, userID, obs.AttrMigrationID, out.MigrationID,
		obs.AttrExecutionID, out.ID, obs.AttrJobID, out.ID, obs.AttrDatabaseID, out.DatabaseID, obs.AttrStatus, string(out.Status))
	return out, nil
}

type PendingItem struct {
	ExecutionID  uint      `json:"executionId"`
	MigrationID  uint      `json:"migrationId"`
	DatabaseID   uint      `json:"databaseId"`
	DatabaseName string    `json:"databaseName"`
	Kind         string    `json:"kind"`
	RequestedBy  string    `json:"requestedBy"`
	CreatedAt    time.Time `json:"createdAt"`
	ApprovedByMe bool      `json:"approvedByMe"`
	Approved     int       `json:"approved"`
	Required     int       `json:"required"`
	Blocked      bool      `json:"blocked"`
}

func (s *Service) Pending(ctx context.Context, userID uint) ([]PendingItem, error) {
	items := []PendingItem{}
	err := s.db.WithContext(ctx).Raw(`
		SELECT e.id AS execution_id, e.migration_id, e.database_id, d.name AS database_name, e.kind,
			e.requested_by_username AS requested_by, e.created_at
		FROM migration_executions e
		JOIN database_connections d ON d.id = e.database_id
		WHERE e.status = ? AND EXISTS (
			SELECT 1 FROM groups g JOIN grants gr ON gr.group_id = g.id
			WHERE gr.database_id = e.database_id AND g.leader_id = ?)
		ORDER BY e.id`, models.ExecPendingReview, userID).Scan(&items).Error
	if err != nil {
		return nil, err
	}
	for i := range items {
		approvers, blocking, err := Approvers(ctx, s.db, items[i].DatabaseID)
		if err != nil {
			return nil, err
		}
		approved, err := liveApprovals(ctx, s.db, items[i].ExecutionID)
		if err != nil {
			return nil, err
		}
		items[i].Required = len(approvers)
		items[i].Blocked = len(blocking) > 0
		items[i].ApprovedByMe = approved[userID]
		for _, p := range approvers {
			if approved[p.UserID] {
				items[i].Approved++
			}
		}
	}
	return items, nil
}
