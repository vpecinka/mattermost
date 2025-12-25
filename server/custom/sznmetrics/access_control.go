// Copyright (c) 2024-present Seznam.cz, a.s.
// All Rights Reserved.
//
// This file is part of custom extensions to Mattermost server
// developed by Seznam.cz, a.s. for internal use only.

package sznmetrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

func (m *SznMetrics) initAccessControlMetrics() {
	m.accessControlSearchQueryDuration = prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace:   MetricsNamespace,
		Subsystem:   MetricsSubsystemAccessControl,
		Name:        "search_query_duration_seconds",
		Help:        "Time taken for access control search query in seconds.",
		ConstLabels: m.additionalLabels,
		Buckets:     []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1},
	})
	m.Registry.MustRegister(m.accessControlSearchQueryDuration)

	m.accessControlExpressionCompileDuration = prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace:   MetricsNamespace,
		Subsystem:   MetricsSubsystemAccessControl,
		Name:        "expression_compile_duration_seconds",
		Help:        "Time taken for access control expression compile in seconds.",
		ConstLabels: m.additionalLabels,
		Buckets:     []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1},
	})
	m.Registry.MustRegister(m.accessControlExpressionCompileDuration)

	m.accessControlEvaluateDuration = prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace:   MetricsNamespace,
		Subsystem:   MetricsSubsystemAccessControl,
		Name:        "evaluate_duration_seconds",
		Help:        "Time taken for access control evaluation in seconds.",
		ConstLabels: m.additionalLabels,
		Buckets:     []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1},
	})
	m.Registry.MustRegister(m.accessControlEvaluateDuration)

	m.accessControlCacheInvalidation = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace:   MetricsNamespace,
		Subsystem:   MetricsSubsystemAccessControl,
		Name:        "cache_invalidation_total",
		Help:        "The total number of access control cache invalidations.",
		ConstLabels: m.additionalLabels,
	})
	m.Registry.MustRegister(m.accessControlCacheInvalidation)
}

// ObserveAccessControlSearchQueryDuration observes access control search query duration
func (m *SznMetrics) ObserveAccessControlSearchQueryDuration(value float64) {
	if m.accessControlSearchQueryDuration != nil {
		m.accessControlSearchQueryDuration.Observe(value)
	}
}

// ObserveAccessControlExpressionCompileDuration observes access control expression compile duration
func (m *SznMetrics) ObserveAccessControlExpressionCompileDuration(value float64) {
	if m.accessControlExpressionCompileDuration != nil {
		m.accessControlExpressionCompileDuration.Observe(value)
	}
}

// ObserveAccessControlEvaluateDuration observes access control evaluate duration
func (m *SznMetrics) ObserveAccessControlEvaluateDuration(value float64) {
	if m.accessControlEvaluateDuration != nil {
		m.accessControlEvaluateDuration.Observe(value)
	}
}

// IncrementAccessControlCacheInvalidation increments access control cache invalidation counter
func (m *SznMetrics) IncrementAccessControlCacheInvalidation() {
	if m.accessControlCacheInvalidation != nil {
		m.accessControlCacheInvalidation.Inc()
	}
}
