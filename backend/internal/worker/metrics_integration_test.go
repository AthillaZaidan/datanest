//go:build integration

package worker

import (
	"context"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	dto "github.com/prometheus/client_model/go"

	"github.com/AthillaZaidan/datanest/backend/internal/dblock"
	"github.com/AthillaZaidan/datanest/backend/internal/metrics"
	"github.com/AthillaZaidan/datanest/backend/internal/models"
)

func samples(t *testing.T, o prometheus.Observer) uint64 {
	t.Helper()
	var m dto.Metric
	if err := o.(prometheus.Metric).Write(&m); err != nil {
		t.Fatal(err)
	}
	return m.GetHistogram().GetSampleCount()
}

type counterDelta struct {
	t      *testing.T
	c      prometheus.Counter
	before float64
}

func watch(t *testing.T, c prometheus.Counter) counterDelta {
	return counterDelta{t: t, c: c, before: testutil.ToFloat64(c)}
}

func (d counterDelta) expect(what string, want float64) {
	d.t.Helper()
	if got := testutil.ToFloat64(d.c) - d.before; got != want {
		d.t.Errorf("%s grew by %v, want %v", what, got, want)
	}
}

func queuedAgo(d time.Duration) *time.Time {
	at := time.Now().Add(-d)
	return &at
}

func TestASuccessfulRunIsCounted(t *testing.T) {
	e := setup(t)
	attempts := watch(t, metrics.JobAttempts.WithLabelValues("run", "normal", "success"))
	claims := watch(t, metrics.JobClaims.WithLabelValues("claimed"))
	batches := watch(t, metrics.BatchesCommitted.WithLabelValues("run", "normal"))
	rows := watch(t, metrics.RowsProcessed.WithLabelValues("run", "normal"))
	duration := metrics.MigrationDuration.WithLabelValues("run", "normal", "success")
	wait := metrics.QueueWait.WithLabelValues("run", "normal")
	acquired := metrics.LockAcquire.WithLabelValues("acquired")
	durationBefore, waitBefore, acquiredBefore := samples(t, duration), samples(t, wait), samples(t, acquired)
	x := e.newExec(models.MigrationExecution{QueuedAt: queuedAgo(2 * time.Second)})

	e.mustProcess(x.ID)

	attempts.expect("success attempts", 1)
	claims.expect("claims", 1)
	batches.expect("committed batches", 1)
	rows.expect("processed rows", 95)
	if got := samples(t, duration) - durationBefore; got != 1 {
		t.Errorf("migration duration samples grew by %d, want 1", got)
	}
	if got := samples(t, wait) - waitBefore; got != 1 {
		t.Errorf("queue wait samples grew by %d, want 1", got)
	}
	if got := samples(t, acquired) - acquiredBefore; got != 1 {
		t.Errorf("lock acquire samples grew by %d, want 1", got)
	}
	if got := testutil.ToFloat64(metrics.JobsInProgress.WithLabelValues("run", "normal")); got != 0 {
		t.Errorf("jobs in progress = %v after the run, want 0", got)
	}
}

func TestEveryCommittedBatchIsCounted(t *testing.T) {
	e := setup(t)
	batches := watch(t, metrics.BatchesCommitted.WithLabelValues("run", "batch"))
	rows := watch(t, metrics.RowsProcessed.WithLabelValues("run", "batch"))
	attempts := watch(t, metrics.JobAttempts.WithLabelValues("run", "batch", "success"))
	x := e.newBatchExec(incrementQtyInRange, 10, models.MigrationExecution{})

	e.mustProcess(x.ID)

	e.expectStatus(x.ID, models.ExecSuccess)
	batches.expect("committed batches", 10)
	rows.expect("processed rows", 95)
	attempts.expect("success attempts", 1)
}

