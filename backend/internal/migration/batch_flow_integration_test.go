//go:build integration

package migration_test

import (
	"strings"
	"testing"

	"github.com/AthillaZaidan/datanest/backend/internal/migration"
	"github.com/AthillaZaidan/datanest/backend/internal/models"
	"github.com/AthillaZaidan/datanest/backend/internal/worker"
)

const (
	failingUp = `UPDATE items SET qty = qty + 1 / (CASE WHEN id > 40 AND (SELECT on_ FROM switch) THEN 0 ELSE 1 END) WHERE id > $1 AND id <= $2`
	failDown  = `UPDATE items SET qty = qty - 1 / (CASE WHEN id <= 40 AND (SELECT on_ FROM switch) THEN 0 ELSE 1 END) WHERE id > $1 AND id <= $2`
)

type batchFlow struct{ *flow }

func setupBatchFlow(t *testing.T) batchFlow {
	t.Helper()
	f := batchFlow{setupFlow(t)}
	if _, err := f.target.Exec(`CREATE TABLE items (id int PRIMARY KEY, qty int NOT NULL DEFAULT 0);
		INSERT INTO items (id) SELECT g FROM generate_series(1, 95) g;
		CREATE TABLE switch (on_ boolean NOT NULL); INSERT INTO switch VALUES (true)`); err != nil {
		t.Fatal(err)
	}
	return f
}

func (f batchFlow) batchRecipe(r migration.Recipe) uint {
	f.t.Helper()
	r.Mode, r.BatchTable, r.BatchColumn = models.ExecModeBatch, "items", "id"
	v, err := f.svc.Create(f.ctx, f.editor.ID, f.dbID, r)
	if err != nil {
		f.t.Fatal(err)
	}
	return v.ID
}

func (f batchFlow) submit(id uint, kind models.ExecutionKind, size int) models.MigrationExecution {
	f.t.Helper()
	var batchSize *int
	if size > 0 {
		batchSize = &size
	}
	e, err := f.svc.Submit(f.ctx, f.editor.ID, id, kind, batchSize)
	if err != nil {
		f.t.Fatalf("submit %s: %v", kind, err)
	}
	return *e
}

func (f batchFlow) refused(id uint, kind models.ExecutionKind, size int, want string) {
	f.t.Helper()
	var batchSize *int
	if size > 0 {
		batchSize = &size
	}
	if _, err := f.svc.Submit(f.ctx, f.editor.ID, id, kind, batchSize); errCode(err) != want {
		f.t.Errorf("%s of migration %d = %v, want %s", kind, id, err, want)
	}
}

func (f batchFlow) expect(e models.MigrationExecution, status models.ExecutionStatus, batches int, rows int64) {
	f.t.Helper()
	if e.Status != status || e.CommittedBatches != batches || e.CommittedRows != rows {
		f.t.Fatalf("%s E%d = %s, %d batches, %d rows (%s %s), want %s, %d, %d",
			e.Kind, e.ID, e.Status, e.CommittedBatches, e.CommittedRows, e.ErrorCode, e.ErrorMessage, status, batches, rows)
	}
}

func (f batchFlow) expectQty(low, high int) {
	f.t.Helper()
	var below, above, wrong int
	f.target.QueryRow(`SELECT count(*) FILTER (WHERE id <= 40 AND qty <> $1), count(*) FILTER (WHERE id > 40 AND qty <> $2), count(*) FROM items`, low, high).
		Scan(&below, &above, &wrong)
	if below != 0 || above != 0 || wrong != 95 {
		f.t.Fatalf("%d of the first 40 rows are not %d, %d of the others are not %d (of %d rows)", below, low, above, high, wrong)
	}
}

func (f batchFlow) turn(on bool) {
	f.t.Helper()
	if _, err := f.target.Exec(`UPDATE switch SET on_ = $1`, on); err != nil {
		f.t.Fatal(err)
	}
}

func (f batchFlow) held(e models.MigrationExecution) {
	f.t.Helper()
	f.setStatus(e, models.ExecQueued)
	if _, result, err := f.w.Claim(f.ctx, e.ID); err != nil || result != worker.ClaimHeld {
		f.t.Fatalf("claim of E%d = %v, %v, want held", e.ID, result, err)
	}
	f.platform.Model(&e).Update("not_before", nil)
}

