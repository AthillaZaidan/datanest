package migration

import (
	"time"

	"github.com/AthillaZaidan/datanest/backend/internal/models"
)

// View is the API shape of a migration.
type View struct {
	ID         uint `json:"id"`
	DatabaseID uint `json:"databaseId"`
	Version    *int `json:"version"`
	// SQL is SQLUp under its Milestone 1 name, kept for older clients.
	SQL                string                 `json:"sql"`
	SQLUp              string                 `json:"sqlUp"`
	SQLDown            string                 `json:"sqlDown"`
	Irreversible       bool                   `json:"irreversible"`
	IrreversibleReason string                 `json:"irreversibleReason,omitempty"`
	FrozenAt           *time.Time             `json:"frozenAt"`
	Status             models.MigrationStatus `json:"status"`
	ErrorMessage       string                 `json:"errorMessage,omitempty"`
	ErrorCode          string                 `json:"errorCode,omitempty"`
	BaseVersion        *int                   `json:"baseVersion"`
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
}

func toView(m *models.Migration, currentVersion int) View {
	base := 0
	if m.BaseVersion != nil {
		base = *m.BaseVersion
	}
	return View{
		ID:                 m.ID,
		DatabaseID:         m.DatabaseID,
		Version:            m.Version,
		SQL:                m.SQL,
		SQLUp:              m.SQL,
		SQLDown:            m.SQLDown,
		Irreversible:       m.Irreversible,
		IrreversibleReason: m.IrreversibleReason,
		FrozenAt:           m.FrozenAt,
		Status:             m.Status,
		ErrorMessage:       m.ErrorMessage,
		ErrorCode:          m.ErrorCode,
		BaseVersion:        m.BaseVersion,
		BaseOutdated:       m.Status == models.MigrationDraft && currentVersion > base,
		Revision:           m.Revision,
		CreatedBy:          userLabel(m.CreatedByID, m.CreatedByUsername),
		UpdatedBy:          userLabel(m.UpdatedByID, m.UpdatedByUsername),
		ExecutedBy:         userLabel(m.ExecutedByID, m.ExecutedByUsername),
		CreatedAt:          m.CreatedAt,
		UpdatedAt:          m.UpdatedAt,
		StartedAt:          m.StartedAt,
		FinishedAt:         m.FinishedAt,
	}
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