func TestABusyLockIsCountedAsLockContention(t *testing.T) {
	e := setup(t)
	if _, ok, err := dblock.New(e.platform).Acquire(context.Background(), e.conn.ID, 999, "someone-else", time.Minute); err != nil || !ok {
		t.Fatalf("take the lock: %v, %v", ok, err)
	}
	busy := watch(t, metrics.JobClaims.WithLabelValues("lock_busy"))
	busyAcquire := metrics.LockAcquire.WithLabelValues("busy")
	before := samples(t, busyAcquire)
	x := e.newExec(models.MigrationExecution{})

	if result, err := e.process(x.ID); err != nil || result != ClaimBusy {
		t.Fatalf("process = %v, %v; want ClaimBusy", result, err)
	}

	busy.expect("lock_busy claims", 1)
	if got := samples(t, busyAcquire) - before; got != 1 {
		t.Errorf("busy lock acquire samples grew by %d, want 1", got)
	}
}

func TestARetryIsCountedAndTheQueueWaitIsNotMeasuredTwice(t *testing.T) {
	e := setup(t)
	failOnce := true
	e.w.Hooks.BeforeCommit = func(uint, int) error {
		if failOnce {
			failOnce = false
			return ErrInjectedTransient
		}
		return nil
	}
	retries := watch(t, metrics.JobAttempts.WithLabelValues("run", "normal", "retry"))
	successes := watch(t, metrics.JobAttempts.WithLabelValues("run", "normal", "success"))
	batches := watch(t, metrics.BatchesCommitted.WithLabelValues("run", "normal"))
	wait := metrics.QueueWait.WithLabelValues("run", "normal")
	waitBefore := samples(t, wait)
	x := e.newExec(models.MigrationExecution{QueuedAt: queuedAgo(time.Second)})

	e.mustProcess(x.ID)
	e.expectStatus(x.ID, models.ExecQueued)
	e.makeDue(x.ID)
	e.mustProcess(x.ID)

	e.expectStatus(x.ID, models.ExecSuccess)
	retries.expect("retry attempts", 1)
	successes.expect("success attempts", 1)
	batches.expect("committed batches", 1)
	if got := samples(t, wait) - waitBefore; got != 1 {
		t.Errorf("queue wait samples grew by %d, want 1: a retry must not measure the queue wait again", got)
	}
}

func TestAnUnknownCommitIsCountedAsUncertain(t *testing.T) {
	e := setup(t)
	e.w.Hooks.BeforeCommit = func(uint, int) error { return ErrInjectedCommitUnknown }
	uncertain := watch(t, metrics.JobAttempts.WithLabelValues("run", "normal", "uncertain"))
	batches := watch(t, metrics.BatchesCommitted.WithLabelValues("run", "normal"))
	x := e.newExec(models.MigrationExecution{})

	e.mustProcess(x.ID)

	e.expectStatus(x.ID, models.ExecUncertain)
	uncertain.expect("uncertain attempts", 1)
	batches.expect("committed batches", 0)
}

func TestAPermanentFailureIsCountedAsFailed(t *testing.T) {
	e := setup(t)
	failed := watch(t, metrics.JobAttempts.WithLabelValues("run", "normal", "failed"))
	rows := watch(t, metrics.RowsProcessed.WithLabelValues("run", "normal"))
	x := e.newExec(models.MigrationExecution{SQLExecuted: `UPDATE items SET qty = 1 / 0`})

	e.mustProcess(x.ID)

	e.expectStatus(x.ID, models.ExecFailed)
	failed.expect("failed attempts", 1)
	rows.expect("processed rows", 0)
}

func TestAPartialBatchRunIsCountedAsPartial(t *testing.T) {
	e := setup(t)
	partial := watch(t, metrics.JobAttempts.WithLabelValues("run", "batch", "partial"))
	batches := watch(t, metrics.BatchesCommitted.WithLabelValues("run", "batch"))
	x := e.newBatchExec(failingFromBatchFive, 10, models.MigrationExecution{})

	e.mustProcess(x.ID)

	e.expectStatus(x.ID, models.ExecPartial)
	partial.expect("partial attempts", 1)
	batches.expect("committed batches", 4)
}
