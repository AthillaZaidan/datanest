package models

import "time"

type ExecutionStatus string

const (
	ExecPendingReview ExecutionStatus = "pending_review"
	ExecApproved      ExecutionStatus = "approved"
	ExecQueued        ExecutionStatus = "queued"
	ExecRunning       ExecutionStatus = "running"
	ExecSuccess       ExecutionStatus = "success"
	ExecFailed        ExecutionStatus = "failed"
	ExecPartial       ExecutionStatus = "partial"
	ExecUncertain     ExecutionStatus = "uncertain"
	ExecRejected      ExecutionStatus = "rejected"
	ExecCanceled      ExecutionStatus = "canceled"
)

type ExecutionKind string

const (
	ExecKindRun      ExecutionKind = "run"
	ExecKindForward  ExecutionKind = "forward"
	ExecKindRollback ExecutionKind = "rollback"
)

type ExecutionMode string

const (
	ExecModeNormal ExecutionMode = "normal"
	ExecModeBatch  ExecutionMode = "batch"
)

type MigrationExecution struct {
	ID          uint               `gorm:"primaryKey"`
	MigrationID uint               `gorm:"not null;index"`
	Migration   Migration          `gorm:"foreignKey:MigrationID;constraint:OnDelete:CASCADE"`
	DatabaseID  uint               `gorm:"not null;index;uniqueIndex:idx_executions_run_seq,priority:1"`
	Database    DatabaseConnection `gorm:"foreignKey:DatabaseID;constraint:OnDelete:CASCADE"`
	RunSeq      *int64             `gorm:"uniqueIndex:idx_executions_run_seq,priority:2"`

	Kind   ExecutionKind   `gorm:"size:16;not null;check:chk_executions_kind,kind IN ('run', 'forward', 'rollback')"`
	Status ExecutionStatus `gorm:"size:16;not null;index;check:chk_executions_status,status IN ('pending_review', 'approved', 'queued', 'running', 'success', 'failed', 'partial', 'uncertain', 'rejected', 'canceled')"`
	Mode   ExecutionMode   `gorm:"size:8;not null;default:normal;check:chk_executions_mode,mode IN ('normal', 'batch')"`

	SQLExecuted string `gorm:"column:sql_executed;type:text;not null"`

	BatchSize   *int
	BatchTable  string `gorm:"not null;default:''"`
	BatchColumn string `gorm:"not null;default:''"`

	RevertsExecutionID   *uint `gorm:"index"`
	ContinuesExecutionID *uint `gorm:"index"`

	RequestedByID       *uint
	RequestedBy         *User  `gorm:"foreignKey:RequestedByID;constraint:OnDelete:SET NULL"`
	RequestedByUsername string `gorm:"size:64;not null;default:''"`

	CorrelationID string `gorm:"not null;default:''"`
	Traceparent   string `gorm:"not null;default:''"`

	WorkerID    string     `gorm:"size:160;not null;default:''"`
	Attempts    int        `gorm:"not null;default:0"`
	NotBefore   *time.Time `gorm:"index"`
	PublishedAt *time.Time

	ErrorMessage string `gorm:"type:text;not null;default:''"`
	ErrorCode    string `gorm:"size:32;not null;default:''"`

	CommittedBatches int   `gorm:"not null;default:0"`
	CommittedRows    int64 `gorm:"not null;default:0"`
	LastCursor       *string
	ScopeMax         *string
	EstimatedRows    *int64

	CreatedAt  time.Time
	UpdatedAt  time.Time
	QueuedAt   *time.Time
	StartedAt  *time.Time
	FinishedAt *time.Time
}

type AttemptStatus string

const (
	AttemptRunning   AttemptStatus = "running"
	AttemptSuccess   AttemptStatus = "success"
	AttemptFailed    AttemptStatus = "failed"
	AttemptUncertain AttemptStatus = "uncertain"
)

type ExecutionAttempt struct {
	ID          uint               `gorm:"primaryKey"`
	ExecutionID uint               `gorm:"not null;uniqueIndex:idx_attempts_execution_number,priority:1"`
	Execution   MigrationExecution `gorm:"foreignKey:ExecutionID;constraint:OnDelete:CASCADE"`
	Number      int                `gorm:"not null;uniqueIndex:idx_attempts_execution_number,priority:2"`

	Status       AttemptStatus `gorm:"size:16;not null;check:chk_attempts_status,status IN ('running', 'success', 'failed', 'uncertain')"`
	WorkerID     string        `gorm:"size:160;not null;default:''"`
	SQLExecuted  string        `gorm:"column:sql_executed;type:text;not null;default:''"`
	ErrorMessage string        `gorm:"type:text;not null;default:''"`
	ErrorCode    string        `gorm:"size:32;not null;default:''"`

	StartedAt  time.Time `gorm:"not null"`
	FinishedAt *time.Time
}
