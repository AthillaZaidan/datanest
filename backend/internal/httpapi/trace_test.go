package httpapi

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/AthillaZaidan/datanest/backend/internal/obs"
)

func recordSpans(t *testing.T) *tracetest.SpanRecorder {
	t.Helper()
	if _, err := obs.SetupTracing(nil); err != nil {
		t.Fatal(err)
	}
	rec := tracetest.NewSpanRecorder()
	prev := otel.GetTracerProvider()
	otel.SetTracerProvider(sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec)))
	t.Cleanup(func() { otel.SetTracerProvider(prev) })
	return rec
}

func TestARequestThatChangesStateStartsATraceThatItsLogsAndSpansShare(t *testing.T) {
	rec := recordSpans(t)
	lines := captureLogs(t)
	var inner sdktrace.ReadOnlySpan
	r := NewRouter(Deps{RequireAuth: func(c *gin.Context) {
		_, s := obs.Start(c.Request.Context(), "inner")
		s.End()
		c.Next()
	}})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("POST", "/api/executions/7/approve", nil))

	var request sdktrace.ReadOnlySpan
	for _, s := range rec.Ended() {
		switch s.Name() {
		case "POST /api/executions/:executionId/approve":
			request = s
		case "inner":
			inner = s
		}
	}
	if request == nil || inner == nil {
		t.Fatalf("spans = %d, want the request span and the inner span", len(rec.Ended()))
	}
	if inner.Parent().SpanID() != request.SpanContext().SpanID() {
		t.Error("a span started in the handler is not a child of the request span")
	}
	line := firstEvent(lines(), obs.EventHTTPRequest)
	if line == nil || line[obs.AttrTraceID] != request.SpanContext().TraceID().String() || line[obs.AttrSpanID] != request.SpanContext().SpanID().String() {
		t.Errorf("http.request line = %v, want the trace and span of the request", line)
	}
	if line[obs.AttrCorrelationID] != w.Header().Get(RequestIDHeader) {
		t.Errorf("correlation id %v does not match X-Request-ID %q", line[obs.AttrCorrelationID], w.Header().Get(RequestIDHeader))
	}
}

func TestAReadStartsNoSpan(t *testing.T) {
	rec := recordSpans(t)
	lines := captureLogs(t)
	r := NewRouter(Deps{})
	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/api/healthz", nil))

	if len(rec.Ended()) != 0 {
		t.Errorf("a GET produced %d spans, want none", len(rec.Ended()))
	}
	if line := firstEvent(lines(), obs.EventHTTPRequest); line == nil || line[obs.AttrTraceID] != nil {
		t.Errorf("http.request line = %v, want no trace_id", line)
	}
}

func TestAServerErrorMarksTheRequestSpanAsAnError(t *testing.T) {
	rec := recordSpans(t)
	r := NewRouter(Deps{RequireAuth: passthrough, RequireAdmin: passthrough})
	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", "/api/executions/7/approve", nil)) // 501: no service wired

	spans := rec.Ended()
	if len(spans) != 1 || spans[0].Status().Code != codes.Error {
		t.Fatalf("spans = %v, want one span with status Error", spans)
	}
}

func TestAnUnknownRouteStillGetsANamedSpan(t *testing.T) {
	rec := recordSpans(t)
	r := NewRouter(Deps{})
	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", "/nowhere", nil))
	if spans := rec.Ended(); len(spans) != 1 || spans[0].Name() != "POST (no route)" {
		t.Errorf("spans = %v", spans)
	}
}
