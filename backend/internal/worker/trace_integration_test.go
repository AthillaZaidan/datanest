//go:build integration

package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"sync"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/AthillaZaidan/datanest/backend/internal/approval"
	"github.com/AthillaZaidan/datanest/backend/internal/authz"
	"github.com/AthillaZaidan/datanest/backend/internal/models"
	"github.com/AthillaZaidan/datanest/backend/internal/obs"
	"github.com/AthillaZaidan/datanest/backend/internal/testutil"
)

type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) lines(t *testing.T) []map[string]any {
	t.Helper()
	b.mu.Lock()
	defer b.mu.Unlock()
	var out []map[string]any
	dec := json.NewDecoder(bytes.NewReader(b.buf.Bytes()))
	for dec.More() {
		var line map[string]any
		if err := dec.Decode(&line); err != nil {
			t.Fatalf("log is not JSON: %v", err)
		}
		out = append(out, line)
	}
	return out
}

func (b *lockedBuffer) text() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func observe(t *testing.T) (*tracetest.SpanRecorder, *lockedBuffer) {
	t.Helper()
	if _, err := obs.SetupTracing(nil); err != nil { // installs the traceparent propagator
		t.Fatal(err)
	}
	rec := tracetest.NewSpanRecorder()
	prevTP, prevLog := otel.GetTracerProvider(), slog.Default()
	otel.SetTracerProvider(sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec)))
	logs := &lockedBuffer{}
	slog.SetDefault(slog.New(obs.NewHandler(logs, slog.LevelDebug)))
	t.Cleanup(func() {
		otel.SetTracerProvider(prevTP)
		slog.SetDefault(prevLog)
	})
	return rec, logs
}

func (e *env) approvedBy(x models.MigrationExecution, correlationID string) models.MigrationExecution {
	e.t.Helper()
	leader := testutil.CreateUser(e.t, e.platform, "leader-"+correlationID)
	group := testutil.CreateGroup(e.t, e.platform, "group-"+correlationID)
	testutil.AddMember(e.t, e.platform, group.ID, leader.ID)
	testutil.GrantAccess(e.t, e.platform, group.ID, x.DatabaseID, authz.View)
	if err := e.platform.Model(&models.Group{}).Where("id = ?", group.ID).Update("leader_id", leader.ID).Error; err != nil {
		e.t.Fatal(err)
	}
	svc := approval.NewService(e.platform, authz.New(e.platform))
	ctx := obs.WithCorrelationID(context.Background(), correlationID)
	approvers, _, err := approval.Approvers(ctx, e.platform, x.DatabaseID)
	if err != nil {
		e.t.Fatal(err)
	}
	for _, a := range approvers {
		if _, err := svc.Approve(ctx, a.UserID, x.ID); err != nil {
			e.t.Fatalf("approve as %s: %v", a.Username, err)
		}
	}
	got := e.get(x.ID)
	if got.Status != models.ExecQueued {
		e.t.Fatalf("status after approval = %s, want queued", got.Status)
	}
	return got
}

func spanByName(spans []sdktrace.ReadOnlySpan, name string) sdktrace.ReadOnlySpan {
	for _, s := range spans {
		if s.Name() == name {
			return s
		}
	}
	return nil
}

