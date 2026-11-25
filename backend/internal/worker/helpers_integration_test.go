//go:build integration

package worker

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/AthillaZaidan/datanest/backend/internal/dbconn"
	"github.com/AthillaZaidan/datanest/backend/internal/migration"
	"github.com/AthillaZaidan/datanest/backend/internal/models"
	"github.com/AthillaZaidan/datanest/backend/internal/queue"
	"github.com/AthillaZaidan/datanest/backend/internal/testutil"
)

const incrementQty = `UPDATE items SET qty = qty + 1`
const incrementQtyInRange = `UPDATE items SET qty = qty + 1 WHERE id > $1 AND id <= $2`

type env struct {
	t        *testing.T
	platform *gorm.DB
	target   *sql.DB
	conn     models.DatabaseConnection
	provider *switchProvider
	w        *Worker
	pub      *fakePublisher
}

type switchProvider struct {
	inner dbconn.TargetDBProvider
	down  atomic.Bool
}

func (p *switchProvider) Get(ctx context.Context, id uint) (*sql.DB, error) {
	if p.down.Load() {
		return nil, errors.New("connection refused")
	}
	return p.inner.Get(ctx, id)
}

func (p *switchProvider) Invalidate(uint) {}

type fakePublisher struct {
	mu   sync.Mutex
	jobs []queue.Job
	fail bool
}

func (f *fakePublisher) Publish(_ context.Context, job queue.Job) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fail {
		return errors.New("broker down")
	}
	f.jobs = append(f.jobs, job)
	return nil
}

func (f *fakePublisher) ids() []uint {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]uint, len(f.jobs))
	for i, j := range f.jobs {
		out[i] = j.ExecutionID
	}
	return out
}

func testConfig() Config {
	return Config{
		WorkerID:       "test",
		Concurrency:    4,
		LeaseTTL:       30 * time.Second,
		HeartbeatEvery: time.Hour,
		MaxAttempts:    3,
		RetryDelays:    []time.Duration{time.Millisecond},
		LockBusyDelay:  time.Minute,
		UncertainRetry: time.Minute,
		RepublishAfter: time.Minute,
		ExecTimeout:    20 * time.Second,
	}
}

func setupWith(t *testing.T, cfg Config) *env {
	t.Helper()
	platform := testutil.PlatformDB(t)
	target := testutil.TargetDB(t)
	if _, err := target.Exec(`CREATE TABLE items (id int PRIMARY KEY, qty int NOT NULL DEFAULT 0, tag text);
		INSERT INTO items (id) SELECT g FROM generate_series(1, 95) g`); err != nil {
		t.Fatal(err)
	}
	tx, err := target.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if err := migration.EnsureExecutionLedger(context.Background(), tx); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	provider := &switchProvider{inner: testutil.StaticProvider{DB: target}}
	w := New(platform, provider, cfg)
	pub := &fakePublisher{}
	w.Publisher = pub
	return &env{t: t, platform: platform, target: target, conn: testutil.CreateDatabaseConnection(t, platform, "toko"), provider: provider, w: w, pub: pub}
}

func setup(t *testing.T) *env { return setupWith(t, testConfig()) }

func (e *env) newExec(x models.MigrationExecution) models.MigrationExecution {
	e.t.Helper()
	if x.DatabaseID == 0 {
		x.DatabaseID = e.conn.ID
	}
	if x.SQLExecuted == "" {
		x.SQLExecuted = incrementQty
	}
	return testutil.CreateExecution(e.t, e.platform, x)
}

func (e *env) newBatchExec(sqlText string, size int, x models.MigrationExecution) models.MigrationExecution {
	e.t.Helper()
	x.Mode = models.ExecModeBatch
	x.BatchTable = "items"
	if x.BatchColumn == "" {
		x.BatchColumn = "id"
	}
	x.BatchSize = &size
	x.SQLExecuted = sqlText
	return e.newExec(x)
}

func (e *env) process(id uint) (ClaimResult, error) {
	e.t.Helper()
	return processWith(e.t, e.w, id)
}

func processWith(t *testing.T, w *Worker, id uint) (ClaimResult, error) {
	t.Helper()
	claimed, result, err := w.Claim(context.Background(), id)
	if err != nil || result != ClaimOK {
		return result, err
	}
	return result, w.Run(context.Background(), claimed)
}

