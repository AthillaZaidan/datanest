package position

import (
	"reflect"
	"sort"
	"testing"

	"github.com/AthillaZaidan/datanest/backend/internal/models"
)

const (
	run      = models.ExecKindRun
	forward  = models.ExecKindForward
	rollback = models.ExecKindRollback
	ok       = models.ExecSuccess
	failed   = models.ExecFailed
)

func ev(id, migration uint, kind models.ExecutionKind, status models.ExecutionStatus) Event {
	return Event{ID: id, MigrationID: migration, Kind: kind, Status: status}
}

func undo(id, migration, reverts uint, status models.ExecutionStatus) Event {
	return Event{ID: id, MigrationID: migration, Kind: rollback, Status: status, Reverts: &reverts}
}

func activeIDs(s State) []uint {
	out := []uint{}
	for _, a := range s.Active {
		out = append(out, a.MigrationID)
	}
	return out
}

func redoIDs(s State) []uint {
	out := []uint{}
	for m := range s.Redo {
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func TestDerive(t *testing.T) {
	const a, b, c = 1, 2, 3
	tests := []struct {
		name   string
		events []Event
		active []uint
		redo   []uint
	}{
		{"nothing executed", nil, []uint{}, []uint{}},
		{"runs stack up", []Event{ev(1, a, run, ok), ev(2, b, run, ok)}, []uint{a, b}, []uint{}},
		{"rolling back the first migration leaves nothing active",
			[]Event{ev(1, a, run, ok), undo(2, a, 1, ok)}, []uint{}, []uint{a}},
		{"failed rollback changes nothing",
			[]Event{ev(1, a, run, ok), undo(2, a, 1, failed)}, []uint{a}, []uint{}},
		{"failed run is not active but can be forwarded",
			[]Event{ev(1, a, run, ok), ev(2, b, run, failed)}, []uint{a}, []uint{b}},
		{"forward re-applies a rolled back migration",
			[]Event{ev(1, a, run, ok), ev(2, b, run, ok), undo(3, b, 2, ok), ev(4, b, forward, ok)}, []uint{a, b}, []uint{}},
		{"design example: A, B, B rollback, B forward, C",
			[]Event{ev(1, a, run, ok), ev(2, b, run, ok), undo(3, b, 2, ok), ev(4, b, forward, ok), ev(5, c, run, ok)},
			[]uint{a, b, c}, []uint{}},
		{"a new run discards rolled back migrations",
			[]Event{ev(1, a, run, ok), ev(2, b, run, ok), undo(3, b, 2, ok), ev(4, c, run, ok)}, []uint{a, c}, []uint{}},
		{"a new run discards failed attempts",
			[]Event{ev(1, a, run, failed), ev(2, b, run, ok)}, []uint{b}, []uint{}},
		{"a new run that fails still discards, and is itself forwardable",
			[]Event{ev(1, a, run, ok), ev(2, b, run, ok), undo(3, b, 2, ok), ev(4, c, run, failed)}, []uint{a}, []uint{c}},
		{"forward keeps the rest of the redo history",
			[]Event{ev(1, a, run, ok), ev(2, b, run, ok), ev(3, c, run, ok), undo(4, c, 3, ok), undo(5, b, 2, ok), ev(6, b, forward, ok)},
			[]uint{a, b}, []uint{c}},
		{"failed forward stays forwardable",
			[]Event{ev(1, a, run, ok), undo(2, a, 1, ok), ev(3, a, forward, failed)}, []uint{}, []uint{a}},
		{"requests that never started are ignored",
			[]Event{ev(1, a, run, ok), ev(2, b, run, models.ExecPendingReview), ev(3, b, run, models.ExecRejected)}, []uint{a}, []uint{}},
		{"a rollback that does not revert the top is ignored",
			[]Event{ev(1, a, run, ok), ev(2, b, run, ok), undo(3, a, 1, ok)}, []uint{a, b}, []uint{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := Derive(tt.events)
			if got := activeIDs(s); !reflect.DeepEqual(got, tt.active) {
				t.Errorf("active = %v, want %v", got, tt.active)
			}
			if got := redoIDs(s); !reflect.DeepEqual(got, tt.redo) {
				t.Errorf("redo = %v, want %v", got, tt.redo)
			}
		})
	}
}

func TestDeriveTracksStartedAndTop(t *testing.T) {
	s := Derive([]Event{ev(1, 1, run, ok), ev(2, 2, run, failed), ev(3, 3, run, models.ExecQueued)})
	if !s.Started[1] || !s.Started[2] || s.Started[3] {
		t.Errorf("started = %v", s.Started)
	}
	if top, ok := s.Top(); !ok || top.MigrationID != 1 || top.ExecutionID != 1 {
		t.Errorf("top = %+v %v", top, ok)
	}
	if !s.IsActive(1) || s.IsActive(2) {
		t.Error("IsActive mismatch")
	}
	if _, ok := Derive(nil).Top(); ok {
		t.Error("empty state has a top")
	}
}
