//go:build integration

package worker

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/AthillaZaidan/datanest/backend/internal/models"
)

const decrementQtyInRange = `UPDATE items SET qty = qty - 1 WHERE id > $1 AND id <= $2`

func fencedConfig(id string) Config {
	cfg := testConfig()
	cfg.WorkerID = id
	cfg.LockTimeout = 300 * time.Millisecond
	return cfg
}

func (e *env) loseWorker(id uint) {
	e.t.Helper()
	e.expireLock(e.conn.ID)
	if swept, err := e.w.Sweep(context.Background()); err != nil || swept.Dead != 1 {
		e.t.Fatalf("sweep = %+v, %v", swept, err)
	}
	e.makeDue(id)
}

func (e *env) crashOnce(id uint) {
	e.t.Helper()
	if _, err := e.process(id); !errors.Is(err, ErrCrash) {
		e.t.Fatalf("process = %v, want a crash", err)
	}
	e.loseWorker(id)
}

type zombie struct {
	e       *env
	release chan error
	done    chan error
}

func (e *env) stallBeforeCommit(id uint, batchNo int) *zombie {
	e.t.Helper()
	z := &zombie{e: e, release: make(chan error), done: make(chan error, 1)}
	arrived := make(chan struct{})
	e.w.Hooks.BeforeCommit = func(_ uint, n int) error {
		if n != batchNo {
			return nil
		}
		close(arrived)
		return <-z.release
	}
	claimed, result, err := e.w.Claim(context.Background(), id)
	if err != nil || result != ClaimOK {
		e.t.Fatalf("claim = %v, %v", result, err)
	}
	go func() { z.done <- e.w.Run(context.Background(), claimed) }()
	e.t.Cleanup(func() {
		select {
		case z.release <- ErrCrash:
			<-z.done
		default:
		}
	})
	select {
	case <-arrived:
	case <-time.After(10 * time.Second):
		e.t.Fatal("the run never reached its commit")
	}
	e.loseWorker(id)
	return z
}

func (z *zombie) wake(err error) error {
	z.release <- err
	return <-z.done
}

func (e *env) exhaust(id uint) {
	e.t.Helper()
	if err := e.platform.Exec(`UPDATE migration_executions SET sql_runs = ? WHERE id = ?`, testConfig().MaxAttempts, id).Error; err != nil {
		e.t.Fatal(err)
	}
}

func (e *env) processBy(w *Worker, id uint) {
	e.t.Helper()
	e.makeDue(id)
	if result, err := processWith(e.t, w, id); err != nil || result != ClaimOK {
		e.t.Fatalf("process %d = %v, %v", id, result, err)
	}
}

func TestACrashLoopFailsAfterTheLastAllowedStart(t *testing.T) {
	e := setupWith(t, fencedConfig("a"))
	x := e.newExec(models.MigrationExecution{})
	starts := 0
	e.w.Hooks.BeforeCommit = func(uint, int) error { starts++; return ErrCrash }
	for i := 0; i < 3; i++ {
		e.crashOnce(x.ID)
	}

	e.mustProcess(x.ID)

	got := e.expectStatus(x.ID, models.ExecFailed)
	if got.ErrorCode != "RETRY_EXHAUSTED" || starts != 3 {
		t.Errorf("code %s after %d starts, want RETRY_EXHAUSTED after 3", got.ErrorCode, starts)
	}
	if e.sumQty() != 0 || len(e.ledger(x.ID)) != 0 || e.lockHeld(e.conn.ID) {
		t.Error("the SQL was applied or the lock is still held")
	}
}

