//go:build integration

package worker

import (
	"context"
	"errors"
	"strconv"
	"sync/atomic"
	"testing"

	"github.com/AthillaZaidan/datanest/backend/internal/models"
)

const failingFromBatchFive = `UPDATE items SET qty = qty + 1 / CASE WHEN id > 40 THEN 0 ELSE 1 END WHERE id > $1 AND id <= $2`

func wantSpans(from, to, step int) [][2]string {
	var out [][2]string
	for lo := from; lo < to; lo += step {
		hi := min(lo+step, to)
		out = append(out, [2]string{strconv.Itoa(lo), strconv.Itoa(hi)})
	}
	return out
}

func sameSpans(t *testing.T, got, want [][2]string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("spans = %v, want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("spans = %v, want %v", got, want)
		}
	}
}

func TestABatchRunCommitsEveryBatchOnce(t *testing.T) {
	e := setup(t)
	x := e.newBatchExec(incrementQtyInRange, 10, models.MigrationExecution{})

	e.mustProcess(x.ID)

	got := e.expectStatus(x.ID, models.ExecSuccess)
	if got.CommittedBatches != 10 || got.CommittedRows != 95 || got.LastCursor == nil || *got.LastCursor != "95" ||
		got.ScopeMax == nil || *got.ScopeMax != "95" || got.EstimatedRows == nil || *got.EstimatedRows != 95 {
		t.Errorf("row = %+v", got)
	}
	entries := e.ledger(x.ID)
	sameSpans(t, spans(entries), wantSpans(0, 95, 10))
	for i, entry := range entries {
		if entry.BatchNo != i+1 {
			t.Errorf("batch numbers are not contiguous: %+v", entries)
		}
	}
	if e.sumQty() != 95 {
		t.Errorf("sum qty = %d, want 95", e.sumQty())
	}
}

func TestOnlyRowsInsideTheScopeAreTouched(t *testing.T) {
	e := setup(t)
	x := e.newBatchExec(incrementQtyInRange, 10, models.MigrationExecution{})
	scope := "50"
	estimated := int64(50)
	e.platform.Model(&models.MigrationExecution{}).Where("id = ?", x.ID).Updates(map[string]any{"scope_max": scope, "estimated_rows": estimated})

	e.mustProcess(x.ID)

	e.expectStatus(x.ID, models.ExecSuccess)
	if e.sumQty() != 50 || e.qty(51) != 0 {
		t.Errorf("sum qty = %d, qty(51) = %d; rows after the scope must stay untouched", e.sumQty(), e.qty(51))
	}
}

func TestRowsAddedAfterTheRunStartedAreNotProcessed(t *testing.T) {
	e := setup(t)
	e.w.Hooks.AfterCommit = func(id uint, batchNo int) error {
		if batchNo == 1 {
			e.target.Exec(`INSERT INTO items (id) VALUES (500), (501)`)
		}
		return nil
	}
	x := e.newBatchExec(incrementQtyInRange, 10, models.MigrationExecution{})

	e.mustProcess(x.ID)

	e.expectStatus(x.ID, models.ExecSuccess)
	if e.qty(500) != 0 || e.qty(501) != 0 || e.sumQty() != 95 {
		t.Errorf("rows added during the run were processed: sum %d", e.sumQty())
	}
}

func TestABatchRunOnAnEmptyTableSucceedsWithoutBatches(t *testing.T) {
	e := setup(t)
	e.target.Exec(`DELETE FROM items`)
	x := e.newBatchExec(incrementQtyInRange, 10, models.MigrationExecution{})

	e.mustProcess(x.ID)

	got := e.expectStatus(x.ID, models.ExecSuccess)
	if got.CommittedBatches != 0 || len(e.ledger(x.ID)) != 0 {
		t.Errorf("row = %+v", got)
	}
}

func TestTheBatchColumnIsValidatedAgainstTheCatalog(t *testing.T) {
	cases := []struct{ name, table, column, code string }{
		{"text column", "items", "tag", "BATCH_COLUMN_TYPE"},
		{"not unique", "items", "qty", "BATCH_COLUMN_NOT_UNIQUE"},
		{"missing column", "items", "nope", "BATCH_COLUMN_NOT_FOUND"},
		{"missing table", "ghost", "id", "BATCH_COLUMN_NOT_FOUND"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := setup(t)
			x := e.newBatchExec(incrementQtyInRange, 10, models.MigrationExecution{BatchColumn: c.column})
			e.platform.Model(&models.MigrationExecution{}).Where("id = ?", x.ID).Update("batch_table", c.table)

			e.mustProcess(x.ID)

			got := e.expectStatus(x.ID, models.ExecFailed)
			if got.ErrorCode != c.code || e.sumQty() != 0 {
				t.Errorf("code = %s (%s), sum %d, want %s", got.ErrorCode, got.ErrorMessage, e.sumQty(), c.code)
			}
		})
	}
}

