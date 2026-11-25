// Package obs holds the names, masking and helpers shared by structured logs and traces.
package obs

const (
	EventHTTPRequest        = "http.request"
	EventAuthLogin          = "auth.login"
	EventAuthzDenied        = "authz.denied"
	EventExplorerQuery      = "explorer.query"
	EventExecutionRequested = "execution.requested"
	EventExecutionApproved  = "execution.approved"
	EventExecutionRejected  = "execution.rejected"
	EventExecutionCanceled  = "execution.canceled"
	EventApprovalWithdrawn  = "execution.approval_withdrawn"
	EventQueuePublished     = "queue.published"
	EventLockAcquired       = "lock.acquired"
	EventLockLost           = "lock.lost"
	EventSQLStart           = "sql.start"
	EventSQLResult          = "sql.result"
	EventSQLCommit          = "sql.commit"
	EventSQLRollback        = "sql.rollback"
	EventRecoveryCheck      = "recovery.check"
	EventGrantChanged       = "grant.changed"
	EventLeaderChanged      = "leader.changed"
)

const (
	AttrEvent         = "event"
	AttrActorID       = "actor_id"
	AttrActor         = "actor"
	AttrDatabaseID    = "database_id"
	AttrStatus        = "status"
	AttrCorrelationID = "correlation_id"
	AttrTraceID       = "trace_id"
	AttrSpanID        = "span_id"

	AttrQuery      = "query"
	AttrParams     = "params"
	AttrDurationMS = "duration_ms"
	AttrRows       = "rows"
	AttrError      = "error"
	AttrSQLState   = "sqlstate"

	AttrMigrationID       = "migration_id"
	AttrMigrationVersion  = "migration_version"
	AttrMigrationRevision = "migration_revision"
	AttrExecutionID       = "execution_id"
	AttrJobID             = "job_id"
	AttrAttempt           = "attempt"
	AttrBatchNo           = "batch_no"

	AttrMethod  = "method"
	AttrPath    = "path"
	AttrGroupID = "group_id"
	AttrLeader  = "leader_id"
	AttrAction  = "action"
)

func MigrationVersionAttrs(version *int, revision int) []any {
	attrs := []any{AttrMigrationRevision, revision}
	if version != nil {
		attrs = append(attrs, AttrMigrationVersion, *version)
	}
	return attrs
}

const (
	SpanAPIApprove    = "api.approve"
	SpanQueuePublish  = "queue.publish"
	SpanWorkerProcess = "worker.process"
	SpanLockAcquire   = "lock.acquire"
	SpanExecute       = "execute"
	SpanBatch         = "batch"
	SpanSQLQuery      = "sql.query"
)