func TestACrashLoopOfABatchEndsPartialWithItsCommittedBatches(t *testing.T) {
	e := setupWith(t, fencedConfig("a"))
	x := e.newBatchExec(incrementQtyInRange, 10, models.MigrationExecution{})
	e.w.Hooks.BeforeCommit = func(_ uint, n int) error {
		if n == 3 {
			return ErrCrash
		}
		return nil
	}
	for i := 0; i < 3; i++ {
		e.crashOnce(x.ID)
	}

	e.mustProcess(x.ID)

	got := e.expectStatus(x.ID, models.ExecPartial)
	if got.ErrorCode != "RETRY_EXHAUSTED" || got.CommittedBatches != 2 || got.CommittedRows != 20 || e.sumQty() != 20 {
		t.Errorf("row = %+v, sum qty %d", got, e.sumQty())
	}
}

func TestACommittedExecutionIsRecoveredEvenAtTheStartLimit(t *testing.T) {
	e := setupWith(t, fencedConfig("a"))
	x := e.newExec(models.MigrationExecution{})
	e.w.Hooks.AfterCommit = func(uint, int) error { return ErrCrash }
	e.crashOnce(x.ID)
	e.exhaust(x.ID)

	e.mustProcess(x.ID)

	e.expectStatus(x.ID, models.ExecSuccess)
	if e.sumQty() != 95 {
		t.Errorf("sum qty = %d, want 95", e.sumQty())
	}
}

func TestEvidenceOfACommitWinsOverARefusedRevalidation(t *testing.T) {
	e := setupWith(t, fencedConfig("a"))
	x := e.newExec(models.MigrationExecution{})
	e.w.Hooks.AfterCommit = func(uint, int) error { return ErrCrash }
	e.crashOnce(x.ID)
	e.w.Revalidate = func(context.Context, *models.MigrationExecution) error { return errors.New("tidak lagi berlaku") }

	e.mustProcess(x.ID)

	e.expectStatus(x.ID, models.ExecSuccess)
}

func TestARefusedRevalidationKeepsCommittedBatchesPartial(t *testing.T) {
	e := setupWith(t, fencedConfig("a"))
	x := e.newBatchExec(incrementQtyInRange, 10, models.MigrationExecution{})
	e.w.Hooks.AfterCommit = func(_ uint, n int) error {
		if n == 4 {
			return ErrCrash
		}
		return nil
	}
	e.crashOnce(x.ID)
	e.w.Revalidate = func(context.Context, *models.MigrationExecution) error { return errors.New("tidak lagi berlaku") }

	e.mustProcess(x.ID)

	got := e.expectStatus(x.ID, models.ExecPartial)
	if got.ErrorCode != "REVALIDATION_FAILED" || got.CommittedBatches != 4 || got.CommittedRows != 40 {
		t.Errorf("row = %+v", got)
	}
}

func TestAnUnreachableTargetKeepsCommittedBatchesPartial(t *testing.T) {
	e := setupWith(t, fencedConfig("a"))
	x := e.newBatchExec(incrementQtyInRange, 10, models.MigrationExecution{})
	e.w.Hooks.BeforeCommit = func(_ uint, n int) error {
		if n == 5 {
			return ErrInjectedTransient
		}
		return nil
	}
	e.mustProcess(x.ID)
	e.expectStatus(x.ID, models.ExecQueued)
	e.provider.down.Store(true)
	for i := 0; i < 2; i++ {
		e.makeDue(x.ID)
		e.mustProcess(x.ID)
	}

	got := e.expectStatus(x.ID, models.ExecPartial)
	if got.CommittedBatches != 4 || got.CommittedRows != 40 || len(e.ledger(x.ID)) != 4 {
		t.Errorf("row = %+v", got)
	}
}

func TestAnUnreachableTargetOnTheFirstAttemptDoesNotRevalidate(t *testing.T) {
	e := setupWith(t, fencedConfig("a"))
	x := e.newExec(models.MigrationExecution{})
	revalidated := 0
	e.w.Revalidate = func(context.Context, *models.MigrationExecution) error { revalidated++; return nil }
	e.provider.down.Store(true)

	e.mustProcess(x.ID)

	e.expectStatus(x.ID, models.ExecQueued)
	if got := e.get(x.ID); revalidated != 0 || got.SQLRuns != 0 {
		t.Errorf("revalidated %d times, sql_runs %d; nothing started yet", revalidated, got.SQLRuns)
	}
}

