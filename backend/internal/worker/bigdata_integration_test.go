//go:build integration && bigdata

package worker

import (
	"fmt"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/AthillaZaidan/datanest/backend/internal/models"
)

func bigdataRows(t *testing.T) int {
	t.Helper()
	if v := os.Getenv("BIGDATA_ROWS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			t.Fatalf("BIGDATA_ROWS must be a positive integer, got %q", v)
		}
		return n
	}
	return 2_000_000
}

func TestBigDataBatchMatchesOneUnbatchedUpdate(t *testing.T) {
	e := setup(t)
	rows := bigdataRows(t)
	batchSize := max(rows/20, 1)

	if _, err := e.target.Exec(fmt.Sprintf(
		`CREATE TABLE big_orders (id int PRIMARY KEY, status text NOT NULL DEFAULT 'new');
		 INSERT INTO big_orders (id) SELECT g FROM generate_series(1, %d) g;
		 ANALYZE big_orders`, rows)); err != nil {
		t.Fatal(err)
	}

	start := time.Now()
	if _, err := e.target.Exec(`UPDATE big_orders SET status = 'done'`); err != nil {
		t.Fatal(err)
	}
	unbatched := time.Since(start)
	t.Logf("unbatched UPDATE: %d rows in %s", rows, unbatched.Round(time.Millisecond))
	if unbatched <= time.Second {
		t.Fatalf("the unbatched UPDATE took %s; the spec needs more than 1 s, raise BIGDATA_ROWS (now %d)", unbatched, rows)
	}
	if _, err := e.target.Exec(`UPDATE big_orders SET status = 'new'`); err != nil {
		t.Fatal(err)
	}

	size := batchSize
	x := e.newExec(models.MigrationExecution{
		Mode: models.ExecModeBatch, BatchSize: &size, BatchTable: "big_orders", BatchColumn: "id",
		SQLExecuted: `UPDATE big_orders SET status = 'done' WHERE id > $1 AND id <= $2`,
	})
	start = time.Now()
	e.mustProcess(x.ID)
	batched := time.Since(start)
	t.Logf("per-batch run: %d rows in %s (%d rows per batch)", rows, batched.Round(time.Millisecond), batchSize)

	got := e.expectStatus(x.ID, models.ExecSuccess)
	wantBatches := (rows + batchSize - 1) / batchSize
	if got.CommittedBatches != wantBatches || got.CommittedRows != int64(rows) {
		t.Errorf("batches = %d (want %d), rows = %d (want %d)", got.CommittedBatches, wantBatches, got.CommittedRows, rows)
	}
	var done int
	if err := e.target.QueryRow(`SELECT count(*) FROM big_orders WHERE status = 'done'`).Scan(&done); err != nil {
		t.Fatal(err)
	}
	if done != rows {
		t.Errorf("%d of %d rows are done", done, rows)
	}
	if n := len(e.ledger(x.ID)); n != wantBatches {
		t.Errorf("ledger rows = %d, want %d", n, wantBatches)
	}
}
