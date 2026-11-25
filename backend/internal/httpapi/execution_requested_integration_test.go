//go:build integration

package httpapi

import (
	"testing"

	"github.com/AthillaZaidan/datanest/backend/internal/obs"
)

func TestRequestingAnExecutionIsLogged(t *testing.T) {
	lines := captureLogs(t)
	e := setupMigrations(t)
	d := e.recipe(t, map[string]any{"sqlUp": "CREATE TABLE r (id int)", "sqlDown": "DROP TABLE r"}).view(t)

	r := e.submit(e.editor, d.ID, map[string]any{"kind": "run"})
	if r.Code != 201 {
		t.Fatalf("submit = %d %s", r.Code, r.Body)
	}
	line := firstEvent(lines(), obs.EventExecutionRequested)
	if line == nil {
		t.Fatal("no execution.requested line")
	}
	if line[obs.AttrActorID] != float64(e.editor.ID) || line[obs.AttrMigrationID] != float64(d.ID) ||
		line[obs.AttrDatabaseID] != float64(e.dbA) || line["kind"] != "run" || line[obs.AttrStatus] != "pending_review" {
		t.Errorf("execution.requested = %v", line)
	}
	if line[obs.AttrExecutionID] == nil {
		t.Error("no execution_id")
	}
}
