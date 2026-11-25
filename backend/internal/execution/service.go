package execution

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/AthillaZaidan/datanest/backend/internal/apperr"
	"github.com/AthillaZaidan/datanest/backend/internal/approval"
	"github.com/AthillaZaidan/datanest/backend/internal/authz"
	"github.com/AthillaZaidan/datanest/backend/internal/models"
)

type Authorizer interface {
	Can(ctx context.Context, userID, databaseID uint, action authz.Action) (bool, error)
}

type Service struct {
	db    *gorm.DB
	store *Store
	authz Authorizer
}

func NewService(db *gorm.DB, az Authorizer) *Service {
	return &Service{db: db, store: NewStore(db), authz: az}
}

type ProgressView struct {
	CommittedBatches int     `json:"committedBatches"`
	CommittedRows    int64   `json:"committedRows"`
	LastCursor       *string `json:"lastCursor"`
	ScopeMax         *string `json:"scopeMax"`
	EstimatedRows    *int64  `json:"estimatedRows"`
}

type AttemptView struct {
	Number       int        `json:"number"`
	Status       string     `json:"status"`
	StartedAt    time.Time  `json:"startedAt"`
	FinishedAt   *time.Time `json:"finishedAt"`
	ErrorMessage string     `json:"errorMessage,omitempty"`
}

type ApproverView = approval.ApproverState

type View struct {
	ID            uint                 `json:"id"`
	MigrationID   uint                 `json:"migrationId"`
	DatabaseID    uint                 `json:"databaseId"`
	Kind          string               `json:"kind"`
	Status        string               `json:"status"`
	Mode          string               `json:"mode"`
	BatchSize     *int                 `json:"batchSize"`
	Progress      *ProgressView        `json:"progress"`
	Attempts      []AttemptView        `json:"attempts"`
	ErrorCode     string               `json:"errorCode,omitempty"`
	ErrorMessage  string               `json:"errorMessage,omitempty"`
	SQLExecuted   *string              `json:"sqlExecuted"`
	Approvers     []ApproverView       `json:"approvers"`
	BlockedGroups []approval.GroupRef  `json:"blockedGroups"`
	Permissions   approval.Permissions `json:"permissions"`
	RequestedBy   string               `json:"requestedBy"`
	CreatedAt     time.Time            `json:"createdAt"`
	StartedAt     *time.Time           `json:"startedAt"`
	FinishedAt    *time.Time           `json:"finishedAt"`
}

var errExecutionNotFound = apperr.ErrNotFound.WithMessage("Eksekusi tidak ditemukan.")

func (s *Service) Get(ctx context.Context, userID, id uint) (*View, error) {
	e, err := s.store.Get(ctx, id)
	if errors.Is(err, ErrNotFound) {
		return nil, errExecutionNotFound
	}
	if err != nil {
		return nil, err
	}
	ok, err := s.authz.Can(ctx, userID, e.DatabaseID, authz.ViewMigrations)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, apperr.ErrForbidden
	}
	attempts, err := s.store.Attempts(ctx, id)
	if err != nil {
		return nil, err
	}
	info, err := approval.Describe(ctx, s.db, e, userID)
	if err != nil {
		return nil, err
	}
	v := toView(e, attempts)
	v.Approvers, v.BlockedGroups, v.Permissions = info.Approvers, info.BlockedGroups, info.Permissions
	return v, nil
}

func toView(e *models.MigrationExecution, attempts []models.ExecutionAttempt) *View {
	v := &View{
		ID: e.ID, MigrationID: e.MigrationID, DatabaseID: e.DatabaseID,
		Kind: string(e.Kind), Status: string(e.Status), Mode: string(e.Mode), BatchSize: e.BatchSize,
		ErrorCode: e.ErrorCode, ErrorMessage: e.ErrorMessage,
		Approvers: []ApproverView{}, BlockedGroups: []approval.GroupRef{},
		RequestedBy: e.RequestedByUsername,
		CreatedAt:   e.CreatedAt, StartedAt: e.StartedAt, FinishedAt: e.FinishedAt,
		Attempts: make([]AttemptView, 0, len(attempts)),
	}
	if e.SQLExecuted != "" {
		v.SQLExecuted = &e.SQLExecuted
	}
	if e.Mode == models.ExecModeBatch {
		v.Progress = &ProgressView{
			CommittedBatches: e.CommittedBatches, CommittedRows: e.CommittedRows,
			LastCursor: e.LastCursor, ScopeMax: e.ScopeMax, EstimatedRows: e.EstimatedRows,
		}
	}
	for _, a := range attempts {
		v.Attempts = append(v.Attempts, AttemptView{
			Number: a.Number, Status: string(a.Status), StartedAt: a.StartedAt, FinishedAt: a.FinishedAt, ErrorMessage: a.ErrorMessage,
		})
	}
	return v
}
