package position

import (
	"errors"
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
		{"forward skips nothing: lowest version first",
			Derive([]Event{ev(1, a, run, ok), ev(2, b, run, ok), ev(3, c, run, ok), undo(4, c, 3, ok), undo(5, b, 2, ok)}),
			mig(c), forward, "FORWARD_NOT_ALLOWED", "", 0},
		{"forward of a discarded migration is refused",
			Derive([]Event{ev(1, a, run, ok), undo(2, a, 1, ok), ev(3, b, run, ok)}), mig(a), forward, "FORWARD_NOT_ALLOWED", "", 0},
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
			if p.Kind != tt.kind || p.SQL != tt.wantSQL {
				t.Errorf("plan = %+v, want kind %s sql %q", p, tt.kind, tt.wantSQL)
			}
			if (p.Reverts == nil) != (tt.reverts == 0) || (p.Reverts != nil && *p.Reverts != tt.reverts) {
				t.Errorf("reverts = %v, want %d", p.Reverts, tt.reverts)
			}
		})
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
