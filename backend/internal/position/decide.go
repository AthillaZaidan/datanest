package position

import (
	"fmt"
	"strings"

	"github.com/AthillaZaidan/datanest/backend/internal/apperr"
	"github.com/AthillaZaidan/datanest/backend/internal/migration/sqlguard"
	"github.com/AthillaZaidan/datanest/backend/internal/models"
)

// Plan is what a valid request will execute.
type Plan struct {
	Kind models.ExecutionKind
	// SQL is copied into the execution: the up SQL for run/forward, the down SQL for rollback.
	SQL string
	// Reverts is the execution a rollback undoes.
	Reverts   *uint
	Continues *uint
}

// Decide applies the version rules (design D2) to a request to execute m. versions maps
// each migration of the database to its version; drafts have none.
func Decide(st State, versions map[uint]int, m *models.Migration, kind models.ExecutionKind) (Plan, error) {
	if st.Open[m.ID] {
		return Plan{}, apperr.ErrExecutionStateConflict.WithMessage("Migration ini masih punya eksekusi yang belum selesai.")
	}
	if p := st.Partial; p != nil {
		if p.MigrationID == m.ID {
			return decidePartial(*p, m, kind)
		}
		if kind == models.ExecKindForward || kind == models.ExecKindRollback {
			return Plan{}, apperr.ErrExecutionStateConflict.WithMessage(fmt.Sprintf(
				"Database ini masih punya migration yang baru sebagian (eksekusi E%d). Lanjutkan atau batalkan dulu.", p.ExecutionID))
		}
	}
	switch kind {
	case models.ExecKindRun:
		if _, versioned := versions[m.ID]; versioned && st.Started[m.ID] {
			return Plan{}, apperr.ErrExecutionStateConflict.WithMessage("Migration ini sudah pernah dijalankan. Gunakan forward untuk menjalankannya lagi.")
		}
		if err := complete(m); err != nil {
			return Plan{}, err
		}
		return Plan{Kind: kind, SQL: m.SQL}, nil

	case models.ExecKindForward:
		if !Forwardable(st, versions, m.ID) {
			return Plan{}, refuseForward(st, versions)
		}
		return Plan{Kind: kind, SQL: m.SQL}, nil

	case models.ExecKindRollback:
		top, ok := st.Top()
		if !ok || top.MigrationID != m.ID {
			return Plan{}, apperr.ErrRollbackNotAllowed.WithMessage("Hanya migration aktif teratas yang dapat di-rollback.")
		}
		if err := reversible(m); err != nil {
			return Plan{}, err
		}
		reverts := top.ExecutionID
		return Plan{Kind: kind, SQL: m.SQLDown, Reverts: &reverts}, nil
	}
	return Plan{}, unknownKind()
}

func decidePartial(p Partial, m *models.Migration, kind models.ExecutionKind) (Plan, error) {
	undoing := p.Kind == models.ExecKindRollback
	switch kind {
	case models.ExecKindRun, models.ExecKindForward:
		if undoing {
			return Plan{}, apperr.ErrExecutionStateConflict.WithMessage("Rollback migration ini baru sebagian. Lanjutkan rollback-nya dulu.")
		}
		if kind == models.ExecKindRun {
			return Plan{}, apperr.ErrExecutionStateConflict.WithMessage("Migration ini baru sebagian dijalankan. Lanjutkan dengan forward atau batalkan dengan rollback.")
		}
		return Plan{Kind: kind, SQL: m.SQL, Continues: &p.ExecutionID}, nil

	case models.ExecKindRollback:
		if undoing {
			return Plan{Kind: kind, SQL: m.SQLDown, Reverts: p.Reverts, Continues: &p.ExecutionID}, nil
		}
		if err := reversible(m); err != nil {
			return Plan{}, err
		}
		return Plan{Kind: kind, SQL: m.SQLDown, Reverts: &p.ExecutionID}, nil
	}
	return Plan{}, unknownKind()
}

func refuseForward(st State, versions map[uint]int) error {
	msg := "Tidak ada migration yang dapat dijalankan maju."
	if next, ok := NextForward(st, versions); ok {
		msg = fmt.Sprintf("Forward mengikuti urutan linear: jalankan maju v%d dahulu.", versions[next])
	} else if Forwardable(st, versions, st.Retry) {
		msg = fmt.Sprintf("Hanya v%d yang dapat dijalankan maju.", versions[st.Retry])
	}
	return apperr.ErrForwardNotAllowed.WithMessage(msg)
}

func reversible(m *models.Migration) error {
	if m.Irreversible {
		msg := "Migration ini irreversible dan tidak dapat di-rollback."
		if m.IrreversibleReason != "" {
			msg += " Alasan: " + m.IrreversibleReason
		}
		return apperr.ErrRollbackNotAllowed.WithMessage(msg)
	}
	if strings.TrimSpace(m.SQLDown) == "" {
		return apperr.ErrRollbackNotAllowed.WithMessage("Migration ini tidak punya SQL down.")
	}
	return nil
}

func unknownKind() error {
	return apperr.ErrValidation.WithDetails(map[string]string{"kind": "harus run, forward, atau rollback"})
}

// complete checks that a recipe may be run: up SQL, plus down SQL or a reason it is
// irreversible, and the per-batch contract of a batch recipe.
func complete(m *models.Migration) error {
	details := map[string]string{}
	if strings.TrimSpace(m.SQL) == "" {
		details["sqlUp"] = "wajib diisi"
	}
	if m.Irreversible && strings.TrimSpace(m.IrreversibleReason) == "" {
		details["irreversibleReason"] = "wajib diisi untuk migration irreversible"
	}
	if !m.Irreversible && strings.TrimSpace(m.SQLDown) == "" {
		details["sqlDown"] = "wajib diisi, atau tandai migration irreversible beserta alasannya"
	}
	if m.Mode == models.ExecModeBatch {
		batchContract(details, "sqlUp", m.SQL)
		if !m.Irreversible {
			batchContract(details, "sqlDown", m.SQLDown)
		}
	}
	if len(details) > 0 {
		return apperr.ErrValidation.WithMessage("Migration belum lengkap.").WithDetails(details)
	}
	return nil
}

func batchContract(details map[string]string, field, sql string) {
	if details[field] != "" {
		return
	}
	if problem := sqlguard.BatchProblem(sql); problem != "" {
		details[field] = problem
	}
}

func Forwardable(st State, versions map[uint]int, migrationID uint) bool {
	if next, ok := NextForward(st, versions); ok && next == migrationID {
		return true
	}
	_, versioned := versions[migrationID]
	return versioned && st.Retry == migrationID
}

// NextForward is the migration to redo next: the lowest version in the redo history.
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
