package worker

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"gorm.io/gorm"

	"github.com/AthillaZaidan/datanest/backend/internal/dbconn"
	"github.com/AthillaZaidan/datanest/backend/internal/dblock"
	"github.com/AthillaZaidan/datanest/backend/internal/execution"
	"github.com/AthillaZaidan/datanest/backend/internal/models"
	"github.com/AthillaZaidan/datanest/backend/internal/queue"
)

var (
	ErrCrash                 = errors.New("worker: simulated crash")
	ErrInjectedTransient     = errors.New("worker: injected transient failure")
	ErrInjectedCommitUnknown = errors.New("worker: injected unknown commit outcome")

	errLockLost = errors.New("worker: the lock was lost")
)

type Hooks struct {
	BeforeCommit    func(executionID uint, batchNo int) error
	AfterCommit     func(executionID uint, batchNo int) error
	BeforeResult    func(executionID uint) error
	BeforeHeartbeat func(executionID uint) error
}

type Publisher interface {
	Publish(ctx context.Context, job queue.Job) error
}

type Config struct {
	WorkerID         string
	Queue            string
	Concurrency      int
	LeaseTTL         time.Duration
	HeartbeatEvery   time.Duration
	MaxAttempts      int
	RetryDelays      []time.Duration
	LockBusyDelay    time.Duration
	UncertainRetry   time.Duration
	RepublishAfter   time.Duration
	LockTimeout      time.Duration
	StatementTimeout time.Duration
	ExecTimeout      time.Duration
}

func (c Config) withDefaults() Config {
	if c.WorkerID == "" {
		c.WorkerID = "worker"
	}
	if c.Queue == "" {
		c.Queue = queue.JobsQueue
	}
	if c.Concurrency < 1 {
		c.Concurrency = 4
	}
	if c.LeaseTTL <= 0 {
		c.LeaseTTL = 30 * time.Second
	}
	if c.HeartbeatEvery <= 0 {
		c.HeartbeatEvery = 10 * time.Second
	}
	if c.MaxAttempts < 1 {
		c.MaxAttempts = 3
	}
	if len(c.RetryDelays) == 0 {
		c.RetryDelays = []time.Duration{5 * time.Second, 15 * time.Second}
	}
	if c.LockBusyDelay <= 0 {
		c.LockBusyDelay = 5 * time.Second
	}
	if c.UncertainRetry <= 0 {
		c.UncertainRetry = 30 * time.Second
	}
	if c.RepublishAfter <= 0 {
		c.RepublishAfter = 30 * time.Second
	}
	if c.ExecTimeout <= 0 {
		c.ExecTimeout = 2 * time.Minute
	}
	return c
}

type Worker struct {
	Hooks           Hooks
	Revalidate      func(ctx context.Context, e *models.MigrationExecution) error
	Publisher       Publisher
	PromoteApproved func(ctx context.Context) (int, error)

	cfg     Config
	store   *execution.Store
	locks   *dblock.Store
	targets dbconn.TargetDBProvider
	seq     atomic.Uint64
	slots   chan struct{}
	running sync.WaitGroup
}

func New(db *gorm.DB, targets dbconn.TargetDBProvider, cfg Config) *Worker {
	cfg = cfg.withDefaults()
	return &Worker{
		cfg:     cfg,
		store:   execution.NewStore(db),
		locks:   dblock.New(db),
		targets: targets,
		slots:   make(chan struct{}, cfg.Concurrency),
	}
}

func (w *Worker) Store() *execution.Store { return w.store }

func (w *Worker) newOwner() string {
	return fmt.Sprintf("%s/%d", w.cfg.WorkerID, w.seq.Add(1))
}

func (w *Worker) retryDelay(attempt int) time.Duration {
	i := attempt - 1
	if i < 0 {
		i = 0
	}
	if i >= len(w.cfg.RetryDelays) {
		i = len(w.cfg.RetryDelays) - 1
	}
	return w.cfg.RetryDelays[i]
}

func (w *Worker) Wait() { w.running.Wait() }