func TestAnInjectedTableNameIsQuotedNotExecuted(t *testing.T) {
	e := setup(t)
	x := e.newBatchExec(incrementQtyInRange, 10, models.MigrationExecution{})
	e.platform.Model(&models.MigrationExecution{}).Where("id = ?", x.ID).Update("batch_table", `items; DROP TABLE items`)

	e.mustProcess(x.ID)

	e.expectStatus(x.ID, models.ExecFailed)
	if _, err := e.target.Exec(`SELECT 1 FROM items`); err != nil {
		t.Fatalf("the items table was dropped: %v", err)
	}
}

func TestACrashAfterABatchResumesFromTheNextBatch(t *testing.T) {
	e := setup(t)
	e.w.Hooks.AfterCommit = func(_ uint, batchNo int) error {
		if batchNo == 4 {
			return ErrCrash
		}
		return nil
	}
	x := e.newBatchExec(incrementQtyInRange, 10, models.MigrationExecution{})
	claimed, result, err := e.w.Claim(context.Background(), x.ID)
	if err != nil || result != ClaimOK {
		t.Fatalf("claim = %v, %v", result, err)
	}
	if err := e.w.Run(context.Background(), claimed); !errors.Is(err, ErrCrash) {
		t.Fatalf("run = %v, want the simulated crash", err)
	}

	e.expectStatus(x.ID, models.ExecRunning)
	if !e.lockHeld(e.conn.ID) {
		t.Error("a crashed worker must leave its lock behind, not release it")
	}
	if len(e.ledger(x.ID)) != 4 || e.sumQty() != 40 {
		t.Fatalf("after the crash: ledger %d, sum %d; want 4 batches and 40 rows", len(e.ledger(x.ID)), e.sumQty())
	}

	e.w.Hooks = Hooks{}
	if result, _ := e.process(x.ID); result != ClaimNone {
		t.Fatal("a running execution must not be claimable while its lock is alive")
	}
	e.expireLock(e.conn.ID)
	swept, err := e.w.Sweep(context.Background())
	if err != nil || swept.Dead != 1 || swept.Published != 1 {
		t.Fatalf("sweep = %+v, %v; want one dead execution republished", swept, err)
	}
	e.expectStatus(x.ID, models.ExecUncertain)
	if ids := e.pub.ids(); len(ids) != 1 || ids[0] != x.ID {
		t.Errorf("published = %v", ids)
	}

	e.mustProcess(x.ID)

	got := e.expectStatus(x.ID, models.ExecSuccess)
	sameSpans(t, spans(e.ledger(x.ID)), wantSpans(0, 95, 10))
	if e.sumQty() != 95 {
		t.Errorf("sum qty = %d, want 95: batches 1 to 4 must not run twice", e.sumQty())
	}
	if got.CommittedBatches != 10 || got.CommittedRows != 95 {
		t.Errorf("row = %+v", got)
	}
	attempts := e.attempts(x.ID)
	if len(attempts) != 2 || attempts[0].Status != models.AttemptUncertain || attempts[0].ErrorCode != "WORKER_LOST" || attempts[1].Status != models.AttemptSuccess {
		t.Errorf("attempts = %+v", attempts)
	}
}

func TestATransientFailureInTheMiddleResumesFromTheCheckpoint(t *testing.T) {
	e := setup(t)
	var failed atomic.Bool
	e.w.Hooks.BeforeCommit = func(_ uint, batchNo int) error {
		if batchNo == 5 && failed.CompareAndSwap(false, true) {
			return ErrInjectedTransient
		}
		return nil
	}
	x := e.newBatchExec(incrementQtyInRange, 10, models.MigrationExecution{})

	e.mustProcess(x.ID)

	got := e.expectStatus(x.ID, models.ExecQueued)
	if got.CommittedBatches != 4 || len(e.ledger(x.ID)) != 4 || got.NotBefore == nil {
		t.Fatalf("after the failure: %+v", got)
	}
	e.makeDue(x.ID)
	e.mustProcess(x.ID)

	e.expectStatus(x.ID, models.ExecSuccess)
	sameSpans(t, spans(e.ledger(x.ID)), wantSpans(0, 95, 10))
	if e.sumQty() != 95 {
		t.Errorf("sum qty = %d, want 95", e.sumQty())
	}
	if n := len(e.attempts(x.ID)); n != 2 {
		t.Errorf("attempts = %d, want 2", n)
	}
}

