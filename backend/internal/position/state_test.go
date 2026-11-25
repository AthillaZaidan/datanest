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
	partial  = models.ExecPartial
)

func ev(id, migration uint, kind models.ExecutionKind, status models.ExecutionStatus) Event {
	return Event{ID: id, MigrationID: migration, Kind: kind, Status: status}
}

func undo(id, migration, reverts uint, status models.ExecutionStatus) Event {
	return Event{ID: id, MigrationID: migration, Kind: rollback, Status: status, Reverts: &reverts}
}

func resume(id, migration, continues uint, status models.ExecutionStatus) Event {
	return Event{ID: id, MigrationID: migration, Kind: forward, Status: status, Continues: &continues}
}

func resumeUndo(id, migration, reverts, continues uint, status models.ExecutionStatus) Event {
	return Event{ID: id, MigrationID: migration, Kind: rollback, Status: status, Reverts: &reverts, Continues: &continues}
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

func partialID(s State) uint {
	if s.Partial == nil {
		return 0
	}
	return s.Partial.ExecutionID
}

func TestDerive(t *testing.T) {
	const a, b, c = 1, 2, 3
	tests := []struct {
		name    string
		events  []Event
		active  []uint
		redo    []uint
		retry   uint
		partial uint
	}{
		{"nothing executed", nil, []uint{}, []uint{}, 0, 0},
		{"runs stack up", []Event{ev(1, a, run, ok), ev(2, b, run, ok)}, []uint{a, b}, []uint{}, 0, 0},
		{"rolling back the first migration leaves nothing active",
			[]Event{ev(1, a, run, ok), undo(2, a, 1, ok)}, []uint{}, []uint{a}, 0, 0},
		{"failed rollback changes nothing",
			[]Event{ev(1, a, run, ok), undo(2, a, 1, failed)}, []uint{a}, []uint{}, 0, 0},
		{"failed run is not active but can be forwarded",
			[]Event{ev(1, a, run, ok), ev(2, b, run, failed)}, []uint{a}, []uint{}, b, 0},
		{"forward re-applies a rolled back migration",
			[]Event{ev(1, a, run, ok), ev(2, b, run, ok), undo(3, b, 2, ok), ev(4, b, forward, ok)}, []uint{a, b}, []uint{}, 0, 0},
		{"design example: A, B, B rollback, B forward, C",
			[]Event{ev(1, a, run, ok), ev(2, b, run, ok), undo(3, b, 2, ok), ev(4, b, forward, ok), ev(5, c, run, ok)},
			[]uint{a, b, c}, []uint{}, 0, 0},
		{"a new run discards rolled back migrations",
			[]Event{ev(1, a, run, ok), ev(2, b, run, ok), undo(3, b, 2, ok), ev(4, c, run, ok)}, []uint{a, c}, []uint{}, 0, 0},
		{"a new run discards failed attempts",
			[]Event{ev(1, a, run, failed), ev(2, b, run, ok)}, []uint{b}, []uint{}, 0, 0},
		{"a new run that fails keeps the rolled back migration and is itself forwardable",
			[]Event{ev(1, a, run, ok), ev(2, b, run, ok), undo(3, b, 2, ok), ev(4, c, run, failed)}, []uint{a}, []uint{b}, c, 0},
		{"a new run that fails takes the place of an earlier failed one",
			[]Event{ev(1, a, run, failed), ev(2, b, run, failed)}, []uint{}, []uint{}, b, 0},
		{"rolling back two migrations keeps both in the redo history",
			[]Event{ev(1, a, run, ok), ev(2, b, run, ok), ev(3, c, run, ok), undo(4, c, 3, ok), undo(5, b, 2, ok)},
			[]uint{a}, []uint{b, c}, 0, 0},
		{"forward keeps the rest of the redo history",
			[]Event{ev(1, a, run, ok), ev(2, b, run, ok), ev(3, c, run, ok), undo(4, c, 3, ok), undo(5, b, 2, ok), ev(6, b, forward, ok)},
			[]uint{a, b}, []uint{c}, 0, 0},
		{"failed forward stays forwardable",
			[]Event{ev(1, a, run, ok), undo(2, a, 1, ok), ev(3, a, forward, failed)}, []uint{}, []uint{a}, 0, 0},
		{"failed forward of the failed run stays forwardable",
			[]Event{ev(1, a, run, ok), ev(2, b, run, failed), ev(3, b, forward, failed)}, []uint{a}, []uint{}, b, 0},
		{"forward of the failed run clears the redo history",
			[]Event{ev(1, a, run, ok), undo(2, a, 1, ok), ev(3, b, run, failed), ev(4, b, forward, ok)}, []uint{b}, []uint{}, 0, 0},
		{"forward of a rolled back migration keeps the failed run forwardable",
			[]Event{ev(1, a, run, ok), undo(2, a, 1, ok), ev(3, b, run, failed), ev(4, a, forward, ok)}, []uint{a}, []uint{}, b, 0},
		{"rolling back the migration below a failed run puts both in the redo history",
			[]Event{ev(1, a, run, ok), ev(2, b, run, failed), undo(3, a, 1, ok)}, []uint{}, []uint{a, b}, 0, 0},
		{"a run that is still running or uncertain discards nothing",
			[]Event{ev(1, a, run, ok), undo(2, a, 1, ok), ev(3, b, run, failed), ev(4, c, run, models.ExecRunning), ev(5, c, run, models.ExecUncertain)},
			[]uint{}, []uint{a}, b, 0},
		{"requests that never started are ignored",
			[]Event{ev(1, a, run, ok), ev(2, b, run, models.ExecPendingReview), ev(3, b, run, models.ExecRejected)}, []uint{a}, []uint{}, 0, 0},
		{"a rollback that does not revert the top is ignored",
			[]Event{ev(1, a, run, ok), ev(2, b, run, ok), undo(3, a, 1, ok)}, []uint{a, b}, []uint{}, 0, 0},

		{"a partial run is not active and discards the redo history",
			[]Event{ev(1, a, run, ok), undo(2, a, 1, ok), ev(3, b, run, partial)}, []uint{}, []uint{}, 0, 3},
		{"a partial run discards an earlier failed run",
			[]Event{ev(1, a, run, failed), ev(2, b, run, partial)}, []uint{}, []uint{}, 0, 2},
		{"a partial run that is continued is active",
			[]Event{ev(1, a, run, ok), ev(2, b, run, partial), resume(3, b, 2, ok)}, []uint{a, b}, []uint{}, 0, 0},
		{"a partial run that is undone can be redone",
			[]Event{ev(1, a, run, ok), ev(2, b, run, partial), undo(3, b, 2, ok)}, []uint{a}, []uint{b}, 0, 0},
		{"a partial continuation takes the place of the one it continues",
			[]Event{ev(1, a, run, partial), resume(2, a, 1, partial)}, []uint{}, []uint{}, 0, 2},
		{"a partial continuation that is continued is active",
			[]Event{ev(1, a, run, partial), resume(2, a, 1, partial), resume(3, a, 2, ok)}, []uint{a}, []uint{}, 0, 0},
		{"a partial continuation that is undone can be redone",
			[]Event{ev(1, a, run, partial), resume(2, a, 1, partial), undo(3, a, 2, ok)}, []uint{}, []uint{a}, 0, 0},
		{"a failed continuation leaves the original partial in place",
			[]Event{ev(1, a, run, partial), resume(2, a, 1, failed)}, []uint{}, []uint{}, 0, 1},
		{"a failed undo leaves the original partial in place",
			[]Event{ev(1, a, run, partial), undo(2, a, 1, failed)}, []uint{}, []uint{}, 0, 1},
		{"a partial rollback keeps the migration active",
			[]Event{ev(1, a, run, ok), undo(2, a, 1, partial)}, []uint{a}, []uint{}, 0, 2},
		{"a partial rollback that is continued can be redone",
			[]Event{ev(1, a, run, ok), undo(2, a, 1, partial), resumeUndo(3, a, 1, 2, ok)}, []uint{}, []uint{a}, 0, 0},
		{"a failed continuation of a partial rollback leaves it in place",
			[]Event{ev(1, a, run, ok), undo(2, a, 1, partial), resumeUndo(3, a, 1, 2, failed)}, []uint{a}, []uint{}, 0, 2},
		{"undoing a partial run can itself be partial",
			[]Event{ev(1, a, run, partial), undo(2, a, 1, partial)}, []uint{}, []uint{}, 0, 2},
		{"a partial undo of a partial run that is continued can be redone",
			[]Event{ev(1, a, run, partial), undo(2, a, 1, partial), resumeUndo(3, a, 1, 2, ok)}, []uint{}, []uint{a}, 0, 0},
		{"a partial forward keeps the redo history until it is continued",
			[]Event{ev(1, a, run, ok), undo(2, a, 1, ok), ev(3, a, forward, partial)}, []uint{}, []uint{a}, 0, 3},
		{"a partial forward that is continued leaves the redo history",
			[]Event{ev(1, a, run, ok), undo(2, a, 1, ok), ev(3, a, forward, partial), resume(4, a, 3, ok)}, []uint{a}, []uint{}, 0, 0},
		{"a partial forward of the failed run that is continued clears the redo history",
			[]Event{ev(1, a, run, ok), undo(2, a, 1, ok), ev(3, b, run, failed), ev(4, b, forward, partial), resume(5, b, 4, ok)},
			[]uint{b}, []uint{}, 0, 0},
		{"a partial forward of the failed run that is undone puts it in the redo history",
			[]Event{ev(1, a, run, ok), undo(2, a, 1, ok), ev(3, b, run, failed), ev(4, b, forward, partial), undo(5, b, 4, ok)},
			[]uint{}, []uint{a, b}, 0, 0},
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
			if s.Retry != tt.retry {
				t.Errorf("retry = %d, want %d", s.Retry, tt.retry)
			}
			if got := partialID(s); got != tt.partial {
				t.Errorf("partial = %d, want %d", got, tt.partial)
			}
		})
	}
}

