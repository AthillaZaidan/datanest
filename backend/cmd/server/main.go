package main

import (
	"context"
	"errors"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/AthillaZaidan/datanest/backend/internal/admin"
	"github.com/AthillaZaidan/datanest/backend/internal/approval"
	"github.com/AthillaZaidan/datanest/backend/internal/auth"
	"github.com/AthillaZaidan/datanest/backend/internal/authz"
	"github.com/AthillaZaidan/datanest/backend/internal/config"
	"github.com/AthillaZaidan/datanest/backend/internal/dbconn"
	"github.com/AthillaZaidan/datanest/backend/internal/execution"
	"github.com/AthillaZaidan/datanest/backend/internal/explorer"
	"github.com/AthillaZaidan/datanest/backend/internal/httpapi"
	"github.com/AthillaZaidan/datanest/backend/internal/migration"
	"github.com/AthillaZaidan/datanest/backend/internal/models"
	"github.com/AthillaZaidan/datanest/backend/internal/obs"
	"github.com/AthillaZaidan/datanest/backend/internal/queue"
	"github.com/AthillaZaidan/datanest/backend/internal/session"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	// .env is optional; real environment variables take precedence. Load each file on its
	// own: godotenv stops at the first missing file.
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

	db, err := gorm.Open(postgres.Open(cfg.PlatformDSN), &gorm.Config{
		Logger: logger.New(log.New(os.Stderr, "", log.LstdFlags), logger.Config{
			SlowThreshold:             200 * time.Millisecond,
			LogLevel:                  logger.Warn,
			IgnoreRecordNotFoundError: true,
			ParameterizedQueries:      true, // never log query values (password hashes, etc.)
		}),
	})
	if err != nil {
		return err
	}
	if err := models.Migrate(db); err != nil {
		return err
	}
	if err := auth.SeedAdmin(db, cfg.AdminUsername, cfg.AdminPassword); err != nil {
		return err
	}
	credentialCipher, err := dbconn.NewCipher(cfg.CredentialsKey)
	if err != nil {
		return err
	}
	sessions := session.New(db, credentialCipher, session.Config(cfg.Session), nil)
	defer sessions.Close()
	authService := &auth.Service{DB: db, Sessions: sessions, CookieSecure: cfg.CookieSecure}

	pools := dbconn.NewPools(db, credentialCipher, cfg.Target.QueryTimeout)
	defer pools.Close()
	az := authz.New(db)
	adminService := admin.New(db, sessions)
	databaseService := dbconn.New(db, credentialCipher, pools, az, cfg.Target.QueryTimeout)
	explorerService := explorer.New(db, pools, az, cfg.Target.QueryTimeout)
	migrationService := migration.New(db, pools, az, migration.Options{LockTimeout: cfg.Target.MigrationLockTimeout, StatementTimeout: cfg.Target.MigrationStatementTimeout, ExecTimeout: cfg.Target.MigrationExecTimeout})
	// An unreachable target leaves its migration running for the next start; it must not keep the platform down.
	if err := migrationService.Reconcile(context.Background()); err != nil {
		slog.Warn("reconcile.failed", obs.AttrError, err)
	}
	approvals := approval.NewService(db, az)
	if cfg.AMQPURL != "" {
		publisher := queue.NewPublisher(cfg.AMQPURL, queue.JobsQueue)
		defer publisher.Close()
		approvals.Publisher = publisher
	}
	router := httpapi.NewRouter(httpapi.Deps{
		Admin: adminService, Databases: databaseService, Explorer: explorerService, Migrations: migrationService,
		Executions: execution.NewService(db, az), Approvals: approvals,
		Security: auth.ValidateStateRequest(cfg.AllowedOrigin), RequireAuth: authService.RequireAuth,
		RequireAdmin: authService.RequireAdmin, Login: authService.Login, Logout: authService.Logout, Me: authService.Me,
	})
	httpapi.ServeSPA(router, cfg.FrontendDir)
	srv := &http.Server{Addr: ":" + cfg.AppPort, Handler: router, ReadHeaderTimeout: 10 * time.Second}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errc := make(chan error, 1)
	go func() {
		slog.Info("server.listening", "addr", srv.Addr)
		errc <- srv.ListenAndServe()
	}()

	select {
	case err := <-errc:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}
