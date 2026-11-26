package metrics

import (
	"context"
	"log/slog"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"gorm.io/gorm"

	"github.com/AthillaZaidan/datanest/backend/internal/models"
)

var (
	executionsByStatus = prometheus.NewDesc(namespace+"_executions", "Executions in the platform database by status (read at scrape time).", []string{"status"}, nil)
	oldestQueued       = prometheus.NewDesc(namespace+"_queue_oldest_wait_seconds", "Age of the oldest execution still waiting in the queue; 0 when none waits.", nil, nil)
	executionsUp       = prometheus.NewDesc(namespace+"_executions_scrape_ok", "1 when the execution counts above were read from the platform database, 0 when the read failed.", nil, nil)
)

var ExecutionStatuses = []models.ExecutionStatus{
	models.ExecPendingReview, models.ExecApproved, models.ExecQueued, models.ExecRunning, models.ExecSuccess,
	models.ExecFailed, models.ExecPartial, models.ExecUncertain, models.ExecRejected, models.ExecCanceled,
}

type executionCollector struct {
	db      *gorm.DB
	timeout time.Duration
}

func NewExecutionCollector(db *gorm.DB) prometheus.Collector {
	return executionCollector{db: db, timeout: 3 * time.Second}
}

func (c executionCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- executionsByStatus
	ch <- oldestQueued
	ch <- executionsUp
}

func (c executionCollector) Collect(ch chan<- prometheus.Metric) {
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()
	counts, oldest, err := c.read(ctx)
	if err != nil {
		slog.Warn("metrics.executions_read_failed", "error", err.Error())
		ch <- prometheus.MustNewConstMetric(executionsUp, prometheus.GaugeValue, 0)
		return
	}
	for _, s := range ExecutionStatuses {
		ch <- prometheus.MustNewConstMetric(executionsByStatus, prometheus.GaugeValue, float64(counts[string(s)]), string(s))
	}
	ch <- prometheus.MustNewConstMetric(oldestQueued, prometheus.GaugeValue, oldest)
	ch <- prometheus.MustNewConstMetric(executionsUp, prometheus.GaugeValue, 1)
}

func (c executionCollector) read(ctx context.Context) (map[string]int64, float64, error) {
	var rows []struct {
		Status string
		N      int64
	}
	db := c.db.WithContext(ctx)
	if err := db.Raw(`SELECT status, count(*) AS n FROM migration_executions GROUP BY status`).Scan(&rows).Error; err != nil {
		return nil, 0, err
	}
	counts := make(map[string]int64, len(rows))
	for _, r := range rows {
		counts[r.Status] = r.N
	}
	var oldest float64
	err := db.Raw(`SELECT COALESCE(EXTRACT(EPOCH FROM now() - min(queued_at)), 0)::float8 FROM migration_executions WHERE status = ?`,
		models.ExecQueued).Scan(&oldest).Error
	return counts, oldest, err
}
