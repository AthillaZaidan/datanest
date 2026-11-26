package worker

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/AthillaZaidan/datanest/backend/internal/metrics"
)

func TestInitMetricsStartsEveryKnownSeriesAtZero(t *testing.T) {
	InitMetrics()

	if got := testutil.CollectAndCount(metrics.JobAttempts); got < 3*2*len(attemptOutcomes) {
		t.Errorf("attempt series = %d, want every kind, mode and outcome (%d)", got, 3*2*len(attemptOutcomes))
	}
	if got := testutil.CollectAndCount(metrics.MigrationDuration); got < 3*2*len(attemptOutcomes) {
		t.Errorf("duration series = %d, want %d", got, 3*2*len(attemptOutcomes))
	}
	if got := testutil.CollectAndCount(metrics.JobClaims); got != 5 {
		t.Errorf("claim series = %d, want 5", got)
	}
	if got := testutil.CollectAndCount(metrics.LockAcquire); got != 2 {
		t.Errorf("lock acquire series = %d, want 2", got)
	}
	if got := testutil.ToFloat64(metrics.JobAttempts.WithLabelValues("forward", "batch", "partial")); got != 0 {
		t.Errorf("an unused series = %v, want 0", got)
	}
}
