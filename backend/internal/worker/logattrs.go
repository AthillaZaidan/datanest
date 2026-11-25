package worker

import "github.com/AthillaZaidan/datanest/backend/internal/models"

// jobAttrs are the attributes every worker log line carries for an execution: the job
// (execution) and migration IDs, the database, and the person who asked for it.
func jobAttrs(e *models.MigrationExecution) []any {
	attrs := []any{"execution_id", e.ID, "job_id", e.ID, "migration_id", e.MigrationID, "database_id", e.DatabaseID}
	if e.RequestedByID != nil {
		attrs = append(attrs, "actor_id", *e.RequestedByID)
	}
	if e.RequestedByUsername != "" {
		attrs = append(attrs, "actor", e.RequestedByUsername)
	}
	return attrs
}

func logAttrs(e *models.MigrationExecution, extra ...any) []any {
	return append(jobAttrs(e), extra...)
}
