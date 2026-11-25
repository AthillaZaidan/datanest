//go:build integration

package worker

import (
	"net/http/httptest"
	"strconv"
	"testing"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"github.com/AthillaZaidan/datanest/backend/internal/approval"
	"github.com/AthillaZaidan/datanest/backend/internal/authz"
	"github.com/AthillaZaidan/datanest/backend/internal/execution"
	"github.com/AthillaZaidan/datanest/backend/internal/httpapi"
	"github.com/AthillaZaidan/datanest/backend/internal/models"
	"github.com/AthillaZaidan/datanest/backend/internal/obs"
	"github.com/AthillaZaidan/datanest/backend/internal/testutil"
)

func TestOneTraceRunsFromTheHTTPRequestThroughTheBrokerIntoTheWorker(t *testing.T) {
	rec, logs := observe(t)
	e, url, pub := brokerSetup(t, 2)
	requester := testutil.CreateUser(t, e.platform, "dev")
	leader := testutil.CreateUser(t, e.platform, "sari")
	group := testutil.CreateGroup(t, e.platform, "Tim A")
	testutil.AddMember(t, e.platform, group.ID, leader.ID)
	testutil.GrantAccess(t, e.platform, group.ID, e.conn.ID, authz.View)
	e.platform.Model(&models.Group{}).Where("id = ?", group.ID).Update("leader_id", leader.ID)
	x := e.newExec(models.MigrationExecution{Status: models.ExecPendingReview, RequestedByID: &requester.ID, RequestedByUsername: "dev"})

	az := authz.New(e.platform)
	approvals := approval.NewService(e.platform, az)
	approvals.Publisher = pub
	router := httpapi.NewRouter(httpapi.Deps{
		RequireAuth: testutil.AsUser(leader.ID), Executions: execution.NewService(e.platform, az), Approvals: approvals,
	})
	serve(t, e, url)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("POST", "/api/executions/"+strconv.FormatUint(uint64(x.ID), 10)+"/approve", nil))
	if w.Code != 200 {
		t.Fatalf("approve = %d %s", w.Code, w.Body)
	}
	requestID := w.Header().Get(httpapi.RequestIDHeader)
	waitFor(t, "the execution to succeed", func() bool { return e.get(x.ID).Status == models.ExecSuccess })
	waitFor(t, "the worker span to end", func() bool { return spanByName(rec.Ended(), obs.SpanWorkerProcess) != nil })

	spans := rec.Ended()
	byName := func(name string) sdktrace.ReadOnlySpan {
		s := spanByName(spans, name)
		if s == nil {
			t.Fatalf("no %s span among %d", name, len(spans))
		}
		return s
	}
	request := byName("POST /api/executions/:executionId/approve")
	if request.Parent().IsValid() {
		t.Error("the request span has a parent; the trace should start at the request")
	}
	for child, parent := range map[string]string{
		obs.SpanAPIApprove:    "POST /api/executions/:executionId/approve",
		obs.SpanQueuePublish:  obs.SpanAPIApprove,
		obs.SpanWorkerProcess: obs.SpanQueuePublish,
		obs.SpanLockAcquire:   obs.SpanWorkerProcess,
		obs.SpanExecute:       obs.SpanWorkerProcess,
		obs.SpanSQLQuery:      obs.SpanExecute,
	} {
		c, p := byName(child), byName(parent)
		if c.SpanContext().TraceID() != request.SpanContext().TraceID() {
			t.Errorf("%s is in another trace", child)
		}
		if c.Parent().SpanID() != p.SpanContext().SpanID() {
			t.Errorf("%s is not a child of %s", child, parent)
		}
	}

	var sawRequest, sawCommit bool
	for _, l := range logs.lines(t) {
		switch l[obs.AttrEvent] {
		case obs.EventHTTPRequest:
			sawRequest = true
			if l[obs.AttrTraceID] != request.SpanContext().TraceID().String() || l[obs.AttrCorrelationID] != requestID {
				t.Errorf("http.request line = %v, want the request's trace and X-Request-ID %s", l, requestID)
			}
		case obs.EventSQLCommit:
			sawCommit = true
			if l[obs.AttrTraceID] != request.SpanContext().TraceID().String() || l[obs.AttrCorrelationID] != requestID {
				t.Errorf("sql.commit line = %v, want the request's trace and X-Request-ID %s", l, requestID)
			}
		}
	}
	if !sawRequest || !sawCommit {
		t.Errorf("http.request seen %v, sql.commit seen %v", sawRequest, sawCommit)
	}
}
