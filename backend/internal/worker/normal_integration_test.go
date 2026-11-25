//go:build integration

package worker

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/AthillaZaidan/datanest/backend/internal/dblock"
	"github.com/AthillaZaidan/datanest/backend/internal/migration"
	"github.com/AthillaZaidan/datanest/backend/internal/models"
)

func TestNormalRunSucceedsAndRecordsEverything(t *testing.T) {
	e := setup(t)
	x := e.newExec(models.MigrationExecution{})

	e.mustProcess(x.ID)

	got := e.expectStatus(x.ID, models.ExecSuccess)
	if got.FinishedAt == nil || got.CommittedBatches != 1 || got.CommittedRows != 95 || got.Attempts != 1 {
		t.Errorf("row = %+v", got)
	}
	if e.sumQty() != 95 {
		t.Errorf("sum qty = %d, want 95 (applied exactly once)", e.sumQty())
	}
	entries := e.ledger(x.ID)
	if len(entries) != 1 || entries[0].BatchNo != migration.NormalBatch || entries[0].RowCount != 95 {
		t.Errorf("ledger = %+v", entries)
	}
	attempts := e.attempts(x.ID)
	if len(attempts) != 1 || attempts[0].Status != models.AttemptSuccess || attempts[0].SQLExecuted != incrementQty || attempts[0].FinishedAt == nil {
		t.Errorf("attempts = %+v", attempts)
	}
	if e.lockHeld(e.conn.ID) {
		t.Error("the lock is still held after the run")
	}
}

func TestADuplicateMessageCommitsOnce(t *testing.T) {
	e := setup(t)
	x := e.newExec(models.MigrationExecution{})

	e.mustProcess(x.ID)
	if result, err := e.process(x.ID); err != nil || result != ClaimNone {
		t.Fatalf("second process = %v, %v; want the duplicate to be ignored", result, err)
	}
	if e.sumQty() != 95 {
		t.Errorf("sum qty = %d, want 95", e.sumQty())
	}
}

func TestReplayingAnExecutionThatAlreadyCommittedDoesNotApplyItAgain(t *testing.T) {
	e := setup(t)
	x := e.newExec(models.MigrationExecution{})
	e.mustProcess(x.ID)

	e.platform.Exec(`UPDATE migration_executions SET status = 'queued', finished_at = NULL WHERE id = ?`, x.ID)
	e.mustProcess(x.ID)

	e.expectStatus(x.ID, models.ExecSuccess)
	if e.sumQty() != 95 {
		t.Errorf("sum qty = %d, want 95: the ledger must stop a second application", e.sumQty())
	}
	if len(e.ledger(x.ID)) != 1 {
		t.Errorf("ledger rows = %d, want 1", len(e.ledger(x.ID)))
	}
}

func TestAPermanentSQLErrorFailsWithoutRetry(t *testing.T) {
	e := setup(t)
	x := e.newExec(models.MigrationExecution{SQLExecuted: `UPDATE items SET qty = 1 / 0`})

	e.mustProcess(x.ID)

	got := e.expectStatus(x.ID, models.ExecFailed)
	if got.ErrorCode != "22012" || !strings.Contains(got.ErrorMessage, "division by zero") || got.Attempts != 1 {
		t.Errorf("row = %+v", got)
	}
	if e.sumQty() != 0 || len(e.ledger(x.ID)) != 0 {
		t.Error("a failed execution left data or a ledger row behind")
	}
	attempts := e.attempts(x.ID)
	if len(attempts) != 1 || attempts[0].Status != models.AttemptFailed || attempts[0].ErrorCode != "22012" {
		t.Errorf("attempts = %+v", attempts)
	}
	if e.lockHeld(e.conn.ID) {
		t.Error("the lock is still held after a failure")
	}
}

