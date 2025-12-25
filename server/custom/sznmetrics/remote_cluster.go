// Copyright (c) 2024-present Seznam.cz, a.s.
// All Rights Reserved.
//
// This file is part of custom extensions to Mattermost server
// developed by Seznam.cz, a.s. for internal use only.

package sznmetrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

func (m *SznMetrics) initRemoteClusterMetrics() {
	m.remoteClusterMsgSent = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace:   MetricsNamespace,
		Subsystem:   MetricsSubsystemRemoteCluster,
		Name:        "msg_sent_total",
		Help:        "The total number of messages sent to remote cluster.",
		ConstLabels: m.additionalLabels,
	}, []string{"remote_id"})
	m.Registry.MustRegister(m.remoteClusterMsgSent)

	m.remoteClusterMsgReceived = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace:   MetricsNamespace,
		Subsystem:   MetricsSubsystemRemoteCluster,
		Name:        "msg_received_total",
		Help:        "The total number of messages received from remote cluster.",
		ConstLabels: m.additionalLabels,
	}, []string{"remote_id"})
	m.Registry.MustRegister(m.remoteClusterMsgReceived)

	m.remoteClusterMsgErrors = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace:   MetricsNamespace,
		Subsystem:   MetricsSubsystemRemoteCluster,
		Name:        "msg_errors_total",
		Help:        "The total number of message errors with remote cluster.",
		ConstLabels: m.additionalLabels,
	}, []string{"remote_id", "timeout"})
	m.Registry.MustRegister(m.remoteClusterMsgErrors)

	m.remoteClusterPingDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace:   MetricsNamespace,
		Subsystem:   MetricsSubsystemRemoteCluster,
		Name:        "ping_duration_seconds",
		Help:        "Time taken for remote cluster ping in seconds.",
		ConstLabels: m.additionalLabels,
		Buckets:     []float64{0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5},
	}, []string{"remote_id"})
	m.Registry.MustRegister(m.remoteClusterPingDuration)

	m.remoteClusterClockSkew = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace:   MetricsNamespace,
		Subsystem:   MetricsSubsystemRemoteCluster,
		Name:        "clock_skew_seconds",
		Help:        "Clock skew with remote cluster in seconds.",
		ConstLabels: m.additionalLabels,
	}, []string{"remote_id"})
	m.Registry.MustRegister(m.remoteClusterClockSkew)

	m.remoteClusterConnStateChange = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace:   MetricsNamespace,
		Subsystem:   MetricsSubsystemRemoteCluster,
		Name:        "conn_state_change_total",
		Help:        "The total number of connection state changes with remote cluster.",
		ConstLabels: m.additionalLabels,
	}, []string{"remote_id", "online"})
	m.Registry.MustRegister(m.remoteClusterConnStateChange)
}

// IncrementRemoteClusterMsgSentCounter increments remote cluster message sent counter
func (m *SznMetrics) IncrementRemoteClusterMsgSentCounter(remoteID string) {
	if m.remoteClusterMsgSent != nil {
		m.remoteClusterMsgSent.With(prometheus.Labels{"remote_id": remoteID}).Inc()
	}
}

// IncrementRemoteClusterMsgReceivedCounter increments remote cluster message received counter
func (m *SznMetrics) IncrementRemoteClusterMsgReceivedCounter(remoteID string) {
	if m.remoteClusterMsgReceived != nil {
		m.remoteClusterMsgReceived.With(prometheus.Labels{"remote_id": remoteID}).Inc()
	}
}

// IncrementRemoteClusterMsgErrorsCounter increments remote cluster message errors counter
func (m *SznMetrics) IncrementRemoteClusterMsgErrorsCounter(remoteID string, timeout bool) {
	if m.remoteClusterMsgErrors != nil {
		timeoutStr := "false"
		if timeout {
			timeoutStr = "true"
		}
		m.remoteClusterMsgErrors.With(prometheus.Labels{"remote_id": remoteID, "timeout": timeoutStr}).Inc()
	}
}

// ObserveRemoteClusterPingDuration observes remote cluster ping duration
func (m *SznMetrics) ObserveRemoteClusterPingDuration(remoteID string, elapsed float64) {
	if m.remoteClusterPingDuration != nil {
		m.remoteClusterPingDuration.With(prometheus.Labels{"remote_id": remoteID}).Observe(elapsed)
	}
}

// ObserveRemoteClusterClockSkew observes remote cluster clock skew
func (m *SznMetrics) ObserveRemoteClusterClockSkew(remoteID string, skew float64) {
	if m.remoteClusterClockSkew != nil {
		m.remoteClusterClockSkew.With(prometheus.Labels{"remote_id": remoteID}).Set(skew)
	}
}

// IncrementRemoteClusterConnStateChangeCounter increments remote cluster connection state change counter
func (m *SznMetrics) IncrementRemoteClusterConnStateChangeCounter(remoteID string, online bool) {
	if m.remoteClusterConnStateChange != nil {
		onlineStr := "false"
		if online {
			onlineStr = "true"
		}
		m.remoteClusterConnStateChange.With(prometheus.Labels{"remote_id": remoteID, "online": onlineStr}).Inc()
	}
}