func TestTraceAndCorrelationIDRunFromTheApprovalIntoTheWorker(t *testing.T) {
	rec, logs := observe(t)
	e := setup(t)
	requester := testutil.CreateUser(t, e.platform, "dev")
	x := e.newExec(models.MigrationExecution{
		Status: models.ExecPendingReview, SQLExecuted: `UPDATE items SET tag = 'hunter2'`,
		RequestedByID: &requester.ID, RequestedByUsername: "dev",
	})
	queued := e.approvedBy(x, "req-42")
	if queued.CorrelationID != "req-42" || !strings.HasPrefix(queued.Traceparent, "00-") {
		t.Fatalf("queued row: correlation %q traceparent %q", queued.CorrelationID, queued.Traceparent)
	}

	e.mustProcess(x.ID)
	e.expectStatus(x.ID, models.ExecSuccess)

	spans := rec.Ended()
	approve := spanByName(spans, obs.SpanAPIApprove)
	if approve == nil {
		t.Fatalf("no %s span among %d spans", obs.SpanAPIApprove, len(spans))
	}
	traceID := approve.SpanContext().TraceID()
	for _, name := range []string{obs.SpanWorkerProcess, obs.SpanLockAcquire, obs.SpanExecute, obs.SpanSQLQuery} {
		s := spanByName(spans, name)
		if s == nil {
			t.Fatalf("no %s span", name)
		}
		if s.SpanContext().TraceID() != traceID {
			t.Errorf("%s is in trace %s, want %s", name, s.SpanContext().TraceID(), traceID)
		}
	}
	process := spanByName(spans, obs.SpanWorkerProcess)
	if process.Parent().SpanID() != approve.SpanContext().SpanID() {
		t.Errorf("worker.process is not a child of api.approve")
	}
	for child, parent := range map[string]string{obs.SpanLockAcquire: obs.SpanWorkerProcess, obs.SpanExecute: obs.SpanWorkerProcess, obs.SpanSQLQuery: obs.SpanExecute} {
		c, p := spanByName(spans, child), spanByName(spans, parent)
		if c.Parent().SpanID() != p.SpanContext().SpanID() {
			t.Errorf("%s is not a child of %s", child, parent)
		}
	}

	byEvent := map[string]map[string]any{}
	for _, line := range logs.lines(t) {
		ev, _ := line[obs.AttrEvent].(string)
		byEvent[ev] = line
		if _, hasExec := line[obs.AttrExecutionID]; hasExec {
			if line[obs.AttrTraceID] != traceID.String() {
				t.Errorf("%s has trace_id %v, want %s", ev, line[obs.AttrTraceID], traceID)
			}
			if line[obs.AttrCorrelationID] != "req-42" {
				t.Errorf("%s has correlation_id %v, want req-42", ev, line[obs.AttrCorrelationID])
			}
		}
	}
	for _, ev := range []string{obs.EventExecutionApproved, obs.EventLockAcquired, obs.EventSQLStart, obs.EventSQLResult, obs.EventSQLCommit} {
		if byEvent[ev] == nil {
			t.Errorf("no %s log line", ev)
		}
	}
	for _, ev := range []string{obs.EventLockAcquired, obs.EventSQLStart, obs.EventSQLResult, obs.EventSQLCommit} {
		line := byEvent[ev]
		if line[obs.AttrActorID] != float64(requester.ID) || line[obs.AttrActor] != "dev" ||
			line[obs.AttrMigrationID] != float64(x.MigrationID) || line[obs.AttrJobID] != float64(x.ID) {
			t.Errorf("%s lacks the actor or the migration/job ids: %v", ev, line)
		}
	}
	for ev, want := range map[string]string{obs.EventSQLStart: "started", obs.EventSQLResult: "ok", obs.EventSQLCommit: "committed"} {
		if byEvent[ev][obs.AttrStatus] != want {
			t.Errorf("%s status = %v, want %s", ev, byEvent[ev][obs.AttrStatus], want)
		}
	}
	if q, _ := byEvent[obs.EventSQLStart][obs.AttrQuery].(string); !strings.Contains(q, "tag = '?'") {
		t.Errorf("sql.start query = %q, want its literal masked", q)
	}
	if strings.Contains(logs.text(), "hunter2") {
		t.Error("a string literal of the user's SQL reached the log")
	}
}