func TestTransientFailuresAreRetriedUpToTheLimit(t *testing.T) {
	e := setup(t)
	e.provider.down.Store(true)
	x := e.newExec(models.MigrationExecution{})

	for attempt := 1; attempt <= 2; attempt++ {
		e.mustProcess(x.ID)
		got := e.expectStatus(x.ID, models.ExecQueued)
		if got.Attempts != attempt || got.NotBefore == nil || got.PublishedAt != nil || got.ErrorCode != "TARGET_UNAVAILABLE" {
			t.Fatalf("after attempt %d: %+v", attempt, got)
		}
		e.makeDue(x.ID)
	}
	e.mustProcess(x.ID)

	got := e.expectStatus(x.ID, models.ExecFailed)
	if got.Attempts != 3 || !strings.Contains(got.ErrorMessage, "Gagal setelah 3 percobaan") {
		t.Errorf("row = %+v", got)
	}
	for _, leak := range []string{"refused", "connection", "localhost", "127.0.0.1"} {
		if strings.Contains(strings.ToLower(got.ErrorMessage), leak) {
			t.Errorf("the error shown to every viewer leaks driver detail %q: %s", leak, got.ErrorMessage)
		}
	}
	for _, a := range e.attempts(x.ID) {
		if strings.Contains(strings.ToLower(a.ErrorMessage), "refused") {
			t.Errorf("attempt %d leaks driver detail: %s", a.Number, a.ErrorMessage)
		}
	}
	if n := len(e.attempts(x.ID)); n != 3 {
		t.Errorf("attempt rows = %d, want 3", n)
	}
}

func TestATransientFailureRecoversOnTheNextAttempt(t *testing.T) {
	e := setup(t)
	e.provider.down.Store(true)
	x := e.newExec(models.MigrationExecution{})

	e.mustProcess(x.ID)
	e.expectStatus(x.ID, models.ExecQueued)

	e.provider.down.Store(false)
	e.makeDue(x.ID)
	e.mustProcess(x.ID)

	e.expectStatus(x.ID, models.ExecSuccess)
	if e.sumQty() != 95 {
		t.Errorf("sum qty = %d, want 95", e.sumQty())
	}
	attempts := e.attempts(x.ID)
	if len(attempts) != 2 || attempts[0].Status != models.AttemptFailed || attempts[1].Status != models.AttemptSuccess {
		t.Errorf("attempts = %+v", attempts)
	}
}

func TestARetryIsNotClaimableBeforeItsDelay(t *testing.T) {
	e := setup(t)
	e.provider.down.Store(true)
	e.w.cfg.RetryDelays = []time.Duration{time.Hour}
	x := e.newExec(models.MigrationExecution{})
	e.mustProcess(x.ID)

	e.provider.down.Store(false)
	if result, err := e.process(x.ID); err != nil || result != ClaimNone {
		t.Fatalf("process before the delay = %v, %v; want it to be ignored", result, err)
	}
	e.expectStatus(x.ID, models.ExecQueued)
}

func TestABusyLockPostponesWithoutUsingAnAttempt(t *testing.T) {
	e := setup(t)
	x := e.newExec(models.MigrationExecution{})
	other := dblock.New(e.platform)
	if _, ok, _ := other.Acquire(context.Background(), e.conn.ID, 999, "someone-else", time.Minute); !ok {
		t.Fatal("setup lock failed")
	}

	result, err := e.process(x.ID)
	if err != nil || result != ClaimBusy {
		t.Fatalf("process = %v, %v; want busy", result, err)
	}
	got := e.expectStatus(x.ID, models.ExecQueued)
	if got.Attempts != 0 || got.NotBefore == nil || got.PublishedAt != nil {
		t.Errorf("row = %+v", got)
	}
	if e.sumQty() != 0 {
		t.Error("the SQL ran although the lock was busy")
	}
}

func TestRevalidationFailureFailsTheExecutionBeforeAnySQLRuns(t *testing.T) {
	e := setup(t)
	e.w.Revalidate = func(context.Context, *models.MigrationExecution) error {
		return errors.New("rollback hanya untuk migration teratas")
	}
	x := e.newExec(models.MigrationExecution{})

	e.mustProcess(x.ID)

	got := e.expectStatus(x.ID, models.ExecFailed)
	if got.ErrorCode != "REVALIDATION_FAILED" || !strings.Contains(got.ErrorMessage, "teratas") {
		t.Errorf("row = %+v", got)
	}
	if e.sumQty() != 0 || e.lockHeld(e.conn.ID) {
		t.Error("SQL ran or the lock is still held")
	}
}

