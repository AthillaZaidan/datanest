package obs

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"reflect"
	"strings"
	"testing"

	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func useRecorder(t *testing.T) *tracetest.SpanRecorder {
	t.Helper()
	if _, err := SetupTracing(nil); err != nil {
		t.Fatal(err)
	}
	rec := tracetest.NewSpanRecorder()
	prev := otel.GetTracerProvider()
	otel.SetTracerProvider(sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec)))
	t.Cleanup(func() { otel.SetTracerProvider(prev) })
	return rec
}

func logLine(t *testing.T, fn func(l *slog.Logger)) map[string]any {
	t.Helper()
	var buf bytes.Buffer
	fn(slog.New(NewHandler(&buf, slog.LevelDebug)))
	var out map[string]any
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("log line is not JSON: %q: %v", buf.String(), err)
	}
	return out
}

func TestSensitiveKeysAreRedacted(t *testing.T) {
	for _, key := range []string{"password", "Password", "sid", "session", "csrf", "csrf_token", "token", "authorization", "cookie", "Set-Cookie"} {
		got := logLine(t, func(l *slog.Logger) { l.Info(EventAuthLogin, key, "hunter2") })
		if got[key] != Redacted {
			t.Errorf("%s = %v, want %s", key, got[key], Redacted)
		}
	}
	got := logLine(t, func(l *slog.Logger) { l.Info(EventLockAcquired, "lock_token", 7, "username", "sari") })
	if got["lock_token"] != float64(7) || got["username"] != "sari" {
		t.Errorf("harmless attributes were changed: %v", got)
	}
}

func TestErrorTextHasItsCredentialsMasked(t *testing.T) {
	cases := map[string]string{
		`failed to connect to host=db user=app password=hunter2 dbname=toko`: `failed to connect to host=db user=app password=` + Redacted + ` dbname=toko`,
		`dial postgres://platform_app:hunter2@localhost:5432/toko: refused`:  `dial postgres://` + Redacted + `@localhost:5432/toko: refused`,
		`PASSWORD: hunter2`: `PASSWORD: ` + Redacted,
		`duplicate key value violates unique constraint "orders_pkey"`: `duplicate key value violates unique constraint "orders_pkey"`,
	}
	for in, want := range cases {
		got := logLine(t, func(l *slog.Logger) { l.Warn(EventSQLResult, AttrError, errors.New(in)) })
		if got[AttrError] != want {
			t.Errorf("error %q logged as %q, want %q", in, got[AttrError], want)
		}
	}
}

func TestQueryParametersAreLogged(t *testing.T) {
	got := logLine(t, func(l *slog.Logger) {
		l.Info(EventSQLStart, AttrQuery, "UPDATE t SET a = 1 WHERE id > $1 AND id <= $2", AttrParams, []int64{0, 100})
	})
	params, _ := got[AttrParams].([]any)
	if len(params) != 2 || params[1] != float64(100) {
		t.Errorf("params = %v, want the batch bounds", got[AttrParams])
	}
}

func TestQueryLiteralsAreMasked(t *testing.T) {
	got := logLine(t, func(l *slog.Logger) {
		l.Info(EventSQLStart, AttrQuery, "UPDATE users SET pw = 'hunter2', note = $$secret$$ WHERE name = 'o''neil' -- token abc")
	})
	q, _ := got[AttrQuery].(string)
	if strings.Contains(q, "hunter2") || strings.Contains(q, "secret") || strings.Contains(q, "neil") || strings.Contains(q, "abc") {
		t.Errorf("query still has literals or comments: %q", q)
	}
	if !strings.Contains(q, "UPDATE users SET pw = '?'") {
		t.Errorf("query lost its shape: %q", q)
	}
}

