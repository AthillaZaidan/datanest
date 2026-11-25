//go:build integration

package migration_test

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/AthillaZaidan/datanest/backend/internal/apperr"
	"github.com/AthillaZaidan/datanest/backend/internal/authz"
	"github.com/AthillaZaidan/datanest/backend/internal/migration"
	"github.com/AthillaZaidan/datanest/backend/internal/models"
	"github.com/AthillaZaidan/datanest/backend/internal/position"
	"github.com/AthillaZaidan/datanest/backend/internal/testutil"
	"github.com/AthillaZaidan/datanest/backend/internal/worker"
)

type allow map[uint]authz.Level

func (a allow) Can(_ context.Context, userID, _ uint, act authz.Action) (bool, error) {
	need, ok := authz.RequiredLevel(act)
	return ok && a[userID] >= need, nil
}

type flow struct {
	t        *testing.T
	ctx      context.Context
	platform *gorm.DB
	target   *sql.DB
	svc      *migration.Service
	w        *worker.Worker
	dbID     uint
	editor   models.User
	viewer   models.User
}

func setupFlow(t *testing.T) *flow {
	t.Helper()
	platform := testutil.PlatformDB(t)
	target := testutil.TargetDB(t)
	editor := testutil.CreateUser(t, platform, "editor")
	viewer := testutil.CreateUser(t, platform, "viewer")
	provider := testutil.StaticProvider{DB: target}
	svc := migration.New(platform, provider, allow{editor.ID: authz.Edit, viewer.ID: authz.View},
		migration.Options{LockTimeout: time.Second, StatementTimeout: 5 * time.Second, ExecTimeout: 10 * time.Second})
	w := worker.New(platform, provider, worker.Config{WorkerID: "test", Concurrency: 1, LeaseTTL: 30 * time.Second,
		HeartbeatEvery: time.Hour, MaxAttempts: 1, RetryDelays: []time.Duration{time.Millisecond},
		LockBusyDelay: time.Minute, UncertainRetry: time.Minute, RepublishAfter: time.Minute, ExecTimeout: 10 * time.Second})
	w.Revalidate = svc.Revalidate
	return &flow{t: t, ctx: context.Background(), platform: platform, target: target, svc: svc, w: w,
		dbID: testutil.CreateDatabaseConnection(t, platform, "toko").ID, editor: editor, viewer: viewer}
}

func (f *flow) recipe(up, down string) uint {
	f.t.Helper()
	v, err := f.svc.Create(f.ctx, f.editor.ID, f.dbID, migration.Recipe{SQLUp: up, SQLDown: down})
	if err != nil {
		f.t.Fatal(err)
	}
	return v.ID
}

// do submits, approves (standing in for the approval flow) and lets the worker run it.
func (f *flow) do(migrationID uint, kind models.ExecutionKind) models.MigrationExecution {
	f.t.Helper()
	e, err := f.svc.Submit(f.ctx, f.editor.ID, migrationID, kind)
	if err != nil {
		f.t.Fatalf("submit %s: %v", kind, err)
	}
	return f.run(*e)
}

func (f *flow) run(e models.MigrationExecution) models.MigrationExecution {
	f.t.Helper()
	if err := f.platform.Model(&e).Update("status", models.ExecQueued).Error; err != nil {
		f.t.Fatal(err)
	}
	claimed, result, err := f.w.Claim(f.ctx, e.ID)
	if err != nil || result != worker.ClaimOK {
		f.t.Fatalf("claim = %v, %v", result, err)
	}
	if err := f.w.Run(f.ctx, claimed); err != nil {
		f.t.Fatalf("run: %v", err)
	}
	var out models.MigrationExecution
	f.platform.First(&out, e.ID)
	return out
}

func (f *flow) setStatus(e models.MigrationExecution, status models.ExecutionStatus) {
	f.t.Helper()
	if err := f.platform.Model(&e).Update("status", status).Error; err != nil {
		f.t.Fatal(err)
	}
}

func (f *flow) current() int {
	f.t.Helper()
	v, err := position.Current(f.ctx, f.platform, f.dbID)
	if err != nil {
		f.t.Fatal(err)
	}
	return v
}

