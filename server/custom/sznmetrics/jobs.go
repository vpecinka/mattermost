// Copyright (c) 2024-present Seznam.cz, a.s.
// All Rights Reserved.
//
// This file is part of custom extensions to Mattermost server
// developed by Seznam.cz, a.s. for internal use only.

package sznmetrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

var (
	jobActiveGauge     *prometheus.GaugeVec
	replicaLagAbsolute *prometheus.GaugeVec
	replicaLagTime     *prometheus.GaugeVec
	enabledUsersGauge  prometheus.Gauge
)

func (m *SznMetrics) initJobMetrics() {
	jobActiveGauge = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace:   MetricsNamespace,
		Subsystem:   MetricsSubsystemJobs,
		Name:        "active",
		Help:        "The number of active jobs by type.",
		ConstLabels: m.additionalLabels,
	}, []string{"type"})
	m.Registry.MustRegister(jobActiveGauge)

	replicaLagAbsolute = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace:   MetricsNamespace,
		Subsystem:   MetricsSubsystemDB,
		Name:        "replica_lag_abs",
		Help:        "Absolute replica lag in bytes.",
		ConstLabels: m.additionalLabels,
	}, []string{"node"})
	m.Registry.MustRegister(replicaLagAbsolute)

	replicaLagTime = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace:   MetricsNamespace,
		Subsystem:   MetricsSubsystemDB,
		Name:        "replica_lag_time",
		Help:        "Replica lag time in seconds.",
		ConstLabels: m.additionalLabels,
	}, []string{"node"})
	m.Registry.MustRegister(replicaLagTime)

	enabledUsersGauge = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace:   MetricsNamespace,
		Subsystem:   MetricsSubsystemSystem,
		Name:        "enabled_users",
		Help:        "The number of enabled users.",
		ConstLabels: m.additionalLabels,
	})
	m.Registry.MustRegister(enabledUsersGauge)
}

// IncrementJobActive increments active jobs gauge
func (m *SznMetrics) IncrementJobActive(jobType string) {
	if jobActiveGauge != nil {
		jobActiveGauge.With(prometheus.Labels{"type": jobType}).Inc()
	}
}

// DecrementJobActive decrements active jobs gauge
func (m *SznMetrics) DecrementJobActive(jobType string) {
	if jobActiveGauge != nil {
		jobActiveGauge.With(prometheus.Labels{"type": jobType}).Dec()
	}
}

// SetReplicaLagAbsolute sets replica lag absolute value
func (m *SznMetrics) SetReplicaLagAbsolute(node string, value float64) {
	if replicaLagAbsolute != nil {
		replicaLagAbsolute.With(prometheus.Labels{"node": node}).Set(value)
	}
}

// SetReplicaLagTime sets replica lag time value
func (m *SznMetrics) SetReplicaLagTime(node string, value float64) {
	if replicaLagTime != nil {
		replicaLagTime.With(prometheus.Labels{"node": node}).Set(value)
	}
}

// ObserveEnabledUsers observes enabled users count
func (m *SznMetrics) ObserveEnabledUsers(users int64) {
	if enabledUsersGauge != nil {
		enabledUsersGauge.Set(float64(users))
	}
}