func TestABatchRunIsContinuedAfterItEndedPartial(t *testing.T) {
	f := setupBatchFlow(t)
	below := f.recipe("CREATE TABLE below (id int)", "DROP TABLE below")
	f.do(below, models.ExecKindRun)
	id := f.batchRecipe(migration.Recipe{SQLUp: failingUp, SQLDown: `UPDATE items SET qty = qty - 1 WHERE id > $1 AND id <= $2`})
	other := f.recipe("CREATE TABLE other (id int)", "DROP TABLE other")

	partial := f.run(f.submit(id, models.ExecKindRun, 10))
	f.expect(partial, models.ExecPartial, 4, 40)
	f.expectQty(1, 0)
	if f.current() != 1 {
		t.Errorf("a partial run moved the active version to %d", f.current())
	}

	waiting := f.submit(other, models.ExecKindRun, 0)
	f.held(waiting)
	f.refused(below, models.ExecKindRollback, 0, "EXECUTION_STATE_CONFLICT")
	f.refused(id, models.ExecKindRun, 10, "EXECUTION_STATE_CONFLICT")
	f.refused(id, models.ExecKindForward, 0, "VALIDATION_ERROR")

	f.turn(false)
	done := f.run(f.submit(id, models.ExecKindForward, 20))
	f.expect(done, models.ExecSuccess, 3, 55)
	if done.ContinuesExecutionID == nil || *done.ContinuesExecutionID != partial.ID {
		t.Errorf("continues = %v, want E%d", valueOf(done.ContinuesExecutionID), partial.ID)
	}
	f.expectQty(1, 1)
	if f.current() != 2 {
		t.Errorf("active version = %d, want 2", f.current())
	}

	f.expect(f.run(waiting), models.ExecSuccess, 1, 0)
	if f.current() != 3 {
		t.Errorf("the held migration got version %d, want 3", f.current())
	}
	f.do(other, models.ExecKindRollback)

	undone := f.run(f.submit(id, models.ExecKindRollback, 0))
	f.expect(undone, models.ExecSuccess, 7, 95)
	f.expectQty(0, 0)
	if f.current() != 1 {
		t.Errorf("after the undo the active version is %d, want 1", f.current())
	}
	f.expect(f.run(f.submit(id, models.ExecKindForward, 50)), models.ExecSuccess, 2, 95)
	f.expectQty(1, 1)
}

func TestAPartialBatchRunIsUndone(t *testing.T) {
	f := setupBatchFlow(t)
	id := f.batchRecipe(migration.Recipe{SQLUp: failingUp, SQLDown: `UPDATE items SET qty = qty - 1 WHERE id > $1 AND id <= $2`})
	partial := f.run(f.submit(id, models.ExecKindRun, 10))
	f.expect(partial, models.ExecPartial, 4, 40)

	undo := f.run(f.submit(id, models.ExecKindRollback, 0))
	f.expect(undo, models.ExecSuccess, 4, 40)
	if undo.RevertsExecutionID == nil || *undo.RevertsExecutionID != partial.ID {
		t.Errorf("reverts = %v, want E%d", valueOf(undo.RevertsExecutionID), partial.ID)
	}
	f.expectQty(0, 0)

	f.turn(false)
	f.expect(f.run(f.submit(id, models.ExecKindForward, 10)), models.ExecSuccess, 10, 95)
	f.expectQty(1, 1)
}

func TestAPartialBatchRollbackIsContinued(t *testing.T) {
	f := setupBatchFlow(t)
	f.turn(false)
	id := f.batchRecipe(migration.Recipe{SQLUp: `UPDATE items SET qty = qty + 1 WHERE id > $1 AND id <= $2`, SQLDown: failDown})
	f.expect(f.run(f.submit(id, models.ExecKindRun, 10)), models.ExecSuccess, 10, 95)

	f.turn(true)
	first := f.run(f.submit(id, models.ExecKindRollback, 0))
	f.expect(first, models.ExecPartial, 6, 55)
	f.expectQty(1, 0)
	f.refused(id, models.ExecKindForward, 10, "EXECUTION_STATE_CONFLICT")
	f.refused(id, models.ExecKindRun, 10, "EXECUTION_STATE_CONFLICT")

	f.turn(false)
	second := f.run(f.submit(id, models.ExecKindRollback, 0))
	f.expect(second, models.ExecSuccess, 4, 40)
	if second.ContinuesExecutionID == nil || *second.ContinuesExecutionID != first.ID {
		t.Errorf("continues = %v, want E%d", valueOf(second.ContinuesExecutionID), first.ID)
	}
	f.expectQty(0, 0)
	if f.current() != 0 {
		t.Errorf("active version = %d, want 0", f.current())
	}
}

func TestAnIrreversiblePartialIsReplacedByACorrectiveRun(t *testing.T) {
	f := setupBatchFlow(t)
	id := f.batchRecipe(migration.Recipe{SQLUp: failingUp, Irreversible: true, IrreversibleReason: "nilai lama hilang"})
	waiting := f.submit(f.recipe("CREATE TABLE early (id int)", "DROP TABLE early"), models.ExecKindRun, 0)
	f.expect(f.run(f.submit(id, models.ExecKindRun, 10)), models.ExecPartial, 4, 40)

	f.refused(id, models.ExecKindRollback, 0, "ROLLBACK_NOT_ALLOWED")
	f.held(waiting)
	corrective := f.recipe("UPDATE items SET qty = 1", "SELECT 1")
	f.expect(f.run(f.submit(corrective, models.ExecKindRun, 0)), models.ExecSuccess, 1, 95)
	f.expectQty(1, 1)
	f.refused(id, models.ExecKindForward, 10, "FORWARD_NOT_ALLOWED")
	f.expect(f.run(waiting), models.ExecSuccess, 1, 0)
}

func TestABatchRunOfAMissingTableIsRefusedWithItsReason(t *testing.T) {
	f := setupBatchFlow(t)
	id := f.batchRecipe(migration.Recipe{SQLUp: failingUp, SQLDown: failDown})
	if _, err := f.target.Exec(`DROP TABLE items`); err != nil {
		t.Fatal(err)
	}
	size := 10
	_, err := f.svc.Submit(f.ctx, f.editor.ID, id, models.ExecKindRun, &size)
	if errCode(err) != "VALIDATION_ERROR" || !strings.Contains(err.Error(), "tidak ditemukan") {
		t.Errorf("submit = %v", err)
	}
}
