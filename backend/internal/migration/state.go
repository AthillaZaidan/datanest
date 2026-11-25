package migration

import (
	"github.com/AthillaZaidan/datanest/backend/internal/models"
)

// Event is one execution as seen by the version rules (design D1/D2).
type Event struct {
	ID          uint
	MigrationID uint
	Kind        models.ExecutionKind
	Status      models.ExecutionStatus
	Reverts     *uint
}

// Applied is a migration in the active stack and the execution that applied it.
type Applied struct {
	MigrationID uint
	ExecutionID uint
}

// State is the active position of one database, derived from its executions in id
// order and never stored separately:
//   - a successful run or forward puts its migration on top of the active stack;
//   - a successful rollback takes it off again;
//   - anything else (failed, pending, rejected...) changes nothing.
type State struct {
	// Active is bottom-first; the last element is the top (the only one that may be rolled back).
	Active []Applied
	// Redo holds migrations that may be forwarded: tried before, not active now, and not
	// discarded by a later run of a new migration (undo/redo: a new run clears redo).
	Redo map[uint]bool
	// Started holds every migration that has had an execution start, so a further
	// attempt must be a forward, not a run.
	Started map[uint]bool
}

// started is true once a worker picked the execution up; earlier statuses are requests.
func started(s models.ExecutionStatus) bool {
	switch s {
	case models.ExecRunning, models.ExecSuccess, models.ExecFailed, models.ExecPartial, models.ExecUncertain:
		return true
	}
	return false
}

// Derive computes the State from all executions of one database, sorted by id.
func Derive(events []Event) State {
	st := State{Redo: map[uint]bool{}, Started: map[uint]bool{}}
	lastAttempt := map[uint]uint{} // migration -> id of its latest started run/forward
	var lastRun uint                // id of the latest started run of any migration

	for _, e := range events {
		if !started(e.Status) {
			continue
		}
		if e.Kind != models.ExecKindRollback {
			st.Started[e.MigrationID] = true
			lastAttempt[e.MigrationID] = e.ID
			if e.Kind == models.ExecKindRun {
				lastRun = e.ID
			}
		}
		if e.Status != models.ExecSuccess {
			continue
		}
		switch e.Kind {
		case models.ExecKindRun, models.ExecKindForward:
			st.Active = append(st.Active, Applied{MigrationID: e.MigrationID, ExecutionID: e.ID})
		case models.ExecKindRollback:
			if n := len(st.Active); n > 0 && e.Reverts != nil && st.Active[n-1].ExecutionID == *e.Reverts {
				st.Active = st.Active[:n-1]
			}
		}
	}

	active := map[uint]bool{}
	for _, a := range st.Active {
		active[a.MigrationID] = true
	}
	for m, at := range lastAttempt {
		if !active[m] && at >= lastRun {
			st.Redo[m] = true
		}
	}
	return st
}

// Top is the migration on top of the active stack.
func (s State) Top() (Applied, bool) {
	if len(s.Active) == 0 {
		return Applied{}, false
	}
	return s.Active[len(s.Active)-1], true
}

// IsActive reports whether the migration is currently applied.
func (s State) IsActive(migrationID uint) bool {
	for _, a := range s.Active {
		if a.MigrationID == migrationID {
			return true
		}
	}
	return false
}
