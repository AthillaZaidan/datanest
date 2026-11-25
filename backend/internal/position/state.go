// Package position derives the active migration position of a database from its
// executions (design D1/D2). It is the single source of truth for what is applied.
package position

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

// State is the active position of one database, derived from its executions in run
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
	// Open holds migrations with an execution that has not reached a final status
	// (waiting for review or the queue, running, uncertain or partial).
	Open map[uint]bool
}

// started is true once a worker picked the execution up; earlier statuses are requests.
func started(s models.ExecutionStatus) bool {
	switch s {
	case models.ExecRunning, models.ExecSuccess, models.ExecFailed, models.ExecPartial, models.ExecUncertain:
		return true
	}
	return false
}

// open is true until the execution reaches a final status.
func open(s models.ExecutionStatus) bool {
	switch s {
	case models.ExecPendingReview, models.ExecApproved, models.ExecQueued, models.ExecRunning,
		models.ExecUncertain, models.ExecPartial:
		return true
	}
	return false
}

// Derive computes the State from all executions of one database, in run order.
func Derive(events []Event) State {
	st := State{Redo: map[uint]bool{}, Started: map[uint]bool{}, Open: map[uint]bool{}}

	for _, e := range events {
		if open(e.Status) {
			st.Open[e.MigrationID] = true
		}
		if !started(e.Status) {
			continue
		}
		ok := e.Status == models.ExecSuccess
		switch e.Kind {
		case models.ExecKindRun:
			st.Started[e.MigrationID] = true
			// Running a new migration discards the redo history, whatever its outcome.
			clear(st.Redo)
			if ok {
				st.Active = append(st.Active, Applied{MigrationID: e.MigrationID, ExecutionID: e.ID})
			} else {
				st.Redo[e.MigrationID] = true // a failed attempt may be forwarded
			}
		case models.ExecKindForward:
			st.Started[e.MigrationID] = true
			if ok {
				st.Active = append(st.Active, Applied{MigrationID: e.MigrationID, ExecutionID: e.ID})
				delete(st.Redo, e.MigrationID)
			}
		case models.ExecKindRollback:
			if n := len(st.Active); ok && n > 0 && e.Reverts != nil && st.Active[n-1].ExecutionID == *e.Reverts {
				st.Active = st.Active[:n-1]
				st.Redo[e.MigrationID] = true
			}
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
