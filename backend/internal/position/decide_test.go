package position

import (
	"errors"
	"fmt"
	"testing"

	"github.com/AthillaZaidan/datanest/backend/internal/apperr"
	"github.com/AthillaZaidan/datanest/backend/internal/models"
)

func code(err error) string {
	var e *apperr.Error
	if errors.As(err, &e) {
		return e.Code
	}
	return ""
}

func message(err error) string {
	var e *apperr.Error
	if errors.As(err, &e) {
		return e.Message
	}
	return ""
}

func describe(p Plan) string {
	return describePlan(p.Kind, p.SQL, idOf(p.Reverts), idOf(p.Continues))
}

func describePlan(kind models.ExecutionKind, sql string, reverts, continues uint) string {
	return fmt.Sprintf("%s %q reverts %d continues %d", kind, sql, reverts, continues)
}

func TestDecide(t *testing.T) {
	const a, b, c, draft = 1, 2, 3, 4
	versions := map[uint]int{a: 1, b: 2, c: 3}
	mig := func(id uint) *models.Migration {
		return &models.Migration{ID: id, SQL: "up", SQLDown: "down"}
	}
	irreversible := &models.Migration{ID: b, SQL: "up", Irreversible: true, IrreversibleReason: "data dihapus"}

	// a and b applied, c rolled back.
	st := Derive([]Event{ev(1, a, run, ok), ev(2, b, run, ok), ev(3, c, run, ok), undo(4, c, 3, ok)})

	tests := []struct {
		name     string
		state    State
		m        *models.Migration
		kind     models.ExecutionKind
		wantCode string
		wantSQL  string
		reverts  uint
	}{
		{"run a draft", st, mig(draft), run, "", "up", 0},
		{"run again is refused", st, mig(a), run, "EXECUTION_STATE_CONFLICT", "", 0},
		{"rollback the top", st, mig(b), rollback, "", "down", 2},
		{"rollback below the top is refused", st, mig(a), rollback, "ROLLBACK_NOT_ALLOWED", "", 0},
		{"rollback with nothing active is refused", Derive(nil), mig(a), rollback, "ROLLBACK_NOT_ALLOWED", "", 0},
		{"rollback an irreversible top is refused", st, irreversible, rollback, "ROLLBACK_NOT_ALLOWED", "", 0},
		{"forward the rolled back migration", st, mig(c), forward, "", "up", 0},
		{"forward an active migration is refused", st, mig(a), forward, "FORWARD_NOT_ALLOWED", "", 0},
		{"forward with nothing to redo is refused", Derive([]Event{ev(1, a, run, ok)}), mig(a), forward, "FORWARD_NOT_ALLOWED", "", 0},
		{"forward a failed attempt",
			Derive([]Event{ev(1, a, run, ok), ev(2, b, run, failed)}), mig(b), forward, "", "up", 0},
		{"forward a rolled back migration after a new run failed",
			Derive([]Event{ev(1, a, run, ok), ev(2, b, run, ok), undo(3, b, 2, ok), ev(4, c, run, failed)}), mig(b), forward, "", "up", 0},
		{"forward the new run that failed",
			Derive([]Event{ev(1, a, run, ok), ev(2, b, run, ok), undo(3, b, 2, ok), ev(4, c, run, failed)}), mig(c), forward, "", "up", 0},
		{"forward a rolled back migration below a failed run first",
			Derive([]Event{ev(1, a, run, ok), ev(2, b, run, failed), undo(3, a, 1, ok)}), mig(a), forward, "", "up", 0},
		{"forward a failed run above a rolled back migration is refused",
			Derive([]Event{ev(1, a, run, ok), ev(2, b, run, failed), undo(3, a, 1, ok)}), mig(b), forward, "FORWARD_NOT_ALLOWED", "", 0},
		{"forward an earlier failed run",
			Derive([]Event{ev(1, a, run, ok), ev(2, b, run, failed), ev(3, c, run, failed)}), mig(b), forward, "", "up", 0},
		{"forward the latest failed run",
			Derive([]Event{ev(1, a, run, ok), ev(2, b, run, failed), ev(3, c, run, failed)}), mig(c), forward, "", "up", 0},
		{"forward an earlier failed run after the latest was forwarded is refused",
			Derive([]Event{ev(1, a, run, ok), ev(2, b, run, failed), ev(3, c, run, failed), ev(4, c, forward, ok)}),
			mig(b), forward, "FORWARD_NOT_ALLOWED", "", 0},
		{"forward a rolled back migration after its partial forward was undone",
			Derive([]Event{ev(1, a, run, ok), undo(2, a, 1, ok), ev(3, b, run, failed), ev(4, a, forward, partial), undo(5, a, 4, ok)}),
			mig(a), forward, "", "up", 0},
		{"forward a failed run after the partial forward of another was undone",
			Derive([]Event{ev(1, a, run, ok), undo(2, a, 1, ok), ev(3, b, run, failed), ev(4, a, forward, partial), undo(5, a, 4, ok)}),
			mig(b), forward, "", "up", 0},
		{"forward of a partial run that was replaced is refused",
			Derive([]Event{ev(1, a, run, partial), ev(2, b, run, ok)}), mig(a), forward, "FORWARD_NOT_ALLOWED", "", 0},
		{"rollback of a partial run that was replaced is refused",
			Derive([]Event{ev(1, a, run, partial), ev(2, b, run, ok)}), mig(a), rollback, "ROLLBACK_NOT_ALLOWED", "", 0},
		{"run of a partial run that was replaced is refused",
			Derive([]Event{ev(1, a, run, partial), ev(2, b, run, ok)}), mig(a), run, "EXECUTION_STATE_CONFLICT", "", 0},
		{"rollback of the run that replaced a partial one",
			Derive([]Event{ev(1, a, run, partial), ev(2, b, run, ok)}), mig(b), rollback, "", "down", 2},
		{"forward skips nothing: lowest version first",
			Derive([]Event{ev(1, a, run, ok), ev(2, b, run, ok), ev(3, c, run, ok), undo(4, c, 3, ok), undo(5, b, 2, ok)}),
			mig(c), forward, "FORWARD_NOT_ALLOWED", "", 0},
		{"forward of a discarded migration is refused",
			Derive([]Event{ev(1, a, run, ok), undo(2, a, 1, ok), ev(3, b, run, ok)}), mig(a), forward, "FORWARD_NOT_ALLOWED", "", 0},
		{"run an unversioned migration whose failed run never started",
			Derive(effective([]Event{ev(1, a, run, ok), ev(2, draft, run, failed)}, versions, 0)), mig(draft), run, "", "up", 0},
		{"run an unversioned migration with a started execution",
			Derive([]Event{ev(1, draft, run, failed)}), mig(draft), run, "", "up", 0},
		{"run a versioned migration whose run failed is refused",
			Derive(effective([]Event{ev(1, a, run, failed)}, versions, 0)), mig(a), run, "EXECUTION_STATE_CONFLICT", "", 0},
		{"a second request while one is open is refused",
			Derive([]Event{ev(1, a, run, ok), undo(2, a, 1, models.ExecPendingReview)}), mig(a), rollback, "EXECUTION_STATE_CONFLICT", "", 0},
		{"run a reversible draft without down SQL is refused", st, &models.Migration{ID: draft, SQL: "up"}, run, "VALIDATION_ERROR", "", 0},
		{"run an irreversible draft without a reason is refused", st, &models.Migration{ID: draft, SQL: "up", Irreversible: true}, run, "VALIDATION_ERROR", "", 0},
		{"run an irreversible draft with a reason",
			st, &models.Migration{ID: draft, SQL: "up", Irreversible: true, IrreversibleReason: "r"}, run, "", "up", 0},
		{"rollback without down SQL is refused", st, &models.Migration{ID: b, SQL: "up"}, rollback, "ROLLBACK_NOT_ALLOWED", "", 0},
		{"unknown kind", st, mig(a), "drop", "VALIDATION_ERROR", "", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := Decide(tt.state, versions, tt.m, tt.kind)
			if got := code(err); got != tt.wantCode {
				t.Fatalf("code = %q (%v), want %q", got, err, tt.wantCode)
			}
			if err != nil {
				return
			}
			if got, want := describe(p), describePlan(tt.kind, tt.wantSQL, tt.reverts, 0); got != want {
				t.Errorf("plan = %s, want %s", got, want)
			}
		})
	}
}

