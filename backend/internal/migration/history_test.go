package migration

import (
	"reflect"
	"testing"

	"github.com/AthillaZaidan/datanest/backend/internal/models"
)

func item(id, migration uint, kind models.ExecutionKind, status models.ExecutionStatus, reverts uint) HistoryItem {
	it := HistoryItem{ID: id, MigrationID: migration, Kind: kind, Status: status}
	if reverts != 0 {
		it.RevertsExecutionID = &reverts
	}
	return it
}

func ids(items []HistoryItem) []uint {
	out := []uint{}
	for _, it := range items {
		out = append(out, it.ID)
	}
	return out
}

func TestHideRolledBack(t *testing.T) {
	const a, b, c = 1, 2, 3
	run, fwd, undo := models.ExecKindRun, models.ExecKindForward, models.ExecKindRollback
	ok, failed := models.ExecSuccess, models.ExecFailed

	tests := []struct {
		name  string
		items []HistoryItem
		want  []uint
	}{
		{"design example: A[E1], B[E2], B-rollback(E2)[E3], B-forward[E4], C[E5]",
			[]HistoryItem{item(1, a, run, ok, 0), item(2, b, run, ok, 0), item(3, b, undo, ok, 2), item(4, b, fwd, ok, 0), item(5, c, run, ok, 0)},
			[]uint{1, 4, 5}},
		{"failed rollback stays, and so does its target",
			[]HistoryItem{item(1, a, run, ok, 0), item(2, a, undo, failed, 1)}, []uint{1, 2}},
		{"failed attempts stay visible",
			[]HistoryItem{item(1, a, run, failed, 0), item(2, a, fwd, ok, 0), item(3, a, undo, ok, 2)}, []uint{1}},
		{"pairs are matched by execution ID, not by migration",
			[]HistoryItem{item(1, a, run, ok, 0), item(2, a, undo, ok, 1), item(3, a, fwd, ok, 0)}, []uint{3}},
		{"nothing to hide", []HistoryItem{item(1, a, run, ok, 0)}, []uint{1}},
	}
	for _, tt := range tests {
		if got := ids(hideRolledBack(tt.items)); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("%s: got %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestHideRolledBackDoesNotModifyItsInput(t *testing.T) {
	in := []HistoryItem{item(1, 1, models.ExecKindRun, models.ExecSuccess, 0), item(2, 1, models.ExecKindRollback, models.ExecSuccess, 1)}
	hideRolledBack(in)
	if len(in) != 2 || in[0].ID != 1 || in[1].ID != 2 {
		t.Errorf("input changed: %v", ids(in))
	}
}
