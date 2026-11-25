package migration

import (
	"context"
	"time"

	"github.com/AthillaZaidan/datanest/backend/internal/authz"
	"github.com/AthillaZaidan/datanest/backend/internal/models"
)

// HistoryItem is one execution in the migration history of a database.
type HistoryItem struct {
	ID                 uint                   `json:"id"`
	MigrationID        uint                   `json:"migrationId"`
	Version            *int                   `json:"version"`
	Kind               models.ExecutionKind   `json:"kind"`
	Status             models.ExecutionStatus `json:"status"`
	SQLExecuted        string                 `json:"sqlExecuted"`
	RevertsExecutionID *uint                  `json:"revertsExecutionId"`
	RequestedBy        string                 `json:"requestedBy"`
	ErrorCode          string                 `json:"errorCode,omitempty"`
	ErrorMessage       string                 `json:"errorMessage,omitempty"`
	StartedAt          *time.Time             `json:"startedAt"`
	FinishedAt         *time.Time             `json:"finishedAt"`
}

// historyStatuses are the statuses shown in the history: execution has started (design D3).
// Requests still waiting for review or the queue are shown on the review screens.
var historyStatuses = []models.ExecutionStatus{models.ExecRunning, models.ExecSuccess, models.ExecFailed,
	models.ExecPartial, models.ExecUncertain}

// History lists a database's executions oldest first. With hideRollback, each successful
// rollback is hidden together with the execution it reverted, matched by execution ID.
// Failed attempts always stay visible, and nothing is deleted.
func (s *Service) History(ctx context.Context, userID, databaseID uint, hideRollback bool) ([]HistoryItem, error) {
	if err := s.requireDatabase(ctx, databaseID); err != nil {
		return nil, err
	}
	if err := s.authorize(ctx, userID, databaseID, authz.ViewMigrations); err != nil {
		return nil, err
	}
	var rows []struct {
		models.MigrationExecution
		Version *int
	}
	err := s.platform.WithContext(ctx).Table("migration_executions e").
		Select("e.*, m.version").Joins("JOIN migrations m ON m.id = e.migration_id").
		Where("e.database_id = ? AND e.status IN ?", databaseID, historyStatuses).
		Order("e.id").Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	items := make([]HistoryItem, len(rows))
	for i, r := range rows {
		e := r.MigrationExecution
		items[i] = HistoryItem{
			ID: e.ID, MigrationID: e.MigrationID, Version: r.Version, Kind: e.Kind, Status: e.Status,
			SQLExecuted: e.SQLExecuted, RevertsExecutionID: e.RevertsExecutionID,
			RequestedBy: userLabel(e.RequestedByID, e.RequestedByUsername),
			ErrorCode:   e.ErrorCode, ErrorMessage: e.ErrorMessage, StartedAt: e.StartedAt, FinishedAt: e.FinishedAt,
		}
	}
	if hideRollback {
		items = hideRolledBack(items)
	}
	return items, nil
}

// hideRolledBack drops each successful rollback and the execution it reverted.
func hideRolledBack(items []HistoryItem) []HistoryItem {
	hidden := map[uint]bool{}
	for _, it := range items {
		if it.Kind == models.ExecKindRollback && it.Status == models.ExecSuccess && it.RevertsExecutionID != nil {
			hidden[it.ID] = true
			hidden[*it.RevertsExecutionID] = true
		}
	}
	out := make([]HistoryItem, 0, len(items))
	for _, it := range items {
		if !hidden[it.ID] {
			out = append(out, it)
		}
	}
	return out
}