func TestDecideWhileAnExecutionIsPartial(t *testing.T) {
	const a, b, draft = 1, 2, 3
	versions := map[uint]int{a: 1, b: 2}
	mig := func(id uint) *models.Migration {
		return &models.Migration{ID: id, SQL: "up", SQLDown: "down"}
	}
	irreversible := &models.Migration{ID: b, SQL: "up", Irreversible: true, IrreversibleReason: "data dihapus"}
	noDown := &models.Migration{ID: b, SQL: "up"}

	partialRun := Derive([]Event{ev(1, a, run, ok), ev(2, b, run, partial)})
	partialForward := Derive([]Event{ev(1, a, run, ok), ev(2, b, run, partial), resume(3, b, 2, partial)})
	partialRollback := Derive([]Event{ev(1, a, run, ok), ev(2, b, run, ok), undo(3, b, 2, partial)})
	partialUndo := Derive([]Event{ev(1, a, run, ok), ev(2, b, run, partial), undo(3, b, 2, partial)})
	partialWithOpen := Derive([]Event{ev(1, a, run, ok), ev(2, b, run, partial), resume(3, b, 2, models.ExecPendingReview)})
	redoBelow := Derive([]Event{ev(1, a, run, ok), undo(2, a, 1, ok), ev(3, b, forward, partial)})

	const (
		continueOrUndo = "Migration ini baru sebagian dijalankan. Lanjutkan dengan forward atau batalkan dengan rollback."
		finishRollback = "Rollback migration ini baru sebagian. Lanjutkan rollback-nya dulu."
		otherByTwo     = "Database ini masih punya migration yang baru sebagian (eksekusi E2). Lanjutkan atau batalkan dulu."
		otherByThree   = "Database ini masih punya migration yang baru sebagian (eksekusi E3). Lanjutkan atau batalkan dulu."
	)
	tests := []struct {
		name      string
		state     State
		m         *models.Migration
		kind      models.ExecutionKind
		wantCode  string
		wantMsg   string
		wantSQL   string
		reverts   uint
		continues uint
	}{
		{"forward continues a partial run", partialRun, mig(b), forward, "", "", "up", 0, 2},
		{"forward continues a partial forward", partialForward, mig(b), forward, "", "", "up", 0, 3},
		{"forward continues an irreversible partial run", partialRun, irreversible, forward, "", "", "up", 0, 2},
		{"rollback undoes a partial run", partialRun, mig(b), rollback, "", "", "down", 2, 0},
		{"rollback undoes a partial forward", partialForward, mig(b), rollback, "", "", "down", 3, 0},
		{"rollback of an irreversible partial run is refused", partialRun, irreversible, rollback, "ROLLBACK_NOT_ALLOWED",
			"Migration ini irreversible dan tidak dapat di-rollback. Alasan: data dihapus", "", 0, 0},
		{"rollback of a partial run without down SQL is refused", partialRun, noDown, rollback, "ROLLBACK_NOT_ALLOWED",
			"Migration ini tidak punya SQL down.", "", 0, 0},
		{"run of a partial run is refused", partialRun, mig(b), run, "EXECUTION_STATE_CONFLICT", continueOrUndo, "", 0, 0},
		{"run of a partial forward is refused", partialForward, mig(b), run, "EXECUTION_STATE_CONFLICT", continueOrUndo, "", 0, 0},
		{"rollback continues a partial rollback", partialRollback, mig(b), rollback, "", "", "down", 2, 3},
		{"rollback continues a partial undo of a partial run", partialUndo, mig(b), rollback, "", "", "down", 2, 3},
		{"run of a partial rollback is refused", partialRollback, mig(b), run, "EXECUTION_STATE_CONFLICT", finishRollback, "", 0, 0},
		{"forward of a partial rollback is refused", partialRollback, mig(b), forward, "EXECUTION_STATE_CONFLICT", finishRollback, "", 0, 0},
		{"unknown kind on a partial run", partialRun, mig(b), "drop", "VALIDATION_ERROR", "", "", 0, 0},
		{"unknown kind on a partial rollback", partialRollback, mig(b), "drop", "VALIDATION_ERROR", "", "", 0, 0},
		{"a second request for the partial migration while one is open is refused", partialWithOpen, mig(b), rollback,
			"EXECUTION_STATE_CONFLICT", "Migration ini masih punya eksekusi yang belum selesai.", "", 0, 0},

		{"run of another migration is accepted", partialRun, mig(draft), run, "", "", "up", 0, 0},
		{"run of another migration still follows the run rules", partialRun, mig(a), run, "EXECUTION_STATE_CONFLICT",
			"Migration ini sudah pernah dijalankan. Gunakan forward untuk menjalankannya lagi.", "", 0, 0},
		{"forward of another migration is refused", redoBelow, mig(a), forward, "EXECUTION_STATE_CONFLICT", otherByThree, "", 0, 0},
		{"rollback of another migration is refused", partialRun, mig(a), rollback, "EXECUTION_STATE_CONFLICT", otherByTwo, "", 0, 0},
		{"unknown kind on another migration", partialRun, mig(a), "drop", "VALIDATION_ERROR", "", "", 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := Decide(tt.state, versions, tt.m, tt.kind)
			if got := code(err); got != tt.wantCode {
				t.Fatalf("code = %q (%v), want %q", got, err, tt.wantCode)
			}
			if err != nil {
				if got := message(err); tt.wantMsg != "" && got != tt.wantMsg {
					t.Errorf("message = %q, want %q", got, tt.wantMsg)
				}
				return
			}
			if got, want := describe(p), describePlan(tt.kind, tt.wantSQL, tt.reverts, tt.continues); got != want {
				t.Errorf("plan = %s, want %s", got, want)
			}
		})
	}
}

