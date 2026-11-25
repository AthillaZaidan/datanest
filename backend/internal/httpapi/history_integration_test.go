//go:build integration

package httpapi

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/AthillaZaidan/datanest/backend/internal/migration"
	"github.com/AthillaZaidan/datanest/backend/internal/models"
)

func (e migEnv) history(t *testing.T, u models.User, query string) (int, []uint) {
	t.Helper()
	r := call(e.as(u), "GET", "/api/databases/"+itoa(e.dbA)+"/executions"+query, nil)
	if r.Code != 200 {
		return r.Code, nil
	}
	var body struct{ Executions []migration.HistoryItem }
	if err := json.Unmarshal(r.Body, &body); err != nil {
		t.Fatalf("decode: %v (%s)", err, r.Body)
	}
	out := []uint{}
	for _, it := range body.Executions {
		out = append(out, it.ID)
	}
	return r.Code, out
}

// The example from the specification: A[E1], B[E2], B rollback of E2 [E3], B forward [E4], C[E5].
func TestHistoryHideRollback(t *testing.T) {
	e := setupMigrations(t)
	mig := func(version int) uint {
		m := models.Migration{DatabaseID: e.dbA, Version: &version, SQL: "SELECT 1", CreatedByUsername: "editor"}
		if err := e.platform.Create(&m).Error; err != nil {
			t.Fatal(err)
		}
		return m.ID
	}
	exec := func(migrationID uint, kind models.ExecutionKind, status models.ExecutionStatus, reverts *uint) uint {
		x := models.MigrationExecution{MigrationID: migrationID, DatabaseID: e.dbA, Kind: kind, Status: status,
			SQLExecuted: "SELECT 1", RevertsExecutionID: reverts, RequestedByUsername: "editor"}
		if err := e.platform.Create(&x).Error; err != nil {
			t.Fatal(err)
		}
		return x.ID
	}
	a, b, c := mig(1), mig(2), mig(3)
	e1 := exec(a, models.ExecKindRun, models.ExecSuccess, nil)
	e2 := exec(b, models.ExecKindRun, models.ExecSuccess, nil)
	e3 := exec(b, models.ExecKindRollback, models.ExecSuccess, &e2)
	e4 := exec(b, models.ExecKindForward, models.ExecSuccess, nil)
	e5 := exec(c, models.ExecKindRun, models.ExecSuccess, nil)
	failedUndo := exec(c, models.ExecKindRollback, models.ExecFailed, &e5)
	exec(c, models.ExecKindRollback, models.ExecPendingReview, &e5) // not started: review screen only

	if code, got := e.history(t, e.viewer, ""); code != 200 || !reflect.DeepEqual(got, []uint{e1, e2, e3, e4, e5, failedUndo}) {
		t.Errorf("full history = %d %v", code, got)
	}
	if code, got := e.history(t, e.viewer, "?hideRollback=true"); code != 200 || !reflect.DeepEqual(got, []uint{e1, e4, e5, failedUndo}) {
		t.Errorf("hide rollback = %d %v, want [E1 E4 E5] plus the failed attempt", code, got)
	}
	var n int64
	e.platform.Model(&models.MigrationExecution{}).Where("database_id = ?", e.dbA).Count(&n)
	if n != 7 {
		t.Errorf("filter deleted rows: %d left, want 7", n)
	}
	if code, _ := e.history(t, e.otherEditor, ""); code != 403 {
		t.Errorf("foreign editor = %d, want 403", code)
	}
}
