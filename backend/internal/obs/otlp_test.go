package obs

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"go.opentelemetry.io/otel"
	collogs "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	coltrace "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	logspb "go.opentelemetry.io/proto/otlp/logs/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/proto"
)

type collector struct {
	mu    sync.Mutex
	logs  []*logspb.ResourceLogs
	spans []*tracepb.ResourceSpans
}

func (c *collector) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	c.mu.Lock()
	defer c.mu.Unlock()
	switch r.URL.Path {
	case "/v1/logs":
		var req collogs.ExportLogsServiceRequest
		if err := proto.Unmarshal(body, &req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		c.logs = append(c.logs, req.GetResourceLogs()...)
		out, _ := proto.Marshal(&collogs.ExportLogsServiceResponse{})
		w.Header().Set("Content-Type", "application/x-protobuf")
		w.Write(out)
	case "/v1/traces":
		var req coltrace.ExportTraceServiceRequest
		if err := proto.Unmarshal(body, &req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		c.spans = append(c.spans, req.GetResourceSpans()...)
		out, _ := proto.Marshal(&coltrace.ExportTraceServiceResponse{})
		w.Header().Set("Content-Type", "application/x-protobuf")
		w.Write(out)
	default:
		http.NotFound(w, r)
	}
}

func resourceAttr(r *resourcepb.Resource, key string) string {
	for _, kv := range r.GetAttributes() {
		if kv.GetKey() == key {
			return kv.GetValue().GetStringValue()
		}
	}
	return ""
}

func attrValue(attrs []*commonpb.KeyValue, key string) (string, bool) {
	for _, kv := range attrs {
		if kv.GetKey() == key {
			return kv.GetValue().GetStringValue(), true
		}
	}
	return "", false
}

func restoreGlobals(t *testing.T) {
	t.Helper()
	prevLogger, prevTracer, prevPropagator := slog.Default(), otel.GetTracerProvider(), otel.GetTextMapPropagator()
	t.Cleanup(func() {
		slog.SetDefault(prevLogger)
		otel.SetTracerProvider(prevTracer)
		otel.SetTextMapPropagator(prevPropagator)
	})
}

func TestSetupShipsMaskedLogsAndSpansOverOTLP(t *testing.T) {
	restoreGlobals(t)
	c := &collector{}
	srv := httptest.NewServer(c)
	defer srv.Close()
	var stdout bytes.Buffer

	shutdown, err := Setup(Options{Level: "info", TraceExporter: "none", OTLPEndpoint: srv.URL + "/",
		Service: "datanest-worker", Instance: "host-42", LogOut: &stdout})
	if err != nil {
		t.Fatal(err)
	}
	ctx, span := Start(WithCorrelationID(context.Background(), "corr-1"), SpanWorkerProcess)
	slog.InfoContext(ctx, EventSQLStart, "password", "hunter2", AttrQuery, "UPDATE t SET name = 'Budi'",
		AttrError, "dial postgres://app:hunter2@db:5432/toko", slog.Group("conn", "token", "abc"))
	slog.DebugContext(ctx, "debug.hidden")
	traceID := span.SpanContext().TraceID()
	span.End()
	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown: %v", err)
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.logs) == 0 || len(c.spans) == 0 {
		t.Fatalf("collector got %d log batches and %d span batches, want both", len(c.logs), len(c.spans))
	}
	if got := resourceAttr(c.logs[0].GetResource(), "service.name"); got != "datanest-worker" {
		t.Errorf("log service.name = %q", got)
	}
	if got := resourceAttr(c.spans[0].GetResource(), "service.instance.id"); got != "host-42" {
		t.Errorf("span service.instance.id = %q", got)
	}

	var records []*logspb.LogRecord
	for _, rl := range c.logs {
		for _, sl := range rl.GetScopeLogs() {
			records = append(records, sl.GetLogRecords()...)
		}
	}
	if len(records) != 1 {
		t.Fatalf("records = %d, want 1 (debug is below the level)", len(records))
	}
	r := records[0]
	if r.GetBody().GetStringValue() != EventSQLStart {
		t.Errorf("body = %q, want the event name", r.GetBody().GetStringValue())
	}
	if !bytes.Equal(r.GetTraceId(), traceID[:]) {
		t.Errorf("log trace id = %x, want %s", r.GetTraceId(), traceID)
	}
	if v, _ := attrValue(r.GetAttributes(), "password"); v != Redacted {
		t.Errorf("password = %q, want %s", v, Redacted)
	}
	if v, _ := attrValue(r.GetAttributes(), AttrQuery); strings.Contains(v, "Budi") {
		t.Errorf("query literal leaked: %q", v)
	}
	if v, _ := attrValue(r.GetAttributes(), AttrError); strings.Contains(v, "hunter2") {
		t.Errorf("error leaked the password: %q", v)
	}
	if v, _ := attrValue(r.GetAttributes(), AttrCorrelationID); v != "corr-1" {
		t.Errorf("correlation_id = %q", v)
	}
	for _, kv := range r.GetAttributes() {
		if kv.GetKey() == "conn" {
			for _, m := range kv.GetValue().GetKvlistValue().GetValues() {
				if m.GetKey() == "token" && m.GetValue().GetStringValue() != Redacted {
					t.Errorf("grouped token = %q, want %s", m.GetValue().GetStringValue(), Redacted)
				}
			}
		}
	}
	if strings.Contains(stdout.String(), "hunter2") || !strings.Contains(stdout.String(), EventSQLStart) {
		t.Errorf("stdout must still get the masked JSON line: %s", stdout.String())
	}
}

func TestSetupWithoutAnEndpointKeepsLogsOnStdoutOnly(t *testing.T) {
	restoreGlobals(t)
	var stdout bytes.Buffer
	shutdown, err := Setup(Options{Level: "info", TraceExporter: "none", LogOut: &stdout})
	if err != nil {
		t.Fatal(err)
	}
	defer shutdown(context.Background())

	if _, ok := slog.Default().Handler().(fanout); ok {
		t.Error("handler fans out although no OTLP endpoint is set")
	}
	slog.Info(EventAuthLogin, "password", "hunter2")
	if !strings.Contains(stdout.String(), Redacted) {
		t.Errorf("stdout = %s", stdout.String())
	}
}

func TestSetupRejectsAnUnknownExporter(t *testing.T) {
	restoreGlobals(t)
	if _, err := Setup(Options{Level: "info", TraceExporter: "jaeger"}); err == nil {
		t.Error("Setup accepted an unknown trace exporter")
	}
}

func TestMaskingCoversAttributesAddedWithWith(t *testing.T) {
	var got []slog.Attr
	h := masking{captureHandler{&got}}
	slog.New(h).With("session_id", "s-1").WithGroup("g").Info("x", "csrf", "c-1")
	for _, a := range got {
		if a.Value.String() == "s-1" || a.Value.String() == "c-1" {
			t.Errorf("attribute %s leaked: %v", a.Key, a.Value)
		}
	}
	if len(got) != 2 {
		t.Errorf("captured %d attributes, want 2", len(got))
	}
}

type captureHandler struct{ attrs *[]slog.Attr }

func (h captureHandler) Enabled(context.Context, slog.Level) bool { return true }
func (h captureHandler) Handle(_ context.Context, r slog.Record) error {
	r.Attrs(func(a slog.Attr) bool {
		*h.attrs = append(*h.attrs, a)
		return true
	})
	return nil
}
func (h captureHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	*h.attrs = append(*h.attrs, attrs...)
	return h
}
func (h captureHandler) WithGroup(string) slog.Handler { return h }