func TestForwardRefusalNamesWhatMayRun(t *testing.T) {
	const a, b, c = 1, 2, 3
	versions := map[uint]int{a: 1, b: 2, c: 3}
	tests := []struct {
		name   string
		events []Event
		want   string
	}{
		{"the lowest rolled back migration first",
			[]Event{ev(1, a, run, ok), ev(2, b, run, ok), undo(3, b, 2, ok), undo(4, a, 1, ok), ev(5, c, run, failed)},
			"Forward mengikuti urutan linear: jalankan maju v1 dahulu."},
		{"only the failed run", []Event{ev(1, a, run, ok), ev(2, b, run, ok), ev(3, c, run, failed)},
			"Hanya v3 yang dapat dijalankan maju."},
		{"nothing", []Event{ev(1, a, run, ok), ev(2, b, run, ok)}, "Tidak ada migration yang dapat dijalankan maju."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Decide(Derive(tt.events), versions, &models.Migration{ID: b, SQL: "up"}, forward)
			if code(err) != "FORWARD_NOT_ALLOWED" || message(err) != tt.want {
				t.Errorf("err = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestForwardable(t *testing.T) {
	const a, b, c = 1, 2, 3
	versions := map[uint]int{a: 1, b: 2, c: 3}
	st := Derive([]Event{ev(1, a, run, ok), ev(2, b, run, ok), undo(3, b, 2, ok), undo(4, a, 1, ok), ev(5, c, run, failed)})
	for id, want := range map[uint]bool{a: true, b: false, c: true, 4: false, 0: false} {
		if got := Forwardable(st, versions, id); got != want {
			t.Errorf("forwardable(%d) = %v, want %v", id, got, want)
		}
	}
	if Forwardable(Derive([]Event{ev(1, 4, run, failed)}), versions, 4) {
		t.Error("a failed run without a version is forwardable")
	}
	if Forwardable(Derive(nil), versions, 0) {
		t.Error("nothing is forwardable without a failed run")
	}
}

func TestIrreversibleRefusalNamesTheReason(t *testing.T) {
	st := Derive([]Event{ev(1, 1, run, ok)})
	_, err := Decide(st, map[uint]int{1: 1}, &models.Migration{ID: 1, Irreversible: true, IrreversibleReason: "kolom dihapus"}, rollback)
	var e *apperr.Error
	if !errors.As(err, &e) || e.Message != "Migration ini irreversible dan tidak dapat di-rollback. Alasan: kolom dihapus" {
		t.Errorf("err = %v", err)
	}
}

func TestActiveVersion(t *testing.T) {
	versions := map[uint]int{1: 1, 2: 3}
	if v := ActiveVersion(Derive(nil), versions); v != 0 {
		t.Errorf("empty = %d", v)
	}
	if v := ActiveVersion(Derive([]Event{ev(1, 1, run, ok), ev(2, 2, run, ok)}), versions); v != 3 {
		t.Errorf("versions may skip: got %d, want 3", v)
	}
	if v := ActiveVersion(Derive([]Event{ev(1, 1, run, ok), undo(2, 1, 1, ok)}), versions); v != 0 {
		t.Errorf("after rolling back the first migration = %d, want 0", v)
	}
}
