//go:build integration

package worker

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AthillaZaidan/datanest/backend/internal/dblock"
	"github.com/AthillaZaidan/datanest/backend/internal/models"
)

func TestACommitThatActuallyHappenedIsRecoveredFromTheLedger(t *testing.T) {
	e := setup(t)
	var once atomic.Bool
	e.w.Hooks.AfterCommit = func(uint, int) error {
		if once.CompareAndSwap(false, true) {
			return ErrInjectedCommitUnknown
		}
		return nil
	}
	x := e.newExec(models.MigrationExecution{})

	e.mustProcess(x.ID)

	got := e.expectStatus(x.ID, models.ExecUncertain)
	if got.ErrorCode != "COMMIT_UNKNOWN" || e.lockHeld(e.conn.ID) || e.sumQty() != 95 {
		t.Fatalf("row = %+v, sum %d", got, e.sumQty())
	}
	if strings.Contains(got.ErrorMessage, "injected") {
		t.Errorf("the commit error text reached the row: %s", got.ErrorMessage)
	}
	if attempts := e.attempts(x.ID); len(attempts) != 1 || attempts[0].Status != models.AttemptUncertain {
		t.Errorf("attempts = %+v", attempts)
	}

	e.mustProcess(x.ID)

	e.expectStatus(x.ID, models.ExecSuccess)
	if e.sumQty() != 95 {
		t.Errorf("sum qty = %d, want 95: recovery must not apply the SQL again", e.sumQty())
	}
}

func TestACommitThatNeverHappenedIsRetriedByRecovery(t *testing.T) {
	e := setup(t)
	var once atomic.Bool
	e.w.Hooks.BeforeCommit = func(uint, int) error {
		if once.CompareAndSwap(false, true) {
			return ErrInjectedCommitUnknown
		}
		return nil
	}
	x := e.newExec(models.MigrationExecution{})

	e.mustProcess(x.ID)
	e.expectStatus(x.ID, models.ExecUncertain)
	if e.sumQty() != 0 || len(e.ledger(x.ID)) != 0 {
		t.Fatal("the simulated lost commit left data behind")
	}

	e.mustProcess(x.ID)

	e.expectStatus(x.ID, models.ExecSuccess)
	if e.sumQty() != 95 {
		t.Errorf("sum qty = %d, want 95", e.sumQty())
	}
}

func TestAnUncertainExecutionStaysUncertainWhileTheTargetCannotBeChecked(t *testing.T) {
	e := setup(t)
	x := e.newExec(models.MigrationExecution{Status: models.ExecUncertain})
	e.provider.down.Store(true)

	e.mustProcess(x.ID)

	got := e.expectStatus(x.ID, models.ExecUncertain)
	if got.NotBefore == nil || got.ErrorCode != "TARGET_UNAVAILABLE" || got.PublishedAt != nil {
		t.Errorf("row = %+v", got)
	}
	for i := 0; i < 4; i++ {
		e.makeDue(x.ID)
		e.mustProcess(x.ID)
	}
	e.expectStatus(x.ID, models.ExecUncertain)

	next := e.newExec(models.MigrationExecution{})
	e.provider.down.Store(false)
	e.expectHeld(next.ID, x.ID)

	e.makeDue(x.ID)
	e.mustProcess(x.ID)
	e.expectStatus(x.ID, models.ExecSuccess)
}

func TestAZombieThatCommitsAfterATakeoverCannotApplyTheSQLTwice(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	g := newGate()
	e.w.Hooks.BeforeCommit = g.beforeCommit
	x := e.newExec(models.MigrationExecution{})

	claimedA, result, err := e.w.Claim(ctx, x.ID)
	if err != nil || result != ClaimOK {
		t.Fatalf("claim by the first worker = %v, %v", result, err)
	}
	aDone := make(chan error, 1)
	go func() { aDone <- e.w.Run(ctx, claimedA) }()
	<-g.arrived

	e.expireLock(e.conn.ID)
	if swept, err := e.w.Sweep(ctx); err != nil || swept.Dead != 1 {
		t.Fatalf("sweep = %+v, %v", swept, err)
	}
	other := New(e.platform, e.provider, testConfig())
	claimedB, result, err := other.Claim(ctx, x.ID)
	if err != nil || result != ClaimOK || claimedB.Lease.Token != claimedA.Lease.Token+1 {
		t.Fatalf("takeover = %v, %v, token %d after %d", result, err, claimedB.Lease.Token, claimedA.Lease.Token)
	}
	bDone := make(chan error, 1)
	go func() { bDone <- other.Run(ctx, claimedB) }()
	time.Sleep(300 * time.Millisecond)

	close(g.release)
	for _, done := range []chan error{aDone, bDone} {
		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("a worker returned %v", err)
			}
		case <-time.After(10 * time.Second):
			t.Fatal("a worker never finished")
		}
	}

	if got := e.get(x.ID); got.Status != models.ExecUncertain && got.Status != models.ExecSuccess {
		t.Fatalf("after the race the execution is %s (%s)", got.Status, got.ErrorMessage)
	}
	if e.get(x.ID).Status == models.ExecUncertain {
		if result, err := processWith(t, other, x.ID); err != nil || result != ClaimOK {
			t.Fatalf("recovery = %v, %v", result, err)
		}
	}

	e.expectStatus(x.ID, models.ExecSuccess)
	if e.sumQty() != 95 || len(e.ledger(x.ID)) != 1 {
		t.Errorf("sum qty = %d, ledger rows = %d; want the SQL applied exactly once", e.sumQty(), len(e.ledger(x.ID)))
	}
}