func TestDeriveDescribesThePartialExecution(t *testing.T) {
	s := Derive([]Event{ev(1, 7, run, ok), undo(2, 7, 1, partial)})
	if p := s.Partial; p == nil || p.ExecutionID != 2 || p.MigrationID != 7 || p.Kind != rollback || p.Reverts == nil || *p.Reverts != 1 {
		t.Errorf("partial = %+v, want the rollback of execution 1", s.Partial)
	}
	s = Derive([]Event{ev(1, 7, run, partial)})
	if p := s.Partial; p == nil || p.ExecutionID != 1 || p.MigrationID != 7 || p.Kind != run || p.Reverts != nil {
		t.Errorf("partial = %+v, want the run", s.Partial)
	}
}

func TestDeriveTracksStartedAndTop(t *testing.T) {
	s := Derive([]Event{ev(1, 1, run, ok), ev(2, 2, run, failed), ev(3, 3, run, models.ExecQueued), ev(4, 4, run, partial)})
	if !s.Started[1] || !s.Started[2] || s.Started[3] || !s.Started[4] {
		t.Errorf("started = %v", s.Started)
	}
	if s.Open[1] || s.Open[2] || !s.Open[3] || s.Open[4] {
		t.Errorf("open = %v, want only the queued migration", s.Open)
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
