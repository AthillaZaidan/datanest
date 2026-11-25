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
		ID                 uint
		MigrationID        uint
		Kind               models.ExecutionKind
		Status             models.ExecutionStatus
		RevertsExecutionID *uint
	}
	err := db.WithContext(ctx).Model(&models.MigrationExecution{}).
		Select("id, migration_id, kind, status, reverts_execution_id").
		Where("database_id = ? AND id <> ?", databaseID, exclude).Order("run_seq NULLS LAST, id").Scan(&rows).Error
	if err != nil {
		return State{}, nil, err
	}
	events := make([]Event, len(rows))
	for i, r := range rows {
		events[i] = Event{ID: r.ID, MigrationID: r.MigrationID, Kind: r.Kind, Status: r.Status, Reverts: r.RevertsExecutionID}
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
	return Derive(events), versions, nil
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
