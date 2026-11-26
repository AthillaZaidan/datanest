package metrics

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel/trace"
)

const namespace = "datanest"

var (
	durationBuckets = []float64{0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 20, 30, 60, 120, 300}
	waitBuckets     = []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 20, 30, 60, 120, 300}
	quickBuckets    = []float64{0.001, 0.0025, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5}
)

var Registry = prometheus.NewRegistry()

var (
	JobsInProgress = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace, Name: "worker_jobs_in_progress",
		Help: "Executions this worker is running now.",
	}, []string{"kind", "mode"})

	JobAttempts = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace, Name: "job_attempts_total",
		Help: "Finished attempts by outcome: success, failed, partial, uncertain, retry (requeued for another attempt) or lock_lost.",
	}, []string{"kind", "mode", "outcome"})

	JobClaims = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace, Name: "job_claims_total",
		Help: "Queue messages handled by outcome: claimed, lock_busy (database locked by another execution), held (an unsettled execution is in the way), skipped or error.",
	}, []string{"outcome"})

	QueueWait = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: namespace, Name: "job_queue_wait_seconds",
		Help:    "Time from queued to the first claim, measured with the platform database clock. Includes time spent waiting for the database lock or behind an unsettled execution.",
		Buckets: waitBuckets,
	}, []string{"kind", "mode"})

	LockAcquire = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: namespace, Name: "lock_acquire_seconds",
		Help:    "Duration of one attempt to take the per-database lock, by result.",
		Buckets: quickBuckets,
	}, []string{"result"})

	MigrationDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: namespace, Name: "migration_duration_seconds",
		Help:    "Duration of one attempt from claim to recorded result.",
		Buckets: durationBuckets,
	}, []string{"kind", "mode", "outcome"})

	BatchDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: namespace, Name: "migration_batch_duration_seconds",
		Help:    "Duration of one committed target transaction (a batch, or the whole run in normal mode).",
		Buckets: durationBuckets,
	}, []string{"kind", "mode"})

	BatchesCommitted = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace, Name: "migration_batches_committed_total",
		Help: "Committed target transactions; normal mode commits one per run.",
	}, []string{"kind", "mode"})

	RowsProcessed = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace, Name: "migration_rows_processed_total",
		Help: "Rows changed by committed target transactions.",
	}, []string{"kind", "mode"})

	HTTPRequests = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace, Name: "http_requests_total",
		Help: "API requests by method, route template and status code.",
	}, []string{"method", "route", "code"})

	HTTPDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: namespace, Name: "http_request_duration_seconds",
		Help:    "API request duration by method and route template.",
		Buckets: quickBuckets,
	}, []string{"method", "route"})
)

func init() {
	Registry.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		JobsInProgress, JobAttempts, JobClaims, QueueWait, LockAcquire,
		MigrationDuration, BatchDuration, BatchesCommitted, RowsProcessed,
		HTTPRequests, HTTPDuration,
	)
}

func Observe(ctx context.Context, o prometheus.Observer, seconds float64) {
	sc := trace.SpanContextFromContext(ctx)
	if eo, ok := o.(prometheus.ExemplarObserver); ok && sc.IsSampled() {
		eo.ObserveWithExemplar(seconds, prometheus.Labels{"trace_id": sc.TraceID().String()})
		return
	}
	o.Observe(seconds)
}

func Since(ctx context.Context, o prometheus.Observer, start time.Time) {
	Observe(ctx, o, time.Since(start).Seconds())
}

func Handler() http.Handler {
	return promhttp.HandlerFor(Registry, promhttp.HandlerOpts{EnableOpenMetrics: true, Registry: Registry})
}

func Start(ctx context.Context, addr string) {
	if addr == "" {
		return
	}
	go func() {
		if err := Serve(ctx, addr); err != nil {
			slog.Error("metrics.serve_failed", "addr", addr, "error", err.Error())
		}
	}()
}

func Serve(ctx context.Context, addr string) error {
	mux := http.NewServeMux()
	mux.Handle("GET /metrics", Handler())
	srv := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	errc := make(chan error, 1)
	go func() {
		slog.Info("metrics.listening", "addr", addr)
		errc <- srv.ListenAndServe()
	}()
	select {
	case err := <-errc:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}