func (f *flow) has(table string) bool {
	var ok bool
	f.target.QueryRow(`SELECT to_regclass($1) IS NOT NULL`, "public."+table).Scan(&ok)
	return ok
}

func errCode(err error) string {
	var e *apperr.Error
	if errors.As(err, &e) {
		return e.Code
	}
	return ""
}

func TestRunRollbackForwardThroughTheWorker(t *testing.T) {
	f := setupFlow(t)
	a := f.recipe("CREATE TABLE a (id int)", "DROP TABLE a")

	if _, err := f.svc.Submit(f.ctx, f.viewer.ID, a, models.ExecKindRun); errCode(err) != "FORBIDDEN" {
		t.Fatalf("viewer submit = %v", err)
	}

	run := f.do(a, models.ExecKindRun)
	if run.Status != models.ExecSuccess || !f.has("a") || f.current() != 1 {
		t.Fatalf("run = %s, table %v, current %d", run.Status, f.has("a"), f.current())
	}
	var m models.Migration
	f.platform.First(&m, a)
	if m.Version == nil || *m.Version != 1 || m.FrozenAt == nil {
		t.Fatalf("first run must assign v1 and freeze: %+v", m)
	}

	undo := f.do(a, models.ExecKindRollback)
	if undo.Status != models.ExecSuccess || undo.RevertsExecutionID == nil || *undo.RevertsExecutionID != run.ID || undo.SQLExecuted != "DROP TABLE a" {
		t.Fatalf("rollback = %+v", undo)
	}
	if f.has("a") || f.current() != 0 {
		t.Fatalf("after rollback: table %v, current %d", f.has("a"), f.current())
	}
	if _, err := f.svc.Submit(f.ctx, f.editor.ID, a, models.ExecKindRollback); errCode(err) != "ROLLBACK_NOT_ALLOWED" {
		t.Errorf("second rollback = %v", err)
	}

	redo := f.do(a, models.ExecKindForward)
	if redo.Status != models.ExecSuccess || !f.has("a") || f.current() != 1 {
		t.Fatalf("forward = %s, table %v, current %d", redo.Status, f.has("a"), f.current())
	}
}

