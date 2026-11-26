package metrics

import (
	"context"
	"database/sql"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	dto "github.com/prometheus/client_model/go"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func TestPoolCollectorReportsEveryPoolFromItsSources(t *testing.T) {
	platform := func() []PoolStats {
		return []PoolStats{{Pool: "platform", Database: "platform", Stats: sql.DBStats{MaxOpenConnections: 10, InUse: 2, Idle: 3, WaitCount: 4, WaitDuration: 1500 * time.Millisecond}}}
	}
	targets := func() []PoolStats {
		return []PoolStats{{Pool: "target", Database: "toko", Stats: sql.DBStats{MaxOpenConnections: 5, InUse: 5, Idle: 0}}}
	}
	want := `
# HELP datanest_db_pool_connections Open connections by state: in_use or idle.
# TYPE datanest_db_pool_connections gauge
datanest_db_pool_connections{database="platform",pool="platform",state="idle"} 3
datanest_db_pool_connections{database="platform",pool="platform",state="in_use"} 2
datanest_db_pool_connections{database="toko",pool="target",state="idle"} 0
datanest_db_pool_connections{database="toko",pool="target",state="in_use"} 5
# HELP datanest_db_pool_max_open_connections Configured maximum of open connections (0 means unlimited).
# TYPE datanest_db_pool_max_open_connections gauge
datanest_db_pool_max_open_connections{database="platform",pool="platform"} 10
datanest_db_pool_max_open_connections{database="toko",pool="target"} 5
# HELP datanest_db_pool_wait_seconds_total Total time callers waited for a free connection.
# TYPE datanest_db_pool_wait_seconds_total counter
datanest_db_pool_wait_seconds_total{database="platform",pool="platform"} 1.5
datanest_db_pool_wait_seconds_total{database="toko",pool="target"} 0
# HELP datanest_db_pool_wait_total Times a caller had to wait for a free connection.
# TYPE datanest_db_pool_wait_total counter
datanest_db_pool_wait_total{database="platform",pool="platform"} 4
datanest_db_pool_wait_total{database="toko",pool="target"} 0
`
	if err := testutil.CollectAndCompare(NewPoolCollector(platform, targets), strings.NewReader(want)); err != nil {
		t.Error(err)
	}
}

func TestPoolCollectorReadsTheSourceAtEveryScrape(t *testing.T) {
	inUse := 1
	source := func() []PoolStats { return []PoolStats{{Pool: "target", Database: "toko", Stats: sql.DBStats{InUse: inUse}}} }
	c := NewPoolCollector(source)
	inUse = 4
	if got := testutil.CollectAndCount(c, namespace+"_db_pool_connections"); got != 2 {
		t.Fatalf("series = %d, want 2", got)
	}
	reg := prometheus.NewPedanticRegistry()
	reg.MustRegister(c)
	families, err := reg.Gather()
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range families {
		for _, m := range f.GetMetric() {
			if f.GetName() == namespace+"_db_pool_connections" && label(m, "state") == "in_use" && m.GetGauge().GetValue() != 4 {
				t.Errorf("in_use = %v, want the value at scrape time (4)", m.GetGauge().GetValue())
			}
		}
	}
}

func label(m *dto.Metric, name string) string {
	for _, l := range m.GetLabel() {
		if l.GetName() == name {
			return l.GetValue()
		}
	}
	return ""
}

func sampledContext(t *testing.T) (context.Context, string) {
	t.Helper()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSampler(sdktrace.AlwaysSample()))
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })
	ctx, span := tp.Tracer("test").Start(context.Background(), "op")
	t.Cleanup(func() { span.End() })
	return ctx, span.SpanContext().TraceID().String()
}

func exemplarOf(t *testing.T, h prometheus.Histogram) *dto.Exemplar {
	t.Helper()
	var m dto.Metric
	if err := h.Write(&m); err != nil {
		t.Fatal(err)
	}
	for _, b := range m.GetHistogram().GetBucket() {
		if b.GetExemplar() != nil {
			return b.GetExemplar()
		}
	}
	return nil
}