func TestNoVerdictWhileAZombieMayStillCommit(t *testing.T) {
	e := setupWith(t, fencedConfig("a"))
	other := New(e.platform, e.provider, fencedConfig("b"))
	x := e.newExec(models.MigrationExecution{})
	z := e.stallBeforeCommit(x.ID, 0)
	e.exhaust(x.ID)

	e.processBy(other, x.ID)
	e.expectStatus(x.ID, models.ExecUncertain)
	z.wake(nil)
	e.expectStatus(x.ID, models.ExecUncertain)

	e.processBy(other, x.ID)
	e.expectStatus(x.ID, models.ExecSuccess)
	if e.sumQty() != 95 || len(e.ledger(x.ID)) != 1 {
		t.Errorf("sum qty = %d, ledger = %d; want the SQL applied once", e.sumQty(), len(e.ledger(x.ID)))
	}
}

func TestTheVerdictStandsOnceTheZombieRolledBack(t *testing.T) {
	e := setupWith(t, fencedConfig("a"))
	other := New(e.platform, e.provider, fencedConfig("b"))
	x := e.newExec(models.MigrationExecution{})
	z := e.stallBeforeCommit(x.ID, 0)
	e.exhaust(x.ID)
	z.wake(ErrCrash)

	e.processBy(other, x.ID)

	got := e.expectStatus(x.ID, models.ExecFailed)
	if got.ErrorCode != "RETRY_EXHAUSTED" || e.sumQty() != 0 {
		t.Errorf("row = %+v, sum qty %d", got, e.sumQty())
	}
}

func TestARefusedRevalidationWaitsForAZombie(t *testing.T) {
	e := setupWith(t, fencedConfig("a"))
	other := New(e.platform, e.provider, fencedConfig("b"))
	other.Revalidate = func(context.Context, *models.MigrationExecution) error { return errors.New("tidak lagi berlaku") }
	x := e.newExec(models.MigrationExecution{})
	z := e.stallBeforeCommit(x.ID, 0)

	e.processBy(other, x.ID)
	e.expectStatus(x.ID, models.ExecUncertain)
	z.wake(nil)
	e.processBy(other, x.ID)
	e.expectStatus(x.ID, models.ExecSuccess)
}

func TestABatchVerdictAgreesWithALedgerAZombieStillWrites(t *testing.T) {
	e := setupWith(t, fencedConfig("a"))
	other := New(e.platform, e.provider, fencedConfig("b"))
	x := e.newBatchExec(incrementQtyInRange, 10, models.MigrationExecution{})
	z := e.stallBeforeCommit(x.ID, 3)
	e.exhaust(x.ID)

	e.processBy(other, x.ID)
	e.expectStatus(x.ID, models.ExecUncertain)
	if err := z.wake(nil); !errors.Is(err, errLockLost) {
		t.Fatalf("the zombie ended with %v, want the lost lock", err)
	}
	e.processBy(other, x.ID)

	got := e.expectStatus(x.ID, models.ExecPartial)
	if got.CommittedBatches != 3 || len(e.ledger(x.ID)) != 3 || e.sumQty() != 30 {
		t.Errorf("row says %d batches, ledger %d, sum qty %d", got.CommittedBatches, len(e.ledger(x.ID)), e.sumQty())
	}
}