// The API view tells the SPA whether a migration is applied and whether a forward may run
// it, from the same derived position as the version rules.
func TestViewShowsActiveAndForwardable(t *testing.T) {
	f := setupFlow(t)
	a := f.recipe("CREATE TABLE a (id int)", "DROP TABLE a")
	b := f.recipe("CREATE TABLE b (id int)", "DROP TABLE b")
	check := func(step string, id uint, active, forwardable bool) {
		t.Helper()
		v, err := f.svc.Get(f.ctx, f.viewer.ID, id)
		if err != nil {
			t.Fatal(err)
		}
		if v.Active != active || v.Forwardable != forwardable {
			t.Errorf("%s: migration %d active %v forwardable %v, want %v %v", step, id, v.Active, v.Forwardable, active, forwardable)
		}
	}
	check("draft", a, false, false)

	f.do(a, models.ExecKindRun)
	f.do(b, models.ExecKindRun)
	check("after runs", a, true, false)
	check("after runs", b, true, false)

	f.do(b, models.ExecKindRollback)
	f.do(a, models.ExecKindRollback)
	check("after rollbacks", a, false, true)
	check("after rollbacks", b, false, false)
	list, err := f.svc.List(f.ctx, f.viewer.ID, f.dbID, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range list.Migrations {
		if v.Active {
			t.Errorf("list after rollbacks: v%d still active", *v.Version)
		}
	}
	if list.CurrentVersion != 0 {
		t.Errorf("list current = %d, want 0", list.CurrentVersion)
	}

	f.do(a, models.ExecKindForward)
	check("after forward", a, true, false)
	check("after forward", b, false, true)
}

func TestPositionFollowsRunOrderNotRequestOrder(t *testing.T) {
	f := setupFlow(t)
	a := f.recipe("CREATE TABLE a (id int)", "DROP TABLE a")
	b := f.recipe("CREATE TABLE b (id int)", "DROP TABLE b")
	requestA, err := f.svc.Submit(f.ctx, f.editor.ID, a, models.ExecKindRun)
	if err != nil {
		t.Fatal(err)
	}
	requestB, err := f.svc.Submit(f.ctx, f.editor.ID, b, models.ExecKindRun)
	if err != nil {
		t.Fatal(err)
	}

	runB := f.run(*requestB)
	runA := f.run(*requestA)
	if runA.Status != models.ExecSuccess || runB.Status != models.ExecSuccess {
		t.Fatalf("runs = %s, %s", runA.Status, runB.Status)
	}

	version := func(id uint) int {
		var m models.Migration
		f.platform.First(&m, id)
		if m.Version == nil {
			t.Fatalf("migration %d has no version", id)
		}
		return *m.Version
	}
	if version(b) != 1 || version(a) != 2 || f.current() != 2 {
		t.Fatalf("versions b=%d a=%d current=%d, want 1 2 2", version(b), version(a), f.current())
	}
	if _, err := f.svc.Submit(f.ctx, f.editor.ID, b, models.ExecKindRollback); errCode(err) != "ROLLBACK_NOT_ALLOWED" {
		t.Errorf("rollback of the migration that ran first = %v", err)
	}

	history, err := f.svc.History(f.ctx, f.viewer.ID, f.dbID, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 2 || history[0].MigrationID != b || history[1].MigrationID != a {
		t.Errorf("history = %+v, want b before a", history)
	}

	if _, err := f.svc.Submit(f.ctx, f.editor.ID, a, models.ExecKindRollback); err != nil {
		t.Errorf("rollback of the migration that ran last = %v", err)
	}
}

func TestFailedRunCanBeForwardedUntilANewRunDiscardsIt(t *testing.T) {
	f := setupFlow(t)
	if _, err := f.target.Exec(`CREATE TABLE blocker (id int)`); err != nil {
		t.Fatal(err)
	}
	bad := f.recipe("CREATE TABLE blocker (id int)", "DROP TABLE blocker")
	if e := f.do(bad, models.ExecKindRun); e.Status != models.ExecFailed {
		t.Fatalf("run = %s, want failed", e.Status)
	}
	if f.current() != 0 {
		t.Fatalf("failed run changed the position to %d", f.current())
	}
	if _, err := f.svc.Submit(f.ctx, f.editor.ID, bad, models.ExecKindRun); errCode(err) != "EXECUTION_STATE_CONFLICT" {
		t.Errorf("second run = %v, want EXECUTION_STATE_CONFLICT", err)
	}

	f.target.Exec(`DROP TABLE blocker`)
	if e := f.do(bad, models.ExecKindForward); e.Status != models.ExecSuccess || f.current() != 1 {
		t.Fatalf("forward of the failed attempt = %s, current %d", e.Status, f.current())
	}

	// Roll it back, then run something new: the rolled back migration is discarded.
	f.do(bad, models.ExecKindRollback)
	other := f.recipe("CREATE TABLE other (id int)", "DROP TABLE other")
	if e := f.do(other, models.ExecKindRun); e.Status != models.ExecSuccess || f.current() != 2 {
		t.Fatalf("new run = %s, current %d", e.Status, f.current())
	}
	if _, err := f.svc.Submit(f.ctx, f.editor.ID, bad, models.ExecKindForward); errCode(err) != "FORWARD_NOT_ALLOWED" {
		t.Errorf("forward after a new run = %v, want FORWARD_NOT_ALLOWED", err)
	}
	if _, err := f.svc.Duplicate(f.ctx, f.editor.ID, bad); err != nil {
		t.Errorf("duplicate of a discarded migration: %v", err)
	}
}

func TestAFailedRunKeepsTheRolledBackMigrationForwardable(t *testing.T) {
	f := setupFlow(t)
	if _, err := f.target.Exec(`CREATE TABLE blocker (id int)`); err != nil {
		t.Fatal(err)
	}
	a := f.recipe("CREATE TABLE a (id int)", "DROP TABLE a")
	b := f.recipe("CREATE TABLE blocker (id int)", "DROP TABLE blocker")
	f.do(a, models.ExecKindRun)
	f.do(a, models.ExecKindRollback)
	if e := f.do(b, models.ExecKindRun); e.Status != models.ExecFailed {
		t.Fatalf("run of b = %s, want failed", e.Status)
	}

	for _, id := range []uint{a, b} {
		v, err := f.svc.Get(f.ctx, f.viewer.ID, id)
		if err != nil {
			t.Fatal(err)
		}
		if v.Active || !v.Forwardable {
			t.Errorf("migration %d active %v forwardable %v, want only forwardable", id, v.Active, v.Forwardable)
		}
		e, err := f.svc.Submit(f.ctx, f.editor.ID, id, models.ExecKindForward)
		if err != nil {
			t.Fatalf("forward of %d = %v", id, err)
		}
		f.setStatus(*e, models.ExecCanceled)
	}
}

func TestARunThatFailedWithoutAVersionCanBeRunAgain(t *testing.T) {
	f := setupFlow(t)
	a := f.recipe("CREATE TABLE a (id int)", "DROP TABLE a")
	first, err := f.svc.Submit(f.ctx, f.editor.ID, a, models.ExecKindRun)
	if err != nil {
		t.Fatal(err)
	}
	f.setStatus(*first, models.ExecFailed)

	if e := f.do(a, models.ExecKindRun); e.Status != models.ExecSuccess || f.current() != 1 {
		t.Fatalf("second run = %s %s, current %d", e.Status, e.ErrorMessage, f.current())
	}
	v, err := f.svc.Get(f.ctx, f.viewer.ID, a)
	if err != nil {
		t.Fatal(err)
	}
	if !v.Active || v.Forwardable {
		t.Errorf("active %v forwardable %v, want only active", v.Active, v.Forwardable)
	}
	if _, err := f.svc.Submit(f.ctx, f.editor.ID, a, models.ExecKindRun); errCode(err) != "EXECUTION_STATE_CONFLICT" {
		t.Errorf("third run = %v, want EXECUTION_STATE_CONFLICT", err)
	}
}

func TestRevalidateAcceptsARetryAfterARunThatNeverStarted(t *testing.T) {
	f := setupFlow(t)
	a := f.recipe("CREATE TABLE a (id int)", "DROP TABLE a")
	first, err := f.svc.Submit(f.ctx, f.editor.ID, a, models.ExecKindRun)
	if err != nil {
		t.Fatal(err)
	}
	f.setStatus(*first, models.ExecFailed)
	second, err := f.svc.Submit(f.ctx, f.editor.ID, a, models.ExecKindRun)
	if err != nil {
		t.Fatal(err)
	}

	for attempt := 1; attempt <= 2; attempt++ {
		if err := f.svc.Revalidate(f.ctx, second); err != nil {
			t.Fatalf("revalidate attempt %d = %v", attempt, err)
		}
		var m models.Migration
		f.platform.First(&m, a)
		if m.Version == nil || *m.Version != 1 {
			t.Fatalf("version after attempt %d = %v, want 1", attempt, m.Version)
		}
	}
}

func TestAPartialExecutionIsContinuedOrUndone(t *testing.T) {
	f := setupFlow(t)
	a := f.recipe("CREATE TABLE IF NOT EXISTS a (id int)", "DROP TABLE IF EXISTS a")
	check := func(step string, active, forwardable bool) {
		t.Helper()
		v, err := f.svc.Get(f.ctx, f.viewer.ID, a)
		if err != nil {
			t.Fatal(err)
		}
		if v.Active != active || v.Forwardable != forwardable {
			t.Errorf("%s: active %v forwardable %v, want %v %v", step, v.Active, v.Forwardable, active, forwardable)
		}
	}
	refersTo := func(step string, got *uint, want uint) {
		t.Helper()
		if (got == nil) != (want == 0) || (got != nil && *got != want) {
			t.Errorf("%s = %v, want %d", step, got, want)
		}
	}

	run := f.do(a, models.ExecKindRun)
	f.setStatus(run, models.ExecPartial)
	check("partial run", false, false)
	if _, err := f.svc.Submit(f.ctx, f.editor.ID, a, models.ExecKindRun); errCode(err) != "EXECUTION_STATE_CONFLICT" {
		t.Errorf("run of a partial run = %v, want EXECUTION_STATE_CONFLICT", err)
	}

	continued := f.do(a, models.ExecKindForward)
	if continued.Status != models.ExecSuccess {
		t.Fatalf("continuation = %s %s", continued.Status, continued.ErrorMessage)
	}
	refersTo("continuation continues", continued.ContinuesExecutionID, run.ID)
	refersTo("continuation reverts", continued.RevertsExecutionID, 0)
	check("continued", true, false)

	undone := f.do(a, models.ExecKindRollback)
	if undone.Status != models.ExecSuccess {
		t.Fatalf("rollback = %s %s", undone.Status, undone.ErrorMessage)
	}
	refersTo("rollback reverts", undone.RevertsExecutionID, continued.ID)
	check("rolled back", false, true)

	again := f.do(a, models.ExecKindForward)
	refersTo("second forward continues", again.ContinuesExecutionID, 0)
	f.setStatus(again, models.ExecPartial)
	check("partial forward", false, false)

	undo := f.do(a, models.ExecKindRollback)
	refersTo("undo reverts", undo.RevertsExecutionID, again.ID)
	refersTo("undo continues", undo.ContinuesExecutionID, 0)
	f.setStatus(undo, models.ExecPartial)
	if _, err := f.svc.Submit(f.ctx, f.editor.ID, a, models.ExecKindForward); errCode(err) != "EXECUTION_STATE_CONFLICT" {
		t.Errorf("forward of a partial rollback = %v, want EXECUTION_STATE_CONFLICT", err)
	}

	finished := f.do(a, models.ExecKindRollback)
	if finished.Status != models.ExecSuccess {
		t.Fatalf("continued rollback = %s %s", finished.Status, finished.ErrorMessage)
	}
	refersTo("continued rollback reverts", finished.RevertsExecutionID, again.ID)
	refersTo("continued rollback continues", finished.ContinuesExecutionID, undo.ID)
	check("undone", false, true)
}

func TestRevalidateRefusesAChangedContinuation(t *testing.T) {
	f := setupFlow(t)
	a := f.recipe("CREATE TABLE IF NOT EXISTS a (id int)", "DROP TABLE IF EXISTS a")
	f.setStatus(f.do(a, models.ExecKindRun), models.ExecPartial)
	e, err := f.svc.Submit(f.ctx, f.editor.ID, a, models.ExecKindForward)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.svc.Revalidate(f.ctx, e); err != nil {
		t.Fatalf("revalidate = %v", err)
	}
	e.ContinuesExecutionID = nil
	if err := f.svc.Revalidate(f.ctx, e); err == nil {
		t.Error("revalidate accepted a forward that no longer continues the partial execution")
	}
}

func TestFailedRollbackKeepsThePosition(t *testing.T) {
	f := setupFlow(t)
	a := f.recipe("CREATE TABLE a (id int)", "DROP TABLE not_there")
	f.do(a, models.ExecKindRun)
	if e := f.do(a, models.ExecKindRollback); e.Status != models.ExecFailed {
		t.Fatalf("rollback = %s, want failed", e.Status)
	}
	if f.current() != 1 || !f.has("a") {
		t.Errorf("failed rollback moved the position: current %d, table %v", f.current(), f.has("a"))
	}
}

func TestRollbackOnlyTheTopAndNeverIrreversible(t *testing.T) {
	f := setupFlow(t)
	a := f.recipe("CREATE TABLE a (id int)", "DROP TABLE a")
	f.do(a, models.ExecKindRun)
	drop, err := f.svc.Create(f.ctx, f.editor.ID, f.dbID, migration.Recipe{SQLUp: "DROP TABLE a", Irreversible: true, IrreversibleReason: "data tabel hilang"})
	if err != nil {
		t.Fatal(err)
	}
	f.do(drop.ID, models.ExecKindRun)

	if _, err := f.svc.Submit(f.ctx, f.editor.ID, a, models.ExecKindRollback); errCode(err) != "ROLLBACK_NOT_ALLOWED" {
		t.Errorf("rollback below the top = %v", err)
	}
	if _, err := f.svc.Submit(f.ctx, f.editor.ID, drop.ID, models.ExecKindRollback); errCode(err) != "ROLLBACK_NOT_ALLOWED" {
		t.Errorf("rollback of an irreversible migration = %v", err)
	}
}

// The worker re-checks the rules under its lock; a request that became invalid while
// waiting fails with the reason instead of running.
func TestRevalidateFailsARequestThatBecameInvalid(t *testing.T) {
	f := setupFlow(t)
	a := f.recipe("CREATE TABLE a (id int)", "DROP TABLE a")
	f.do(a, models.ExecKindRun)

	pending, err := f.svc.Submit(f.ctx, f.editor.ID, a, models.ExecKindRollback)
	if err != nil {
		t.Fatal(err)
	}
	// Meanwhile another migration ran, so a is no longer on top.
	b := f.recipe("CREATE TABLE b (id int)", "DROP TABLE b")
	f.do(b, models.ExecKindRun)

	f.platform.Model(pending).Update("status", models.ExecQueued)
	claimed, _, err := f.w.Claim(f.ctx, pending.ID)
	if err != nil {
		t.Fatal(err)
	}
	f.w.Run(f.ctx, claimed)
	var got models.MigrationExecution
	f.platform.First(&got, pending.ID)
	if got.Status != models.ExecFailed || got.ErrorCode != "REVALIDATION_FAILED" || got.ErrorMessage == "" {
		t.Fatalf("stale rollback = %s %s %q", got.Status, got.ErrorCode, got.ErrorMessage)
	}
	if !f.has("a") || f.current() != 2 {
		t.Errorf("stale rollback changed the target: table a %v, current %d", f.has("a"), f.current())
	}
}

func TestRevalidateRefusesChangedSQL(t *testing.T) {
	f := setupFlow(t)
	a := f.recipe("CREATE TABLE a (id int)", "DROP TABLE a")
	e, err := f.svc.Submit(f.ctx, f.editor.ID, a, models.ExecKindRun)
	if err != nil {
		t.Fatal(err)
	}
	e.SQLExecuted = "DROP SCHEMA public CASCADE"
	if err := f.svc.Revalidate(f.ctx, e); err == nil {
		t.Fatal("revalidate accepted SQL that differs from the recipe")
	}
	var m models.Migration
	f.platform.First(&m, a)
	if m.Version != nil || m.FrozenAt != nil {
		t.Errorf("refused revalidation still versioned or froze the recipe: %+v", m)
	}
}

func TestSubmitRefusesTransactionControlHiddenInAnEscapeString(t *testing.T) {
	f := setupFlow(t)
	id := f.recipe("CREATE TABLE leaked (id int); SELECT E'\\''; COMMIT; SELECT 1/0; --'", "DROP TABLE leaked")
	if _, err := f.svc.Submit(f.ctx, f.editor.ID, id, models.ExecKindRun); errCode(err) != "TRANSACTION_CONTROL_NOT_ALLOWED" {
		t.Fatalf("submit = %v", err)
	}
	if f.has("leaked") {
		t.Fatal("refused migration created a table")
	}
}

func TestRunForcesStandardConformingStringsOn(t *testing.T) {
	f := setupFlow(t)
	var dbName string
	if err := f.target.QueryRow(`SELECT current_database()`).Scan(&dbName); err != nil {
		t.Fatal(err)
	}
	quoted := `"` + strings.ReplaceAll(dbName, `"`, `""`) + `"`
	if _, err := f.target.Exec(`ALTER DATABASE ` + quoted + ` SET standard_conforming_strings = off`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := f.target.Exec(`ALTER DATABASE ` + quoted + ` RESET standard_conforming_strings`); err != nil {
			t.Error(err)
		}
	})
	f.target.SetMaxIdleConns(0)
	var before string
	if err := f.target.QueryRow(`SELECT current_setting('standard_conforming_strings')`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	if before != "off" {
		t.Fatalf("a fresh session sees standard_conforming_strings = %q, want off", before)
	}

	id := f.recipe("CREATE TABLE seen AS SELECT current_setting('standard_conforming_strings') AS value", "DROP TABLE seen")
	if run := f.do(id, models.ExecKindRun); run.Status != models.ExecSuccess {
		t.Fatalf("run = %s", run.Status)
	}
	var value string
	if err := f.target.QueryRow(`SELECT value FROM seen`).Scan(&value); err != nil {
		t.Fatal(err)
	}
	if value != "on" {
		t.Fatalf("standard_conforming_strings = %q during the run", value)
	}
}