func (e *env) mustProcess(id uint) {
	e.t.Helper()
	result, err := e.process(id)
	if err != nil || result != ClaimOK {
		e.t.Fatalf("process %d = %v, %v", id, result, err)
	}
}

func (e *env) get(id uint) models.MigrationExecution {
	e.t.Helper()
	var x models.MigrationExecution
	if err := e.platform.First(&x, id).Error; err != nil {
		e.t.Fatal(err)
	}
	return x
}

func (e *env) attempts(id uint) []models.ExecutionAttempt {
	e.t.Helper()
	list, err := e.w.store.Attempts(context.Background(), id)
	if err != nil {
		e.t.Fatal(err)
	}
	return list
}

func (e *env) ledger(id uint) []migration.LedgerEntry {
	e.t.Helper()
	entries, err := migration.LookupExecution(context.Background(), e.target, id)
	if err != nil {
		e.t.Fatal(err)
	}
	return entries
}

func (e *env) qty(id int) int {
	e.t.Helper()
	var n int
	if err := e.target.QueryRow(`SELECT qty FROM items WHERE id = $1`, id).Scan(&n); err != nil {
		e.t.Fatal(err)
	}
	return n
}

func (e *env) sumQty() int {
	e.t.Helper()
	var n int
	if err := e.target.QueryRow(`SELECT COALESCE(sum(qty), 0) FROM items`).Scan(&n); err != nil {
		e.t.Fatal(err)
	}
	return n
}

func (e *env) lockHeld(databaseID uint) bool {
	e.t.Helper()
	var n int64
	e.platform.Raw(`SELECT count(*) FROM database_locks WHERE database_id = ? AND lease_until > now()`, databaseID).Scan(&n)
	return n > 0
}

func (e *env) expireLock(databaseID uint) {
	e.t.Helper()
	if err := e.platform.Exec(`UPDATE database_locks SET lease_until = now() - interval '1 second' WHERE database_id = ?`, databaseID).Error; err != nil {
		e.t.Fatal(err)
	}
}

func (e *env) makeDue(id uint) {
	e.t.Helper()
	if err := e.platform.Exec(`UPDATE migration_executions SET not_before = now() - interval '1 second' WHERE id = ?`, id).Error; err != nil {
		e.t.Fatal(err)
	}
}

func (e *env) expectStatus(id uint, want models.ExecutionStatus) models.MigrationExecution {
	e.t.Helper()
	x := e.get(id)
	if x.Status != want {
		e.t.Fatalf("execution %d status = %s (%s %s), want %s", id, x.Status, x.ErrorCode, x.ErrorMessage, want)
	}
	return x
}

func (e *env) expectHeld(id uint, blockers ...uint) {
	e.t.Helper()
	result, err := e.process(id)
	if err != nil || result != ClaimHeld {
		e.t.Fatalf("process %d = %v, %v; want held", id, result, err)
	}
	x := e.expectStatus(id, models.ExecQueued)
	if x.ErrorCode != "HELD" || x.Attempts != 0 || x.NotBefore == nil || x.PublishedAt != nil {
		e.t.Errorf("held row = %+v", x)
	}
	for _, b := range blockers {
		if !strings.Contains(x.ErrorMessage, fmt.Sprintf("E%d ", b)) {
			e.t.Errorf("reason %q does not name E%d", x.ErrorMessage, b)
		}
	}
	if len(e.ledger(id)) != 0 || len(e.attempts(id)) != 0 || e.lockHeld(x.DatabaseID) {
		e.t.Error("a held execution left a ledger row, an attempt or the lock behind")
	}
}

func (e *env) secondDatabase() models.DatabaseConnection {
	e.t.Helper()
	return testutil.CreateDatabaseConnection(e.t, e.platform, "kampus")
}

func spans(entries []migration.LedgerEntry) [][2]string {
	var out [][2]string
	for _, entry := range entries {
		if entry.RangeFrom != nil && entry.RangeTo != nil {
			out = append(out, [2]string{*entry.RangeFrom, *entry.RangeTo})
		}
	}
	return out
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

type gate struct {
	arrived chan uint
	release chan struct{}
}

func newGate() *gate { return &gate{arrived: make(chan uint, 16), release: make(chan struct{})} }

func (g *gate) beforeCommit(id uint, _ int) error {
	g.arrived <- id
	<-g.release
	return nil
}