func TestALostLockStopsTheRunWithoutWritingAnything(t *testing.T) {
	cfg := testConfig()
	cfg.HeartbeatEvery = 20 * time.Millisecond
	cfg.LeaseTTL = 300 * time.Millisecond
	e := setupWith(t, cfg)
	x := e.newExec(models.MigrationExecution{SQLExecuted: `UPDATE items SET qty = qty + 1 WHERE (SELECT pg_sleep(8)) IS NOT NULL`})
	claimed, result, err := e.w.Claim(context.Background(), x.ID)
	if err != nil || result != ClaimOK {
		t.Fatalf("claim = %v, %v", result, err)
	}
	done := make(chan error, 1)
	go func() { done <- e.w.Run(context.Background(), claimed) }()
	time.Sleep(200 * time.Millisecond)

	e.platform.Exec(`UPDATE database_locks SET owner = 'thief', token = token + 1, lease_until = now() + interval '1 minute' WHERE database_id = ?`, e.conn.ID)

	select {
	case err := <-done:
		if !errors.Is(err, errLockLost) {
			t.Fatalf("run = %v, want the lost-lock error", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the run kept going after it lost its lock")
	}
	got := e.expectStatus(x.ID, models.ExecRunning)
	if got.FinishedAt != nil || e.sumQty() != 0 || len(e.ledger(x.ID)) != 0 {
		t.Errorf("a worker that lost its lock wrote something: %+v", got)
	}
	var owner string
	e.platform.Raw(`SELECT owner FROM database_locks WHERE database_id = ?`, e.conn.ID).Scan(&owner)
	if owner != "thief" {
		t.Errorf("the lock owner is %q; the loser must not release the new owner's lock", owner)
	}
}

func TestAHeartbeatKeepsALongRunAlive(t *testing.T) {
	cfg := testConfig()
	cfg.HeartbeatEvery = 100 * time.Millisecond
	cfg.LeaseTTL = 1500 * time.Millisecond
	e := setupWith(t, cfg)
	x := e.newExec(models.MigrationExecution{SQLExecuted: `UPDATE items SET qty = qty + 1 WHERE (SELECT pg_sleep(4)) IS NOT NULL`})
	claimed, result, err := e.w.Claim(context.Background(), x.ID)
	if err != nil || result != ClaimOK {
		t.Fatalf("claim = %v, %v", result, err)
	}
	done := make(chan error, 1)
	go func() { done <- e.w.Run(context.Background(), claimed) }()

	time.Sleep(3 * time.Second)
	if !e.lockHeld(e.conn.ID) {
		t.Error("the lease expired during a long run although the heartbeat was running")
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	e.expectStatus(x.ID, models.ExecSuccess)
}

func TestWithoutAHeartbeatTheLeaseExpiresEvenWhileTheRunContinues(t *testing.T) {
	cfg := testConfig()
	cfg.HeartbeatEvery = 20 * time.Millisecond
	cfg.LeaseTTL = 300 * time.Millisecond
	e := setupWith(t, cfg)
	e.w.Hooks.BeforeHeartbeat = func(uint) error { return ErrCrash }
	x := e.newExec(models.MigrationExecution{SQLExecuted: `UPDATE items SET qty = qty + 1 WHERE (SELECT pg_sleep(1.5)) IS NOT NULL`})
	claimed, _, _ := e.w.Claim(context.Background(), x.ID)
	done := make(chan error, 1)
	go func() { done <- e.w.Run(context.Background(), claimed) }()

	time.Sleep(800 * time.Millisecond)
	if e.lockHeld(e.conn.ID) {
		t.Error("the lease was extended although the heartbeat stopped")
	}
	if _, err := e.w.Sweep(context.Background()); err != nil {
		t.Fatal(err)
	}
	e.expectStatus(x.ID, models.ExecUncertain)
	<-done
}

func TestLockTokenGrowsAcrossTakeovers(t *testing.T) {
	e := setup(t)
	locks := dblock.New(e.platform)
	first, _, _ := locks.Acquire(context.Background(), e.conn.ID, 1, "a", time.Minute)
	e.expireLock(e.conn.ID)
	second, ok, _ := locks.Acquire(context.Background(), e.conn.ID, 2, "b", time.Minute)
	if !ok || second.Token <= first.Token {
		t.Errorf("tokens %d then %d", first.Token, second.Token)
	}
}

func TestSweepPublishesEachQueuedExecutionOncePerWindow(t *testing.T) {
	e := setup(t)
	a := e.newExec(models.MigrationExecution{Traceparent: "00-aaaa-bbbb-01", CorrelationID: "corr-a"})
	b := e.newExec(models.MigrationExecution{})
	e.newExec(models.MigrationExecution{Status: models.ExecApproved})

	first, err := e.w.Sweep(context.Background())
	if err != nil || first.Published != 2 {
		t.Fatalf("first sweep = %+v, %v", first, err)
	}
	if ids := e.pub.ids(); len(ids) != 2 || ids[0] != a.ID || ids[1] != b.ID {
		t.Errorf("published = %v", ids)
	}
	if job := e.pub.jobs[0]; job.Traceparent != "00-aaaa-bbbb-01" || job.CorrelationID != "corr-a" {
		t.Errorf("job = %+v", job)
	}
	if again, _ := e.w.Sweep(context.Background()); again.Published != 0 {
		t.Errorf("second sweep republished %d executions inside the window", again.Published)
	}

	e.platform.Exec(`UPDATE migration_executions SET published_at = now() - interval '2 minutes' WHERE id = ?`, a.ID)
	if later, _ := e.w.Sweep(context.Background()); later.Published != 1 {
		t.Errorf("sweep after the window published %d, want 1", later.Published)
	}
}

func TestSweepKeepsAnExecutionDueWhenThePublishFails(t *testing.T) {
	e := setup(t)
	x := e.newExec(models.MigrationExecution{})
	e.pub.fail = true

	if res, err := e.w.Sweep(context.Background()); err != nil || res.Published != 0 {
		t.Fatalf("sweep = %+v, %v", res, err)
	}
	if e.get(x.ID).PublishedAt != nil {
		t.Fatal("an execution that was never delivered was marked as published")
	}
	e.pub.fail = false
	if res, _ := e.w.Sweep(context.Background()); res.Published != 1 {
		t.Errorf("the next sweep published %d, want 1", res.Published)
	}
}

func TestSweepLeavesAHealthyRunningExecutionAlone(t *testing.T) {
	e := setup(t)
	x := e.newExec(models.MigrationExecution{})
	claimed, _, _ := e.w.Claim(context.Background(), x.ID)

	res, err := e.w.Sweep(context.Background())
	if err != nil || res.Dead != 0 {
		t.Fatalf("sweep = %+v, %v", res, err)
	}
	e.expectStatus(x.ID, models.ExecRunning)
	e.w.Run(context.Background(), claimed)
	e.expectStatus(x.ID, models.ExecSuccess)
}

func TestAPostponedExecutionIsPublishedAgainOnlyAfterItsDelay(t *testing.T) {
	e := setup(t)
	locks := dblock.New(e.platform)
	locks.Acquire(context.Background(), e.conn.ID, 999, "someone-else", time.Minute)
	x := e.newExec(models.MigrationExecution{})
	if result, _ := e.process(x.ID); result != ClaimBusy {
		t.Fatal("expected a busy lock")
	}

	if res, _ := e.w.Sweep(context.Background()); res.Published != 0 {
		t.Fatal("published before the postponement ended")
	}
	e.makeDue(x.ID)
	if res, _ := e.w.Sweep(context.Background()); res.Published != 1 {
		t.Fatal("never published again once the postponement ended")
	}
	if got := e.get(x.ID); got.Status != models.ExecQueued || got.Attempts != 0 {
		t.Errorf("row = %+v; waiting for a busy lock must not use up attempts", got)
	}
}
