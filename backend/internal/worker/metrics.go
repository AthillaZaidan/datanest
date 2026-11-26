package worker

import (
	"context"
	"time"

	"go.opentelemetry.io/otel/trace"

	"github.com/AthillaZaidan/datanest/backend/internal/metrics"
	"github.com/AthillaZaidan/datanest/backend/internal/models"
)

const (
	outcomeSuccess   = "success"
	outcomeUncertain = "uncertain"
	outcomeRetry     = "retry"
	outcomeLockLost  = "lock_lost"
)

var claimOutcomes = map[ClaimResult]string{
	ClaimNone: "skipped",
	ClaimBusy: "lock_busy",
	ClaimHeld: "held",
	ClaimOK:   "claimed",
}

func recordClaim(ctx context.Context, cl *Claimed, result ClaimResult, err error) {
	if err != nil {
		metrics.JobClaims.WithLabelValues("error").Inc()
		return
	}
	metrics.JobClaims.WithLabelValues(claimOutcomes[result]).Inc()
	if result != ClaimOK || cl.Exec.Attempts != 1 || cl.Exec.QueuedAt == nil || cl.Exec.StartedAt == nil {
		return
	}
	if cl.span != nil {
		ctx = trace.ContextWithSpan(ctx, cl.span)
	}
	wait := cl.Exec.StartedAt.Sub(*cl.Exec.QueuedAt)
	metrics.Observe(ctx, metrics.QueueWait.WithLabelValues(jobLabels(cl)...), max(wait, 0).Seconds())
}

func lockResult(acquired bool) string {
	if acquired {
		return "acquired"
	}
	return "busy"
}

func jobLabels(cl *Claimed) []string {
	return []string{string(cl.Exec.Kind), string(cl.Exec.Mode)}
}

func recordAttempt(ctx context.Context, cl *Claimed, started time.Time) {
	if cl.outcome == "" {
		return
	}
	kind, mode := string(cl.Exec.Kind), string(cl.Exec.Mode)
	metrics.JobAttempts.WithLabelValues(kind, mode, cl.outcome).Inc()
	metrics.Since(ctx, metrics.MigrationDuration.WithLabelValues(kind, mode, cl.outcome), started)
}

func recordCommit(ctx context.Context, cl *Claimed, rows int64, started time.Time) {
	labels := jobLabels(cl)
	metrics.BatchesCommitted.WithLabelValues(labels...).Inc()
	metrics.RowsProcessed.WithLabelValues(labels...).Add(float64(rows))
	metrics.Since(ctx, metrics.BatchDuration.WithLabelValues(labels...), started)
}

func trackInProgress(cl *Claimed) func() {
	g := metrics.JobsInProgress.WithLabelValues(jobLabels(cl)...)
	g.Inc()
	return g.Dec
}

var (
	metricKinds     = []models.ExecutionKind{models.ExecKindRun, models.ExecKindForward, models.ExecKindRollback}
	metricModes     = []models.ExecutionMode{models.ExecModeNormal, models.ExecModeBatch}
	attemptOutcomes = []string{outcomeSuccess, string(models.ExecFailed), string(models.ExecPartial), outcomeUncertain, outcomeRetry, outcomeLockLost}
)

func InitMetrics() {
	for _, outcome := range claimOutcomes {
		metrics.JobClaims.WithLabelValues(outcome)
	}
	metrics.JobClaims.WithLabelValues("error")
	metrics.LockAcquire.WithLabelValues(lockResult(true))
	metrics.LockAcquire.WithLabelValues(lockResult(false))
	for _, k := range metricKinds {
		for _, m := range metricModes {
			kind, mode := string(k), string(m)
			metrics.JobsInProgress.WithLabelValues(kind, mode)
			metrics.QueueWait.WithLabelValues(kind, mode)
			metrics.BatchDuration.WithLabelValues(kind, mode)
			metrics.BatchesCommitted.WithLabelValues(kind, mode)
			metrics.RowsProcessed.WithLabelValues(kind, mode)
			for _, outcome := range attemptOutcomes {
				metrics.JobAttempts.WithLabelValues(kind, mode, outcome)
				metrics.MigrationDuration.WithLabelValues(kind, mode, outcome)
			}
		}
	}
}
