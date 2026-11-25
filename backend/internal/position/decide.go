package position

import (
	"fmt"

	"github.com/AthillaZaidan/datanest/backend/internal/apperr"
	"github.com/AthillaZaidan/datanest/backend/internal/models"
)

// Plan is what a valid request will execute.
type Plan struct {
	Kind models.ExecutionKind
	// SQL is copied into the execution: the up SQL for run/forward, the down SQL for rollback.
	SQL string
	// Reverts is the execution a rollback undoes.
	Reverts *uint
}

// Decide applies the version rules (design D2) to a request to execute m. versions maps
// each migration of the database to its version; drafts have none.
func Decide(st State, versions map[uint]int, m *models.Migration, kind models.ExecutionKind) (Plan, error) {
	if st.Open[m.ID] {
		return Plan{}, apperr.ErrExecutionStateConflict.WithMessage("Migration ini masih punya eksekusi yang belum selesai.")
	}
	switch kind {
	case models.ExecKindRun:
		if st.Started[m.ID] {
			return Plan{}, apperr.ErrExecutionStateConflict.WithMessage("Migration ini sudah pernah dijalankan. Gunakan forward untuk menjalankannya lagi.")
		}
		return Plan{Kind: kind, SQL: m.SQL}, nil

	case models.ExecKindForward:
		next, ok := NextForward(st, versions)
		if !ok {
			return Plan{}, apperr.ErrForwardNotAllowed.WithMessage("Tidak ada migration yang dapat dijalankan maju.")
		}
		if next != m.ID {
			return Plan{}, apperr.ErrForwardNotAllowed.WithMessage(fmt.Sprintf(
				"Forward mengikuti urutan linear: jalankan maju v%d dahulu.", versions[next]))
		}
		return Plan{Kind: kind, SQL: m.SQL}, nil

	case models.ExecKindRollback:
		top, ok := st.Top()
		if !ok || top.MigrationID != m.ID {
			return Plan{}, apperr.ErrRollbackNotAllowed.WithMessage("Hanya migration aktif teratas yang dapat di-rollback.")
		}
		if m.Irreversible {
			msg := "Migration ini irreversible dan tidak dapat di-rollback."
			if m.IrreversibleReason != "" {
				msg += " Alasan: " + m.IrreversibleReason
			}
			return Plan{}, apperr.ErrRollbackNotAllowed.WithMessage(msg)
		}
		reverts := top.ExecutionID
		return Plan{Kind: kind, SQL: m.SQLDown, Reverts: &reverts}, nil
	}
	return Plan{}, apperr.ErrValidation.WithDetails(map[string]string{"kind": "harus run, forward, atau rollback"})
}

// NextForward is the migration a forward may run: the lowest version in the redo history.
func NextForward(st State, versions map[uint]int) (uint, bool) {
	var best uint
	found := false
	for m := range st.Redo {
		v, ok := versions[m]
		if !ok {
			continue
		}
		if !found || v < versions[best] {
			best, found = m, true
		}
	}
	return best, found
}

// ActiveVersion is the version of the top active migration, or 0 when nothing is active.
func ActiveVersion(st State, versions map[uint]int) int {
	if top, ok := st.Top(); ok {
		return versions[top.MigrationID]
	}
	return 0
}