func TestObserveAttachesTheTraceIDAsAnExemplar(t *testing.T) {
	ctx, traceID := sampledContext(t)
	h := prometheus.NewHistogram(prometheus.HistogramOpts{Name: "h", Buckets: durationBuckets})

	Observe(ctx, h, 0.3)

	ex := exemplarOf(t, h)
	if ex == nil || len(ex.GetLabel()) != 1 || ex.GetLabel()[0].GetName() != "trace_id" || ex.GetLabel()[0].GetValue() != traceID {
		t.Errorf("exemplar = %v, want trace_id=%s", ex, traceID)
	}
}

func TestObserveWithoutATraceRecordsNoExemplar(t *testing.T) {
	h := prometheus.NewHistogram(prometheus.HistogramOpts{Name: "h", Buckets: durationBuckets})

	Observe(context.Background(), h, 0.3)

	if ex := exemplarOf(t, h); ex != nil {
		t.Errorf("exemplar = %v, want none", ex)
	}
	if got := testutil.CollectAndCount(h); got != 1 {
		t.Errorf("series = %d, want 1", got)
	}
}

func TestObserveHTTPKeepsLabelsBounded(t *testing.T) {
	before := testutil.ToFloat64(HTTPRequests.WithLabelValues("OTHER", "/api/x", "405"))

	ObserveHTTP(context.Background(), "BREW", "/api/x", 405, time.Millisecond)
	ObserveHTTP(context.Background(), "PROPFIND", "/api/x", 405, time.Millisecond)

	if got := testutil.ToFloat64(HTTPRequests.WithLabelValues("OTHER", "/api/x", "405")) - before; got != 2 {
		t.Errorf("OTHER requests = %v, want 2: unknown methods must share one label value", got)
	}
}

func TestHandlerServesOpenMetricsWithExemplars(t *testing.T) {
	ctx, traceID := sampledContext(t)
	Observe(ctx, MigrationDuration.WithLabelValues("run", "normal", "success"), 1.2)

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	req.Header.Set("Accept", "application/openmetrics-text; version=1.0.0")
	rec := httptest.NewRecorder()
	Handler().ServeHTTP(rec, req)

	body, _ := io.ReadAll(rec.Body)
	text := string(body)
	if !strings.HasPrefix(rec.Header().Get("Content-Type"), "application/openmetrics-text") {
		t.Errorf("content type = %q, want OpenMetrics", rec.Header().Get("Content-Type"))
	}
	if !strings.Contains(text, `datanest_migration_duration_seconds_bucket{kind="run",mode="normal",outcome="success"`) {
		t.Error("migration duration histogram missing from /metrics")
	}
	if !strings.Contains(text, `# {trace_id="`+traceID+`"}`) {
		t.Error("exemplar with the trace id missing from /metrics")
	}
	if !strings.Contains(text, "go_goroutines") || !strings.Contains(text, "process_") {
		t.Error("runtime and process metrics missing")
	}
}

func TestServeStopsWhenTheContextEnds(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- Serve(ctx, "127.0.0.1:0") }()
	time.Sleep(50 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Serve = %v, want nil after the context ends", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Serve did not stop")
	}
}

func TestServeReportsAnAddressInUse(t *testing.T) {
	l := httptest.NewServer(http.NotFoundHandler())
	defer l.Close()
	addr := strings.TrimPrefix(l.URL, "http://")
	if err := Serve(context.Background(), addr); err == nil {
		t.Error("Serve on a taken address returned nil")
	}
}

func gaugeValue(t *testing.T, c prometheus.Collector, name string) float64 {
	t.Helper()
	reg := prometheus.NewPedanticRegistry()
	reg.MustRegister(c)
	families, err := reg.Gather()
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range families {
		if f.GetName() == name && len(f.GetMetric()) == 1 {
			return f.GetMetric()[0].GetGauge().GetValue()
		}
	}
	t.Fatalf("%s not found", name)
	return 0
}
