package migration

import (
	"time"

	"github.com/AthillaZaidan/datanest/backend/internal/models"
	"github.com/AthillaZaidan/datanest/backend/internal/position"
)

type LastExecution struct {
	ID     uint                   `json:"id"`
	Kind   models.ExecutionKind   `json:"kind"`
	Status models.ExecutionStatus `json:"status"`
}

type PartialExecution struct {
	ExecutionID uint                 `json:"executionId"`
	Kind        models.ExecutionKind `json:"kind"`
}

// View is the API shape of a migration.
type View struct {
	ID         uint `json:"id"`
	DatabaseID uint `json:"databaseId"`
	Version    *int `json:"version"`
	// SQL is SQLUp under its Milestone 1 name, kept for older clients.
	SQL                string     `json:"sql"`
	SQLUp              string     `json:"sqlUp"`
	SQLDown            string     `json:"sqlDown"`
	Irreversible       bool       `json:"irreversible"`
	IrreversibleReason string     `json:"irreversibleReason,omitempty"`
	FrozenAt           *time.Time `json:"frozenAt"`
	// Active and Forwardable come from the derived position (package position), the same
	// source as the version rules, so the SPA never infers them from status or MAX(version).
	Active       bool                   `json:"active"`
	Forwardable  bool                   `json:"forwardable"`
	Status       models.MigrationStatus `json:"status"`
	ErrorMessage string                 `json:"errorMessage,omitempty"`
	ErrorCode    string                 `json:"errorCode,omitempty"`
	BaseVersion  *int                   `json:"baseVersion"`
	// BaseOutdated warns that the database moved on since this draft was written.
	BaseOutdated bool       `json:"baseOutdated"`
	Revision     int        `json:"revision"`
	CreatedBy    string     `json:"createdBy"`
	UpdatedBy    string     `json:"updatedBy,omitempty"`
	ExecutedBy   string     `json:"executedBy,omitempty"`
	CreatedAt    time.Time  `json:"createdAt"`
	UpdatedAt    time.Time  `json:"updatedAt"`
	StartedAt    *time.Time `json:"startedAt"`
	FinishedAt   *time.Time `json:"finishedAt"`
	// LastExecution is the migration's newest execution, whatever its status.
	LastExecution *LastExecution `json:"lastExecution"`
	// Partial is set when the database's unsettled partial execution belongs to this migration.
	Partial *PartialExecution `json:"partial"`
}

func toView(m *models.Migration, p snapshot) View {
	base := 0
	if m.BaseVersion != nil {
		base = *m.BaseVersion
	}
	v := View{
		ID:                 m.ID,
		DatabaseID:         m.DatabaseID,
		Version:            m.Version,
		SQL:                m.SQL,
		SQLUp:              m.SQL,
		SQLDown:            m.SQLDown,
		Irreversible:       m.Irreversible,
		IrreversibleReason: m.IrreversibleReason,
		FrozenAt:           m.FrozenAt,
		Active:             p.state.IsActive(m.ID),
		Forwardable:        p.state.Partial == nil && position.Forwardable(p.state, p.versions, m.ID),
		Status:             m.Status,
		ErrorMessage:       m.ErrorMessage,
		ErrorCode:          m.ErrorCode,
		BaseVersion:        m.BaseVersion,
		BaseOutdated:       m.Status == models.MigrationDraft && p.current > base,
		Revision:           m.Revision,
		CreatedBy:          userLabel(m.CreatedByID, m.CreatedByUsername),
		UpdatedBy:          userLabel(m.UpdatedByID, m.UpdatedByUsername),
		ExecutedBy:         userLabel(m.ExecutedByID, m.ExecutedByUsername),
		CreatedAt:          m.CreatedAt,
		UpdatedAt:          m.UpdatedAt,
		StartedAt:          m.StartedAt,
		FinishedAt:         m.FinishedAt,
	}
	if last, ok := p.last[m.ID]; ok {
		v.LastExecution = &last
	}
	if partial := p.state.Partial; partial != nil && partial.MigrationID == m.ID {
		v.Partial = &PartialExecution{ExecutionID: partial.ExecutionID, Kind: partial.Kind}
	}
	return v
}

// userLabel marks deleted users so they are not confused with a new user of the same name.
func userLabel(id *uint, username string) string {
	if username == "" {
		return ""
	}
	if id == nil {
		return username + " (deleted)"
	}
	return username
}
