//go:build integration

package main

import (
	"context"
	"testing"
	"time"

	"github.com/AthillaZaidan/datanest/backend/internal/authz"
	"github.com/AthillaZaidan/datanest/backend/internal/config"
	"github.com/AthillaZaidan/datanest/backend/internal/migration"
	"github.com/AthillaZaidan/datanest/backend/internal/models"
	"github.com/AthillaZaidan/datanest/backend/internal/testutil"
	"github.com/AthillaZaidan/datanest/backend/internal/worker"
)

type allowAll struct{}

func (allowAll) Can(context.Context, uint, uint, authz.Action) (bool, error) { return true, nil }

// The worker built by main must re-check a request before running it: SQL that no
// longer matches the approved recipe fails instead of reaching the target.
func TestNewWorkerRevalidatesBeforeRunning(t *testing.T) {
	ctx := context.Background()
	platform := testutil.PlatformDB(t)
	target := testutil.TargetDB(t)
	provider := testutil.StaticProvider{DB: target}
	editor := testutil.CreateUser(t, platform, "editor")
	dbID := testutil.CreateDatabaseConnection(t, platform, "toko").ID

	svc := migration.New(platform, provider, allowAll{}, migration.Options{})
	m, err := svc.Create(ctx, editor.ID, dbID, migration.Recipe{SQLUp: "CREATE TABLE a (id int)", SQLDown: "DROP TABLE a"})
	if err != nil {
		t.Fatal(err)
	}
	e, err := svc.Submit(ctx, editor.ID, m.ID, models.ExecKindRun, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := platform.Model(e).Updates(map[string]any{"status": models.ExecQueued, "sql_executed": "CREATE TABLE b (id int)"}).Error; err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{}
	cfg.Worker.Concurrency, cfg.Worker.MaxAttempts = 1, 1
	cfg.Target.MigrationLockTimeout, cfg.Target.MigrationStatementTimeout, cfg.Target.MigrationExecTimeout = time.Second, 5*time.Second, 10*time.Second
	w := newWorker(platform, provider, cfg)
	claimed, result, err := w.Claim(ctx, e.ID)
	if err != nil || result != worker.ClaimOK {
		t.Fatalf("claim = %v, %v", result, err)
	}
	w.Run(ctx, claimed)

	var got models.MigrationExecution
	platform.First(&got, e.ID)
	if got.Status != models.ExecFailed || got.ErrorCode != "REVALIDATION_FAILED" {
		t.Fatalf("tampered run = %s %s %q", got.Status, got.ErrorCode, got.ErrorMessage)
	}
	var ran bool
	target.QueryRow(`SELECT to_regclass('public.b') IS NOT NULL`).Scan(&ran)
	if ran {
		t.Error("tampered SQL reached the target")
	}
}
