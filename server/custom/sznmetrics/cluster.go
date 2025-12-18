// Copyright (c) 2024-present Seznam.cz, a.s.
// All Rights Reserved.
//
// This file is part of custom extensions to Mattermost server
// developed by Seznam.cz, a.s. for internal use only.

package sznmetrics

import (
	"github.com/mattermost/mattermost/server/public/model"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	clusterRequestsDuration prometheus.Histogram
	clusterRequestsCounter  prometheus.Counter
	clusterEventTypeCounter *prometheus.CounterVec
)

func (m *SznMetrics) initClusterMetrics() {
	clusterRequestsDuration = prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace:   MetricsNamespace,
		Subsystem:   MetricsSubsystemCluster,
		Name:        "request_duration_seconds",
		Help:        "Time taken for cluster requests in seconds.",
		ConstLabels: m.additionalLabels,
		Buckets:     []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1},
	})
	m.Registry.MustRegister(clusterRequestsDuration)

	clusterRequestsCounter = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace:   MetricsNamespace,
		Subsystem:   MetricsSubsystemCluster,
		Name:        "requests_total",
		Help:        "The total number of cluster requests.",
		ConstLabels: m.additionalLabels,
	})
	m.Registry.MustRegister(clusterRequestsCounter)

	clusterEventTypeCounter = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace:   MetricsNamespace,
		Subsystem:   MetricsSubsystemCluster,
		Name:        "event_type_totals",
		Help:        "The total number of cluster events by type.",
		ConstLabels: m.additionalLabels,
	}, []string{"type"})
	m.Registry.MustRegister(clusterEventTypeCounter)
}

// IncrementClusterRequest increments the cluster request counter
func (m *SznMetrics) IncrementClusterRequest() {
	if clusterRequestsCounter != nil {
		clusterRequestsCounter.Inc()
	}
}

// ObserveClusterRequestDuration observes cluster request duration
func (m *SznMetrics) ObserveClusterRequestDuration(elapsed float64) {
	if clusterRequestsDuration != nil {
		clusterRequestsDuration.Observe(elapsed)
	}
}

// IncrementClusterEventType increments the cluster event type counter
func (m *SznMetrics) IncrementClusterEventType(eventType model.ClusterEvent) {
	if clusterEventTypeCounter != nil {
		clusterEventTypeCounter.With(prometheus.Labels{"type": string(eventType)}).Inc()
	}
}
