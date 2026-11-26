package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"

	"github.com/AthillaZaidan/datanest/backend/internal/admin"
	"github.com/AthillaZaidan/datanest/backend/internal/approval"
	"github.com/AthillaZaidan/datanest/backend/internal/auth"
	"github.com/AthillaZaidan/datanest/backend/internal/authz"
	"github.com/AthillaZaidan/datanest/backend/internal/config"
	"github.com/AthillaZaidan/datanest/backend/internal/dbconn"
	"github.com/AthillaZaidan/datanest/backend/internal/execution"
	"github.com/AthillaZaidan/datanest/backend/internal/explorer"
	"github.com/AthillaZaidan/datanest/backend/internal/httpapi"
	"github.com/AthillaZaidan/datanest/backend/internal/metrics"
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

func instanceID() string {
	host, _ := os.Hostname()
	return fmt.Sprintf("%s-%d", host, os.Getpid())
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
	if os.Getenv(gin.EnvGinMode) == "" {
		gin.SetMode(gin.ReleaseMode)
	}
	shutdownObs, err := obs.Setup(obs.Options{
		Level: cfg.LogLevel, TraceExporter: cfg.TraceExporter, OTLPEndpoint: cfg.OTLPEndpoint,
		Service: "datanest-server", Instance: instanceID(), LogOut: os.Stdout, TraceOut: os.Stderr,
	})
	if err != nil {
		return err
	}
	defer func() {
		flushCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = shutdownObs(flushCtx)
	}()

	db, err := dbconn.OpenPlatform(cfg.PlatformDSN, cfg.PlatformMaxOpenConns)
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
	platformSQL, err := db.DB()
	if err != nil {
		return err
	}
	metrics.Registry.MustRegister(
		metrics.NewExecutionCollector(db),
		metrics.NewPoolCollector(metrics.SinglePool("platform", "platform", platformSQL), pools.Stats),
	)
	az := authz.New(db)
	adminService := admin.New(db, sessions)
	databaseService := dbconn.New(db, credentialCipher, pools, az, cfg.Target.QueryTimeout)
	explorerService := explorer.New(db, pools, az, cfg.Target.QueryTimeout)
	migrationService := migration.New(db, pools, az, migration.Options{LockTimeout: cfg.Target.MigrationLockTimeout, StatementTimeout: cfg.Target.MigrationStatementTimeout, ExecTimeout: cfg.Target.MigrationExecTimeout})
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
	metrics.Start(ctx, cfg.MetricsAddr)

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
