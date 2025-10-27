package storage

import "github.com/zombar/purpletab/pkg/metrics"

// MetricsAdapter adapts businessMetrics to the storage.BusinessMetrics interface
type MetricsAdapter struct {
	metrics *metrics.BusinessMetrics
}

// NewMetricsAdapter creates a new metrics adapter
func NewMetricsAdapter(m *metrics.BusinessMetrics) *MetricsAdapter {
	return &MetricsAdapter{metrics: m}
}

// RecordTombstone records a tombstone creation metric
func (a *MetricsAdapter) RecordTombstone(reason, tag string, periodDays int) {
	if a.metrics == nil {
		return
	}
	a.metrics.TombstonesCreatedTotal.WithLabelValues(reason, tag).Inc()
	a.metrics.TombstoneDaysHistogram.WithLabelValues(reason).Observe(float64(periodDays))
}
