package metrics

import (
	"database/sql"

	"github.com/prometheus/client_golang/prometheus"
)

type PoolStats struct {
	Pool     string
	Database string
	Stats    sql.DBStats
}

type PoolSource func() []PoolStats

var (
	poolLabels   = []string{"pool", "database"}
	poolConns    = prometheus.NewDesc(namespace+"_db_pool_connections", "Open connections by state: in_use or idle.", append(poolLabels, "state"), nil)
	poolMaxOpen  = prometheus.NewDesc(namespace+"_db_pool_max_open_connections", "Configured maximum of open connections (0 means unlimited).", poolLabels, nil)
	poolWaits    = prometheus.NewDesc(namespace+"_db_pool_wait_total", "Times a caller had to wait for a free connection.", poolLabels, nil)
	poolWaitTime = prometheus.NewDesc(namespace+"_db_pool_wait_seconds_total", "Total time callers waited for a free connection.", poolLabels, nil)
)

type poolCollector struct{ sources []PoolSource }

func NewPoolCollector(sources ...PoolSource) prometheus.Collector {
	return poolCollector{sources: sources}
}

func (c poolCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- poolConns
	ch <- poolMaxOpen
	ch <- poolWaits
	ch <- poolWaitTime
}

func (c poolCollector) Collect(ch chan<- prometheus.Metric) {
	for _, source := range c.sources {
		for _, p := range source() {
			s := p.Stats
			ch <- prometheus.MustNewConstMetric(poolConns, prometheus.GaugeValue, float64(s.InUse), p.Pool, p.Database, "in_use")
			ch <- prometheus.MustNewConstMetric(poolConns, prometheus.GaugeValue, float64(s.Idle), p.Pool, p.Database, "idle")
			ch <- prometheus.MustNewConstMetric(poolMaxOpen, prometheus.GaugeValue, float64(s.MaxOpenConnections), p.Pool, p.Database)
			ch <- prometheus.MustNewConstMetric(poolWaits, prometheus.CounterValue, float64(s.WaitCount), p.Pool, p.Database)
			ch <- prometheus.MustNewConstMetric(poolWaitTime, prometheus.CounterValue, s.WaitDuration.Seconds(), p.Pool, p.Database)
		}
	}
}

func SinglePool(pool, database string, db *sql.DB) PoolSource {
	return func() []PoolStats {
		return []PoolStats{{Pool: pool, Database: database, Stats: db.Stats()}}
	}
}