func TestAnUndoVerdictAgreesWithALedgerAZombieStillWrites(t *testing.T) {
	e := setupWith(t, fencedConfig("a"))
	other := New(e.platform, e.provider, fencedConfig("b"))
	run := e.newBatchExec(incrementQtyInRange, 10, models.MigrationExecution{})
	e.mustProcess(run.ID)
	x := e.newBatchExec(decrementQtyInRange, 10, models.MigrationExecution{Kind: models.ExecKindRollback, RevertsExecutionID: &run.ID})
	z := e.stallBeforeCommit(x.ID, 2)
	e.exhaust(x.ID)

	e.processBy(other, x.ID)
	e.expectStatus(x.ID, models.ExecUncertain)
	z.wake(nil)
	e.processBy(other, x.ID)

	got := e.expectStatus(x.ID, models.ExecPartial)
	if got.CommittedBatches != 2 || len(e.ledger(x.ID)) != 2 || e.sumQty() != 80 {
		t.Errorf("row says %d batches, ledger %d, sum qty %d; want 2, 2 and 80", got.CommittedBatches, len(e.ledger(x.ID)), e.sumQty())
	}
}

func TestRecoveryOfDDLWaitsForTheZombieInsteadOfCollidingWithIt(t *testing.T) {
	e := setupWith(t, fencedConfig("a"))
	other := New(e.platform, e.provider, fencedConfig("b"))
	x := e.newExec(models.MigrationExecution{SQLExecuted: `CREATE TABLE baru (id int)`})
	z := e.stallBeforeCommit(x.ID, 0)

	e.processBy(other, x.ID)
	e.expectStatus(x.ID, models.ExecUncertain)
	z.wake(nil)
	e.processBy(other, x.ID)
	e.expectStatus(x.ID, models.ExecSuccess)
}

func TestAWorkerThatLostItsClaimStartsNoSQL(t *testing.T) {
	e := setupWith(t, fencedConfig("a"))
	x := e.newExec(models.MigrationExecution{})
	claimed, result, err := e.w.Claim(context.Background(), x.ID)
	if err != nil || result != ClaimOK {
		t.Fatalf("claim = %v, %v", result, err)
	}
	e.loseWorker(x.ID)

	if err := e.w.Run(context.Background(), claimed); !errors.Is(err, errLockLost) {
		t.Fatalf("run = %v, want the lost lock", err)
	}
	e.expectStatus(x.ID, models.ExecUncertain)
	if got := e.get(x.ID); e.sumQty() != 0 || len(e.ledger(x.ID)) != 0 || got.SQLRuns != 0 {
		t.Errorf("sum qty %d, sql_runs %d", e.sumQty(), got.SQLRuns)
	}
}

func TestAWorkerThatLosesItsClaimBetweenBatchesAppliesNoMore(t *testing.T) {
	e := setupWith(t, fencedConfig("a"))
	x := e.newBatchExec(incrementQtyInRange, 10, models.MigrationExecution{})
	e.w.Hooks.AfterCommit = func(_ uint, n int) error {
		if n == 1 {
			e.loseWorker(x.ID)
		}
		return nil
	}

	if _, err := e.process(x.ID); !errors.Is(err, errLockLost) {
		t.Fatalf("run = %v, want the lost lock", err)
	}
	if e.sumQty() != 10 || len(e.ledger(x.ID)) != 1 {
		t.Errorf("sum qty %d, ledger %d; only the first batch may stay", e.sumQty(), len(e.ledger(x.ID)))
	}
	if attempts := e.attempts(x.ID); len(attempts) != 1 || attempts[0].Status != models.AttemptUncertain {
		t.Errorf("attempts = %+v; the loser must not rewrite the attempt", attempts)
	}
}

func TestTheLedgerRowHoldsTheRowCountOfItsBatch(t *testing.T) {
	e := setupWith(t, fencedConfig("a"))
	x := e.newBatchExec(incrementQtyInRange, 40, models.MigrationExecution{})
	e.mustProcess(x.ID)
	var got []int64
	for _, entry := range e.ledger(x.ID) {
		got = append(got, entry.RowCount)
	}
	if len(got) != 3 || got[0] != 40 || got[1] != 40 || got[2] != 15 {
		t.Errorf("row counts = %v, want 40, 40, 15", got)
	}
}
