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
	Continues   *uint
}

// Applied is a migration in the active stack and the execution that applied it.
type Applied struct {
	MigrationID uint
	ExecutionID uint
}

type Partial struct {
	ExecutionID uint
	MigrationID uint
	Kind        models.ExecutionKind
	Reverts     *uint
}

// State is the active position of one database, derived from its executions in run
// order and never stored separately:
//   - a successful run or forward puts its migration on top of the active stack;
//   - a successful rollback takes it off again;
//   - anything else (failed, partial, pending, rejected...) leaves the active stack as it is.
type State struct {
	// Active is bottom-first; the last element is the top (the only one that may be rolled back).
	Active []Applied
	// Redo holds migrations that may be forwarded in version order: rolled back (or failed
	// above one that was), and not discarded by a newer migration being applied (undo/redo:
	// an applied run clears redo).
	Redo  map[uint]bool
	Retry uint
	// Started holds every migration that has had a run or forward start; once it has a
	// version, a further attempt must be a forward, not a run.
	Started map[uint]bool
	// Open holds migrations with an execution that has not reached a final status
	// (waiting for review or the queue, running or uncertain).
	Open    map[uint]bool
	Partial *Partial
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
		models.ExecUncertain:
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
		applied := Applied{MigrationID: e.MigrationID, ExecutionID: e.ID}
		switch e.Kind {
		case models.ExecKindRun:
			st.Started[e.MigrationID] = true
			switch e.Status {
			case models.ExecSuccess:
				st.discard()
				st.Active = append(st.Active, applied)
			case models.ExecPartial:
				st.discard()
			case models.ExecFailed:
				st.shelveRetry()
				st.Retry = e.MigrationID
			}
		case models.ExecKindForward:
			st.Started[e.MigrationID] = true
			if ok {
				if st.Partial.continuedBy(e) {
					st.Partial = nil
				}
				if e.MigrationID == st.Retry {
					st.discard()
				} else {
					delete(st.Redo, e.MigrationID)
				}
				st.Active = append(st.Active, applied)
			}
		case models.ExecKindRollback:
			if ok {
				st.rollback(e)
			}
		}
		if e.Status == models.ExecPartial {
			st.Partial = &Partial{ExecutionID: e.ID, MigrationID: e.MigrationID, Kind: e.Kind, Reverts: e.Reverts}
		}
	}
	return st
}

func (s *State) discard() {
	clear(s.Redo)
	s.Retry = 0
}

func (s *State) rollback(e Event) {
	n := len(s.Active)
	onTop := n > 0 && refersTo(e.Reverts, s.Active[n-1].ExecutionID)
	settles := s.Partial.continuedBy(e) || s.Partial.undoneBy(e)
	if !onTop && !settles {
		return
	}
	if onTop {
		s.Active = s.Active[:n-1]
		s.shelveRetry()
	}
	if settles {
		s.Partial = nil
	}
	s.Redo[e.MigrationID] = true
}

func (s *State) shelveRetry() {
	if s.Retry != 0 {
		s.Redo[s.Retry] = true
		s.Retry = 0
	}
}

func (p *Partial) continuedBy(e Event) bool {
	return p != nil && refersTo(e.Continues, p.ExecutionID)
}

func (p *Partial) undoneBy(e Event) bool {
	return p != nil && p.Kind != models.ExecKindRollback && refersTo(e.Reverts, p.ExecutionID)
}

func refersTo(id *uint, executionID uint) bool {
	return id != nil && *id == executionID
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