func TestMaskSQLKeepsIdentifiersAndParameters(t *testing.T) {
	cases := map[string]string{
		`SELECT "Weird 'name'" FROM t WHERE id > $1 AND id <= $2`: `SELECT "Weird 'name'" FROM t WHERE id > $1 AND id <= $2`,
		`SELECT 'a', 'b'`:                       `SELECT '?', '?'`,
		`DO $body$ BEGIN PERFORM 1; END $body$`: `DO '?'`,
		`SELECT 1 /* x */ + 2`:                  `SELECT 1   + 2`,
		`SELECT 7`:                              `SELECT 7`, // numbers are not masked (documented limitation)
	}
	for in, want := range cases {
		if got := MaskSQL(in); got != want {
			t.Errorf("MaskSQL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLogLineHasTheFixedAttributes(t *testing.T) {
	rec := useRecorder(t)
	ctx, span := Start(context.Background(), SpanAPIApprove)
	ctx = WithCorrelationID(ctx, "abc123")
	got := logLine(t, func(l *slog.Logger) {
		l.InfoContext(ctx, EventExecutionApproved, AttrActorID, 5, AttrActor, "sari", AttrDatabaseID, 2, AttrStatus, "approved")
	})
	span.End()

	for _, key := range []string{"time", "level", AttrEvent, AttrActorID, AttrActor, AttrDatabaseID, AttrStatus, AttrCorrelationID, AttrTraceID, AttrSpanID} {
		if _, ok := got[key]; !ok {
			t.Errorf("log line has no %q: %v", key, got)
		}
	}
	if got[AttrEvent] != EventExecutionApproved || got[AttrCorrelationID] != "abc123" {
		t.Errorf("event/correlation = %v / %v", got[AttrEvent], got[AttrCorrelationID])
	}
	ended := rec.Ended()
	if len(ended) != 1 || got[AttrTraceID] != ended[0].SpanContext().TraceID().String() || got[AttrSpanID] != ended[0].SpanContext().SpanID().String() {
		t.Errorf("trace ids in the log do not match the span")
	}
}

func TestLogLineWithoutSpanHasNoTraceIDs(t *testing.T) {
	useRecorder(t)
	got := logLine(t, func(l *slog.Logger) { l.InfoContext(context.Background(), EventHTTPRequest) })
	if _, ok := got[AttrTraceID]; ok {
		t.Errorf("trace_id without a span: %v", got)
	}
}

func TestTraceparentJoinsTheTrace(t *testing.T) {
	rec := useRecorder(t)
	pubCtx, pub := Start(context.Background(), SpanQueuePublish)
	tp := Traceparent(pubCtx)
	pub.End()
	if tp == "" {
		t.Fatal("no traceparent for an active span")
	}

	workerCtx := WithTraceparent(context.Background(), tp)
	_, work := Start(workerCtx, SpanWorkerProcess)
	work.End()

	ended := rec.Ended()
	if len(ended) != 2 {
		t.Fatalf("ended spans = %d", len(ended))
	}
	if ended[0].SpanContext().TraceID() != ended[1].SpanContext().TraceID() {
		t.Errorf("worker span is in another trace")
	}
	if ended[1].Parent().SpanID() != ended[0].SpanContext().SpanID() {
		t.Errorf("worker span is not a child of the publish span")
	}
}

func TestTraceparentEdgeCases(t *testing.T) {
	useRecorder(t)
	if got := Traceparent(context.Background()); got != "" {
		t.Errorf("traceparent without a span = %q", got)
	}
	ctx := WithTraceparent(context.Background(), "not-a-traceparent")
	_, s := Start(ctx, "x")
	defer s.End()
	if s.SpanContext().TraceID().String() == "" {
		t.Error("malformed traceparent broke span creation")
	}
}

func TestCorrelationIDs(t *testing.T) {
	a, b := NewCorrelationID(), NewCorrelationID()
	if a == b || len(a) != 16 {
		t.Errorf("ids = %q, %q", a, b)
	}
	if CorrelationID(context.Background()) != "" {
		t.Error("empty context has a correlation id")
	}
}

func TestStartBatchNamesTheBatch(t *testing.T) {
	rec := useRecorder(t)
	_, s := StartBatch(context.Background(), 3)
	s.End()
	if got := rec.Ended()[0].Name(); got != "batch[3]" {
		t.Errorf("name = %q", got)
	}
}

func TestMigrationVersionAttrs(t *testing.T) {
	version := 3
	got := MigrationVersionAttrs(&version, 2)
	want := []any{AttrMigrationRevision, 2, AttrMigrationVersion, 3}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("with a version = %v, want %v", got, want)
	}
	got = MigrationVersionAttrs(nil, 1)
	want = []any{AttrMigrationRevision, 1}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("without a version = %v, want %v", got, want)
	}
}
