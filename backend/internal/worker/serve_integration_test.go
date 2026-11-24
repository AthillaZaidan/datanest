//go:build integration

package worker

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/AthillaZaidan/datanest/backend/internal/models"
	"github.com/AthillaZaidan/datanest/backend/internal/queue"
)

func brokerURL(t *testing.T) string {
	t.Helper()
	url := os.Getenv("TEST_AMQP_URL")
	if url == "" {
		t.Skip("TEST_AMQP_URL not set; start rabbitmq with docker compose and see .env.example")
	}
	return url
}

func brokerSetup(t *testing.T, concurrency int) (*env, string, *queue.Publisher) {
	t.Helper()
	url := brokerURL(t)
	name := fmt.Sprintf("test.worker.%d", time.Now().UnixNano())
	t.Cleanup(func() {
		conn, err := amqp.Dial(url)
		if err != nil {
			return
		}
		defer conn.Close()
		if ch, err := conn.Channel(); err == nil {
			ch.QueueDelete(name, false, false, false)
		}
	})
	cfg := testConfig()
	cfg.Queue = name
	cfg.Concurrency = concurrency
	e := setupWith(t, cfg)
	pub := queue.NewPublisher(url, name)
	t.Cleanup(func() { pub.Close() })
	e.w.Publisher = pub
	return e, url, pub
}

func serve(t *testing.T, e *env, url string) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- e.w.Serve(ctx, url, 50*time.Millisecond) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(15 * time.Second):
			t.Error("the worker did not shut down")
		}
	})
}

func TestAQueuedExecutionRunsEndToEndThroughTheBroker(t *testing.T) {
	e, url, pub := brokerSetup(t, 4)
	x := e.newExec(models.MigrationExecution{CorrelationID: "corr-1"})
	serve(t, e, url)

	waitFor(t, "the execution to succeed", func() bool { return e.get(x.ID).Status == models.ExecSuccess })

	if e.sumQty() != 95 || e.get(x.ID).PublishedAt == nil {
		t.Errorf("sum qty = %d, published_at = %v", e.sumQty(), e.get(x.ID).PublishedAt)
	}

	for i := 0; i < 3; i++ {
		if err := pub.Publish(context.Background(), queue.Job{ExecutionID: x.ID}); err != nil {
			t.Fatal(err)
		}
	}
	time.Sleep(500 * time.Millisecond)
	if e.sumQty() != 95 || len(e.ledger(x.ID)) != 1 {
		t.Errorf("duplicate messages changed the data: sum %d, ledger rows %d", e.sumQty(), len(e.ledger(x.ID)))
	}
}

func TestMalformedAndUnknownMessagesDoNotStopTheWorker(t *testing.T) {
	e, url, pub := brokerSetup(t, 2)
	serve(t, e, url)

	if err := pub.Publish(context.Background(), queue.Job{ExecutionID: 987654}); err != nil {
		t.Fatal(err)
	}
	x := e.newExec(models.MigrationExecution{})
	waitFor(t, "the execution to succeed", func() bool { return e.get(x.ID).Status == models.ExecSuccess })
}

func TestConcurrencyIsBoundedByTheWorkerNotByThePrefetch(t *testing.T) {
	e, url, _ := brokerSetup(t, 1)
	other := e.secondDatabase()
	e.target.Exec(`CREATE TABLE items2 (id int PRIMARY KEY, qty int NOT NULL DEFAULT 0); INSERT INTO items2 (id) VALUES (1)`)
	slow := e.newExec(models.MigrationExecution{SQLExecuted: `UPDATE items SET qty = qty + 1 WHERE (SELECT pg_sleep(1.5)) IS NOT NULL`})
	waiting := e.newExec(models.MigrationExecution{DatabaseID: other.ID, SQLExecuted: `UPDATE items2 SET qty = qty + 1`})
	serve(t, e, url)

	waitFor(t, "the first execution to start", func() bool { return e.get(slow.ID).Status == models.ExecRunning })
	time.Sleep(500 * time.Millisecond)
	if got := e.get(waiting.ID).Status; got != models.ExecQueued {
		t.Fatalf("the second execution is %s while the only slot is busy; the worker must hold it back", got)
	}
	waitFor(t, "both executions to finish", func() bool {
		return e.get(slow.ID).Status == models.ExecSuccess && e.get(waiting.ID).Status == models.ExecSuccess
	})
}

func TestAWorkerThatShutsDownMidRunLeavesARecoverableExecution(t *testing.T) {
	e, url, _ := brokerSetup(t, 2)
	x := e.newExec(models.MigrationExecution{SQLExecuted: `UPDATE items SET qty = qty + 1 WHERE (SELECT pg_sleep(8)) IS NOT NULL`})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- e.w.Serve(ctx, url, 50*time.Millisecond) }()

	waitFor(t, "the execution to start", func() bool { return e.get(x.ID).Status == models.ExecRunning })
	cancel()
	select {
	case <-done:
	case <-time.After(15 * time.Second):
		t.Fatal("the worker did not shut down")
	}

	got := e.get(x.ID)
	if got.Status != models.ExecUncertain || e.sumQty() != 0 {
		t.Errorf("after the shutdown: %s (%s), sum %d; want uncertain with nothing applied", got.Status, got.ErrorCode, e.sumQty())
	}
	e.platform.Model(&models.MigrationExecution{}).Where("id = ?", x.ID).Update("sql_executed", incrementQty)
	e.mustProcess(x.ID)
	e.expectStatus(x.ID, models.ExecSuccess)
	if e.sumQty() != 95 {
		t.Errorf("sum qty = %d, want 95", e.sumQty())
	}
}
