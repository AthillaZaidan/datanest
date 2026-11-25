package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/AthillaZaidan/datanest/backend/internal/approval"
	"github.com/AthillaZaidan/datanest/backend/internal/authz"
	"github.com/AthillaZaidan/datanest/backend/internal/config"
	"github.com/AthillaZaidan/datanest/backend/internal/dbconn"
	"github.com/AthillaZaidan/datanest/backend/internal/migration"
	"github.com/AthillaZaidan/datanest/backend/internal/models"
	"github.com/AthillaZaidan/datanest/backend/internal/obs"
	"github.com/AthillaZaidan/datanest/backend/internal/queue"
	"github.com/AthillaZaidan/datanest/backend/internal/worker"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

// newWorker builds the worker with B's revalidation hook: the request is re-checked
// (version rules, frozen SQL) under the database lock before it runs.
func newWorker(db *gorm.DB, targets dbconn.TargetDBProvider, cfg *config.Config) *worker.Worker {
	host, _ := os.Hostname()
	w := worker.New(db, targets, worker.Config{
		WorkerID:         fmt.Sprintf("%s-%d", host, os.Getpid()),
		Concurrency:      cfg.Worker.Concurrency,
		MaxAttempts:      cfg.Worker.MaxAttempts,
		LeaseTTL:         cfg.Worker.LeaseTTL,
		HeartbeatEvery:   cfg.Worker.HeartbeatInterval,
		RepublishAfter:   cfg.Worker.RepublishAfter,
		LockTimeout:      cfg.Target.MigrationLockTimeout,
		StatementTimeout: cfg.Target.MigrationStatementTimeout,
		ExecTimeout:      cfg.Target.MigrationExecTimeout,
	})
	migrationService := migration.New(db, targets, authz.New(db), migration.Options{
		LockTimeout:      cfg.Target.MigrationLockTimeout,
		StatementTimeout: cfg.Target.MigrationStatementTimeout,
		ExecTimeout:      cfg.Target.MigrationExecTimeout,
	})
	w.Revalidate = migrationService.Revalidate
	approvals := approval.NewService(db, authz.New(db))
	w.PromoteApproved = func(ctx context.Context) (int, error) { return approvals.PromoteStale(ctx, 10*time.Second) }
	return w
}

func run() error {
	for _, f := range []string{".env", "../.env"} {
		_ = godotenv.Load(f)
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	shutdownObs, err := obs.Setup(cfg.LogLevel, cfg.TraceExporter, os.Stdout, os.Stderr)
	if err != nil {
		return err
	}
	defer shutdownObs(context.Background())
	if cfg.AMQPURL == "" {
		return errors.New("AMQP_URL is required to run the worker")
	}

	db, err := gorm.Open(postgres.Open(cfg.PlatformDSN), &gorm.Config{
		Logger: logger.New(log.New(os.Stderr, "", log.LstdFlags), logger.Config{
			SlowThreshold:             200 * time.Millisecond,
			LogLevel:                  logger.Warn,
			IgnoreRecordNotFoundError: true,
			ParameterizedQueries:      true,
		}),
	})
	if err != nil {
		return err
	}
	if err := models.Migrate(db); err != nil {
		return err
	}
	cipher, err := dbconn.NewCipher(cfg.CredentialsKey)
	if err != nil {
		return err
	}
	pools := dbconn.NewPools(db, cipher, cfg.Target.QueryTimeout)
	defer pools.Close()

	w := newWorker(db, pools, cfg)
	publisher := queue.NewPublisher(cfg.AMQPURL, queue.JobsQueue)
	defer publisher.Close()
	w.Publisher = publisher

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	slog.Info("worker.started", "slots", cfg.Worker.Concurrency, "queue", queue.JobsQueue)
	return w.Serve(ctx, cfg.AMQPURL, cfg.Worker.SweepInterval)
}
