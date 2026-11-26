package worker

import (
	"context"
	"time"

	"go.opentelemetry.io/otel/trace"

	"github.com/AthillaZaidan/datanest/backend/internal/metrics"
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
