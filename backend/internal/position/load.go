package position

import (
	"context"

	"gorm.io/gorm"

	"github.com/AthillaZaidan/datanest/backend/internal/models"
)

// Load reads the executions and migration versions of one database and derives its
// State. exclude (0 for none) leaves one execution out, so a worker can check its own
// execution against everything else.
func Load(ctx context.Context, db *gorm.DB, databaseID, exclude uint) (State, map[uint]int, error) {
	var rows []struct {
		ID                   uint
		MigrationID          uint
		Kind                 models.ExecutionKind
		Status               models.ExecutionStatus
		RevertsExecutionID   *uint
		ContinuesExecutionID *uint
	}
	err := db.WithContext(ctx).Model(&models.MigrationExecution{}).
		Select("id, migration_id, kind, status, reverts_execution_id, continues_execution_id").
		Where("database_id = ?", databaseID).Order("run_seq NULLS LAST, id").Scan(&rows).Error
	if err != nil {
		return State{}, nil, err
	}
	events := make([]Event, len(rows))
	for i, r := range rows {
		events[i] = Event{ID: r.ID, MigrationID: r.MigrationID, Kind: r.Kind, Status: r.Status,
			Reverts: r.RevertsExecutionID, Continues: r.ContinuesExecutionID}
	}

	var vs []struct {
		ID      uint
		Version int
	}
	err = db.WithContext(ctx).Model(&models.Migration{}).Select("id, version").
		Where("database_id = ? AND version IS NOT NULL", databaseID).Scan(&vs).Error
	if err != nil {
		return State{}, nil, err
	}
	versions := make(map[uint]int, len(vs))
	for _, v := range vs {
		versions[v.ID] = v.Version
	}
	return Derive(effective(events, versions, exclude)), versions, nil
}

func effective(events []Event, versions map[uint]int, exclude uint) []Event {
	lastRun := map[uint]uint{}
	for _, e := range events {
		if e.Kind == models.ExecKindRun {
			lastRun[e.MigrationID] = max(lastRun[e.MigrationID], e.ID)
		}
	}
	out := make([]Event, 0, len(events))
	for _, e := range events {
		_, versioned := versions[e.MigrationID]
		neverStarted := e.Kind == models.ExecKindRun && e.Status == models.ExecFailed &&
			(!versioned || e.ID < lastRun[e.MigrationID])
		if e.ID != exclude && !neverStarted {
			out = append(out, e)
		}
	}
	return out
}

// Current is the schema version of a database: the version of its top active
// migration, or 0 when nothing is active.
func Current(ctx context.Context, db *gorm.DB, databaseID uint) (int, error) {
	st, versions, err := Load(ctx, db, databaseID, 0)
	if err != nil {
		return 0, err
	}
	return ActiveVersion(st, versions), nil
}