func TestAFailedExecutionMarksItsSpansAsErrors(t *testing.T) {
	rec, logs := observe(t)
	e := setup(t)
	x := e.newExec(models.MigrationExecution{Status: models.ExecPendingReview, SQLExecuted: `UPDATE items SET qty = 1 / 0`})
	e.approvedBy(x, "req-err")
	e.mustProcess(x.ID)
	e.expectStatus(x.ID, models.ExecFailed)

	for _, name := range []string{obs.SpanSQLQuery, obs.SpanExecute, obs.SpanWorkerProcess} {
		s := spanByName(rec.Ended(), name)
		if s == nil {
			t.Fatalf("no %s span", name)
		}
		if s.Status().Code != codes.Error {
			t.Errorf("%s status = %v, want Error", name, s.Status().Code)
		}
	}
	if s := spanByName(rec.Ended(), obs.SpanLockAcquire); s == nil || s.Status().Code == codes.Error {
		t.Errorf("lock.acquire should not be an error: %v", s)
	}
	var rolledBack bool
	for _, l := range logs.lines(t) {
		if l[obs.AttrEvent] == obs.EventSQLRollback && l[obs.AttrStatus] == "rolled_back" && l[obs.AttrJobID] == float64(x.ID) {
			rolledBack = true
		}
	}
	if !rolledBack {
		t.Error("no sql.rollback line with status rolled_back for the failed execution")
	}
}

func TestRollbackAndBatchesAreLoggedAndTraced(t *testing.T) {
	rec, logs := observe(t)
	e := setup(t)

	failing := e.newExec(models.MigrationExecution{Status: models.ExecPendingReview, SQLExecuted: `UPDATE items SET qty = 1 / 0`})
	e.approvedBy(failing, "req-fail")
	e.mustProcess(failing.ID)
	e.expectStatus(failing.ID, models.ExecFailed)

	batched := e.newBatchExec(incrementQtyInRange, 40, models.MigrationExecution{Status: models.ExecPendingReview})
	e.approvedBy(batched, "req-batch")
	e.mustProcess(batched.ID)
	e.expectStatus(batched.ID, models.ExecSuccess)

	events := map[string][]map[string]any{}
	for _, line := range logs.lines(t) {
		ev, _ := line[obs.AttrEvent].(string)
		events[ev] = append(events[ev], line)
	}
	if len(events[obs.EventSQLRollback]) == 0 {
		t.Error("a failed statement left no sql.rollback line")
	}
	var failedResult map[string]any
	for _, l := range events[obs.EventSQLResult] {
		if l[obs.AttrSQLState] != nil && l[obs.AttrSQLState] != "" {
			failedResult = l
		}
	}
	if failedResult == nil || failedResult[obs.AttrSQLState] != "22012" {
		t.Errorf("sql.result of the failed statement = %v, want SQLSTATE 22012", failedResult)
	}
	if got := len(events[obs.EventSQLCommit]); got < 3 {
		t.Errorf("sql.commit lines = %d, want at least 3 (one per batch)", got)
	}

	var batchSpans []string
	for _, s := range rec.Ended() {
		if strings.HasPrefix(s.Name(), obs.SpanBatch+"[") {
			batchSpans = append(batchSpans, s.Name())
			if s.Parent().SpanID() != spanByName(rec.Ended(), obs.SpanExecute).SpanContext().SpanID() &&
				!hasSpanID(rec.Ended(), s.Parent().SpanID(), obs.SpanExecute) {
				t.Errorf("%s is not a child of an execute span", s.Name())
			}
		}
	}
	var withParams int
	for _, l := range events[obs.EventSQLStart] {
		if params, _ := l[obs.AttrParams].([]any); len(params) == 2 {
			withParams++
		}
	}
	if withParams != 3 {
		t.Errorf("sql.start lines with the batch bounds as params = %d, want 3", withParams)
	}
	if len(batchSpans) != 3 {
		t.Errorf("batch spans = %v, want batch[1] to batch[3] for 95 rows in batches of 40", batchSpans)
	}
}

func hasSpanID(spans []sdktrace.ReadOnlySpan, id interface{ String() string }, name string) bool {
	for _, s := range spans {
		if s.Name() == name && s.SpanContext().SpanID().String() == id.String() {
			return true
		}
	}
	return false
}