func TestAnUncertainExecutionHoldsTheNextOneOnTheSameDatabase(t *testing.T) {
	e := setup(t)
	blocker := e.newExec(models.MigrationExecution{Status: models.ExecUncertain})
	x := e.newExec(models.MigrationExecution{})

	e.expectHeld(x.ID, blocker.ID)
	if e.sumQty() != 0 {
		t.Fatalf("sum qty = %d: the held SQL ran", e.sumQty())
	}

	e.mustProcess(blocker.ID)
	e.expectStatus(blocker.ID, models.ExecSuccess)
	e.makeDue(x.ID)
	e.mustProcess(x.ID)
	got := e.expectStatus(x.ID, models.ExecSuccess)
	if got.ErrorCode != "" || got.ErrorMessage != "" || e.sumQty() != 190 {
		t.Errorf("after the blocker recovered: row %+v, sum qty %d", got, e.sumQty())
	}
}

func TestAnExecutionWaitingForARetryIsNotOvertaken(t *testing.T) {
	e := setup(t)
	other := e.secondDatabase()
	failOnce := true
	e.w.Hooks.BeforeCommit = func(uint, int) error {
		if failOnce {
			failOnce = false
			return ErrInjectedTransient
		}
		return nil
	}
	retrying := e.newExec(models.MigrationExecution{})
	fresh := e.newExec(models.MigrationExecution{})
	elsewhere := e.newExec(models.MigrationExecution{DatabaseID: other.ID})
	e.mustProcess(retrying.ID)
	e.expectStatus(retrying.ID, models.ExecQueued)

	e.expectHeld(fresh.ID, retrying.ID)
	e.mustProcess(elsewhere.ID)
	e.expectStatus(elsewhere.ID, models.ExecSuccess)

	e.makeDue(retrying.ID)
	e.mustProcess(retrying.ID)
	e.expectStatus(retrying.ID, models.ExecSuccess)
	e.makeDue(fresh.ID)
	e.mustProcess(fresh.ID)
	e.expectStatus(fresh.ID, models.ExecSuccess)
	if first, second := e.get(retrying.ID), e.get(fresh.ID); *first.RunSeq >= *second.RunSeq {
		t.Errorf("run order = %d then %d, want the retried one first", *first.RunSeq, *second.RunSeq)
	}
}

func TestADifferentDatabaseIsNotBlocked(t *testing.T) {
	e := setup(t)
	other := e.secondDatabase()
	e.newExec(models.MigrationExecution{Status: models.ExecUncertain})
	x := e.newExec(models.MigrationExecution{DatabaseID: other.ID})

	e.mustProcess(x.ID)
	e.expectStatus(x.ID, models.ExecSuccess)
}

func TestDifferentDatabasesRunInParallelButOneDatabaseRunsOneAtATime(t *testing.T) {
	e := setup(t)
	other := e.secondDatabase()
	if _, err := e.target.Exec(`CREATE TABLE items2 (id int PRIMARY KEY, qty int NOT NULL DEFAULT 0); INSERT INTO items2 (id) VALUES (1)`); err != nil {
		t.Fatal(err)
	}
	g := newGate()
	e.w.Hooks.BeforeCommit = g.beforeCommit
	first := e.newExec(models.MigrationExecution{})
	sameDatabase := e.newExec(models.MigrationExecution{})
	elsewhere := e.newExec(models.MigrationExecution{DatabaseID: other.ID, SQLExecuted: `UPDATE items2 SET qty = qty + 1`})

	var wg sync.WaitGroup
	for _, id := range []uint{first.ID, elsewhere.ID} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			e.process(id)
		}()
	}
	for i := 0; i < 2; i++ {
		select {
		case <-g.arrived:
		case <-time.After(10 * time.Second):
			t.Fatal("the two databases did not run at the same time")
		}
	}

	if result, err := e.process(sameDatabase.ID); err != nil || result != ClaimBusy {
		t.Fatalf("second execution on a busy database = %v, %v; want busy", result, err)
	}
	close(g.release)
	wg.Wait()

	e.expectStatus(first.ID, models.ExecSuccess)
	e.expectStatus(elsewhere.ID, models.ExecSuccess)
	e.expectStatus(sameDatabase.ID, models.ExecQueued)

	e.platform.Exec(`UPDATE migration_executions SET not_before = NULL WHERE id = ?`, sameDatabase.ID)
	e.mustProcess(sameDatabase.ID)
	e.expectStatus(sameDatabase.ID, models.ExecSuccess)
}
