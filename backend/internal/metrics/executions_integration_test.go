//go:build integration

package metrics

import (
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/AthillaZaidan/datanest/backend/internal/models"
	dbtest "github.com/AthillaZaidan/datanest/backend/internal/testutil"
)

func TestExecutionCollectorCountsEveryStatusFromThePlatformDatabase(t *testing.T) {
	db := dbtest.PlatformDB(t)
	conn := dbtest.CreateDatabaseConnection(t, db, "toko")
	queuedAt := time.Now().Add(-90 * time.Second)
	for _, s := range []models.ExecutionStatus{models.ExecQueued, models.ExecQueued, models.ExecRunning, models.ExecSuccess, models.ExecFailed} {
		x := models.MigrationExecution{DatabaseID: conn.ID, Status: s}
		if s == models.ExecQueued {
			x.QueuedAt = &queuedAt
		}
		dbtest.CreateExecution(t, db, x)
	}

	c := NewExecutionCollector(db)
	want := `
# HELP datanest_executions Executions in the platform database by status (read at scrape time).
# TYPE datanest_executions gauge
datanest_executions{status="approved"} 0
datanest_executions{status="canceled"} 0
datanest_executions{status="failed"} 1
datanest_executions{status="partial"} 0
datanest_executions{status="pending_review"} 0
datanest_executions{status="queued"} 2
datanest_executions{status="rejected"} 0
datanest_executions{status="running"} 1
datanest_executions{status="success"} 1
datanest_executions{status="uncertain"} 0
# HELP datanest_executions_scrape_ok 1 when the execution counts above were read from the platform database, 0 when the read failed.
# TYPE datanest_executions_scrape_ok gauge
datanest_executions_scrape_ok 1
`
	if err := testutil.CollectAndCompare(c, strings.NewReader(want), "datanest_executions", "datanest_executions_scrape_ok"); err != nil {
		t.Error(err)
	}
	oldest := gaugeValue(t, c, "datanest_queue_oldest_wait_seconds")
	if oldest < 85 || oldest > 600 {
		t.Errorf("oldest queued wait = %v s, want about 90 s", oldest)
	}
}

func TestExecutionCollectorReportsZeroWaitWhenNothingIsQueued(t *testing.T) {
	db := dbtest.PlatformDB(t)
	if got := gaugeValue(t, NewExecutionCollector(db), "datanest_queue_oldest_wait_seconds"); got != 0 {
		t.Errorf("oldest queued wait = %v, want 0", got)
	}
}

func TestExecutionCollectorFlagsAFailedRead(t *testing.T) {
	db := dbtest.PlatformDB(t)
	sqlDB, _ := db.DB()
	sqlDB.Close()

	want := `
# HELP datanest_executions_scrape_ok 1 when the execution counts above were read from the platform database, 0 when the read failed.
# TYPE datanest_executions_scrape_ok gauge
datanest_executions_scrape_ok 0
`
	c := NewExecutionCollector(db)
	if err := testutil.CollectAndCompare(c, strings.NewReader(want)); err != nil {
		t.Error(err)
	}
}