func TestAPermanentErrorInTheMiddleLeavesAPartialExecution(t *testing.T) {
	e := setup(t)
	x := e.newBatchExec(failingFromBatchFive, 10, models.MigrationExecution{})

	e.mustProcess(x.ID)

	got := e.expectStatus(x.ID, models.ExecPartial)
	if got.CommittedBatches != 4 || got.ErrorCode != "22012" || got.Attempts != 1 {
		t.Errorf("row = %+v", got)
	}
	if e.sumQty() != 40 || e.qty(40) != 1 || e.qty(41) != 0 {
		t.Errorf("sum qty = %d; the four committed batches must stay and the failing one must not", e.sumQty())
	}
	if e.lockHeld(e.conn.ID) {
		t.Error("the lock is still held after a partial result")
	}

	stranger := e.newExec(models.MigrationExecution{})
	e.expectHeld(stranger.ID, x.ID)
}

func TestAPartialExecutionCanBeContinuedFromItsCursor(t *testing.T) {
	e := setup(t)
	partial := e.newBatchExec(failingFromBatchFive, 10, models.MigrationExecution{})
	e.mustProcess(partial.ID)
	e.expectStatus(partial.ID, models.ExecPartial)

	continuation := e.newBatchExec(incrementQtyInRange, 10, models.MigrationExecution{Kind: models.ExecKindForward, ContinuesExecutionID: &partial.ID})
	e.mustProcess(continuation.ID)

	got := e.expectStatus(continuation.ID, models.ExecSuccess)
	sameSpans(t, spans(e.ledger(continuation.ID)), wantSpans(40, 95, 10))
	if e.sumQty() != 95 || e.qty(41) != 1 {
		t.Errorf("sum qty = %d: the first four batches must not run again", e.sumQty())
	}
	if got.ScopeMax == nil || *got.ScopeMax != "95" || got.CommittedBatches != 6 {
		t.Errorf("row = %+v", got)
	}

	next := e.newExec(models.MigrationExecution{})
	e.mustProcess(next.ID)
	e.expectStatus(next.ID, models.ExecSuccess)
}

func TestABatchRollbackUndoesTheBatchesInReverseOrder(t *testing.T) {
	e := setup(t)
	run := e.newBatchExec(incrementQtyInRange, 10, models.MigrationExecution{})
	e.mustProcess(run.ID)
	e.expectStatus(run.ID, models.ExecSuccess)

	rollback := e.newBatchExec(`UPDATE items SET qty = qty - 1 WHERE id > $1 AND id <= $2`, 10,
		models.MigrationExecution{Kind: models.ExecKindRollback, RevertsExecutionID: &run.ID})
	e.mustProcess(rollback.ID)

	got := e.expectStatus(rollback.ID, models.ExecSuccess)
	applied := wantSpans(0, 95, 10)
	var reversed [][2]string
	for i := len(applied) - 1; i >= 0; i-- {
		reversed = append(reversed, applied[i])
	}
	sameSpans(t, spans(e.ledger(rollback.ID)), reversed)
	if e.sumQty() != 0 || got.CommittedBatches != 10 {
		t.Errorf("sum qty = %d, row = %+v", e.sumQty(), got)
	}
	e.expectStatus(run.ID, models.ExecSuccess)
}

func TestAFailedRollbackIsPartiallyRevertedAndCanBeContinued(t *testing.T) {
	e := setup(t)
	run := e.newBatchExec(incrementQtyInRange, 10, models.MigrationExecution{})
	e.mustProcess(run.ID)

	failingDown := `UPDATE items SET qty = qty - 1 / CASE WHEN id > 30 AND id <= 40 THEN 0 ELSE 1 END WHERE id > $1 AND id <= $2`
	first := e.newBatchExec(failingDown, 10, models.MigrationExecution{Kind: models.ExecKindRollback, RevertsExecutionID: &run.ID})
	e.mustProcess(first.ID)

	got := e.expectStatus(first.ID, models.ExecPartial)
	if got.CommittedBatches != 6 || e.sumQty() != 40 {
		t.Fatalf("row = %+v, sum qty = %d; want 6 batches undone and 40 rows still applied", got, e.sumQty())
	}

	second := e.newBatchExec(`UPDATE items SET qty = qty - 1 WHERE id > $1 AND id <= $2`, 10,
		models.MigrationExecution{Kind: models.ExecKindRollback, RevertsExecutionID: &run.ID, ContinuesExecutionID: &first.ID})
	e.mustProcess(second.ID)

	e.expectStatus(second.ID, models.ExecSuccess)
	sameSpans(t, spans(e.ledger(second.ID)), [][2]string{{"30", "40"}, {"20", "30"}, {"10", "20"}, {"0", "10"}})
	if e.sumQty() != 0 {
		t.Errorf("sum qty = %d, want 0: every batch must be undone exactly once", e.sumQty())
	}
}

func TestARollbackWithoutATargetFails(t *testing.T) {
	e := setup(t)
	x := e.newBatchExec(incrementQtyInRange, 10, models.MigrationExecution{Kind: models.ExecKindRollback})

	e.mustProcess(x.ID)

	if got := e.expectStatus(x.ID, models.ExecFailed); got.ErrorCode != "ROLLBACK_TARGET_MISSING" {
		t.Errorf("row = %+v", got)
	}
}
