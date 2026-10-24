package migration

import (
	"time"

	"github.com/AthillaZaidan/datanest/backend/internal/models"
)

// View is the API shape of a migration.
type View struct {
	ID           uint                   `json:"id"`
	DatabaseID   uint                   `json:"databaseId"`
	Version      *int                   `json:"version"`
	SQL          string                 `json:"sql"`
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
}

func toView(m *models.Migration, currentVersion int) View {
	base := 0
	if m.BaseVersion != nil {
		base = *m.BaseVersion
	}
	return View{
		ID:           m.ID,
		DatabaseID:   m.DatabaseID,
		Version:      m.Version,
		SQL:          m.SQL,
		Status:       m.Status,
		ErrorMessage: m.ErrorMessage,
		ErrorCode:    m.ErrorCode,
		BaseVersion:  m.BaseVersion,
		BaseOutdated: m.Status == models.MigrationDraft && currentVersion > base,
		Revision:     m.Revision,
		CreatedBy:    userLabel(m.CreatedByID, m.CreatedByUsername),
		UpdatedBy:    userLabel(m.UpdatedByID, m.UpdatedByUsername),
		ExecutedBy:   userLabel(m.ExecutedByID, m.ExecutedByUsername),
		CreatedAt:    m.CreatedAt,
		UpdatedAt:    m.UpdatedAt,
		StartedAt:    m.StartedAt,
		FinishedAt:   m.FinishedAt,
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
