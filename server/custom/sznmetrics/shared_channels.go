// Copyright (c) 2024-present Seznam.cz, a.s.
// All Rights Reserved.
//
// This file is part of custom extensions to Mattermost server
// developed by Seznam.cz, a.s. for internal use only.

package sznmetrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

func (m *SznMetrics) initSharedChannelsMetrics() {
	m.sharedChannelsSyncCounter = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace:   MetricsNamespace,
		Subsystem:   MetricsSubsystemSharedChannels,
		Name:        "sync_total",
		Help:        "The total number of shared channels syncs.",
		ConstLabels: m.additionalLabels,
	}, []string{"remote_id"})
	m.Registry.MustRegister(m.sharedChannelsSyncCounter)

	m.sharedChannelsTaskInQueueDuration = prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace:   MetricsNamespace,
		Subsystem:   MetricsSubsystemSharedChannels,
		Name:        "task_in_queue_duration_seconds",
		Help:        "Time a shared channels task spent in queue in seconds.",
		ConstLabels: m.additionalLabels,
		Buckets:     []float64{0.1, 0.5, 1, 5, 10, 30, 60, 300, 600},
	})
	m.Registry.MustRegister(m.sharedChannelsTaskInQueueDuration)

	m.sharedChannelsQueueSize = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace:   MetricsNamespace,
		Subsystem:   MetricsSubsystemSharedChannels,
		Name:        "queue_size",
		Help:        "The size of the shared channels queue.",
		ConstLabels: m.additionalLabels,
	})
	m.Registry.MustRegister(m.sharedChannelsQueueSize)

	m.sharedChannelsSyncCollectionDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace:   MetricsNamespace,
		Subsystem:   MetricsSubsystemSharedChannels,
		Name:        "sync_collection_duration_seconds",
		Help:        "Time taken to collect data for shared channels sync in seconds.",
		ConstLabels: m.additionalLabels,
		Buckets:     []float64{0.1, 0.5, 1, 5, 10, 30, 60},
	}, []string{"remote_id"})
	m.Registry.MustRegister(m.sharedChannelsSyncCollectionDuration)

	m.sharedChannelsSyncSendDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace:   MetricsNamespace,
		Subsystem:   MetricsSubsystemSharedChannels,
		Name:        "sync_send_duration_seconds",
		Help:        "Time taken to send data for shared channels sync in seconds.",
		ConstLabels: m.additionalLabels,
		Buckets:     []float64{0.1, 0.5, 1, 5, 10, 30, 60},
	}, []string{"remote_id"})
	m.Registry.MustRegister(m.sharedChannelsSyncSendDuration)

	m.sharedChannelsSyncCollectionStepDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace:   MetricsNamespace,
		Subsystem:   MetricsSubsystemSharedChannels,
		Name:        "sync_collection_step_duration_seconds",
		Help:        "Time taken for each step in shared channels sync collection in seconds.",
		ConstLabels: m.additionalLabels,
		Buckets:     []float64{0.01, 0.05, 0.1, 0.5, 1, 5, 10},
	}, []string{"remote_id", "step"})
	m.Registry.MustRegister(m.sharedChannelsSyncCollectionStepDuration)

	m.sharedChannelsSyncSendStepDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace:   MetricsNamespace,
		Subsystem:   MetricsSubsystemSharedChannels,
		Name:        "sync_send_step_duration_seconds",
		Help:        "Time taken for each step in shared channels sync send in seconds.",
		ConstLabels: m.additionalLabels,
		Buckets:     []float64{0.01, 0.05, 0.1, 0.5, 1, 5, 10},
	}, []string{"remote_id", "step"})
	m.Registry.MustRegister(m.sharedChannelsSyncSendStepDuration)
}

// IncrementSharedChannelsSyncCounter increments shared channels sync counter
func (m *SznMetrics) IncrementSharedChannelsSyncCounter(remoteID string) {
	if m.sharedChannelsSyncCounter != nil {
		m.sharedChannelsSyncCounter.With(prometheus.Labels{"remote_id": remoteID}).Inc()
	}
}

// ObserveSharedChannelsTaskInQueueDuration observes task in queue duration
func (m *SznMetrics) ObserveSharedChannelsTaskInQueueDuration(elapsed float64) {
	if m.sharedChannelsTaskInQueueDuration != nil {
		m.sharedChannelsTaskInQueueDuration.Observe(elapsed)
	}
}

// ObserveSharedChannelsQueueSize observes queue size
func (m *SznMetrics) ObserveSharedChannelsQueueSize(size int64) {
	if m.sharedChannelsQueueSize != nil {
		m.sharedChannelsQueueSize.Set(float64(size))
	}
}

// ObserveSharedChannelsSyncCollectionDuration observes sync collection duration
func (m *SznMetrics) ObserveSharedChannelsSyncCollectionDuration(remoteID string, elapsed float64) {
	if m.sharedChannelsSyncCollectionDuration != nil {
		m.sharedChannelsSyncCollectionDuration.With(prometheus.Labels{"remote_id": remoteID}).Observe(elapsed)
	}
}

// ObserveSharedChannelsSyncSendDuration observes sync send duration
func (m *SznMetrics) ObserveSharedChannelsSyncSendDuration(remoteID string, elapsed float64) {
	if m.sharedChannelsSyncSendDuration != nil {
		m.sharedChannelsSyncSendDuration.With(prometheus.Labels{"remote_id": remoteID}).Observe(elapsed)
	}
}

// ObserveSharedChannelsSyncCollectionStepDuration observes sync collection step duration
func (m *SznMetrics) ObserveSharedChannelsSyncCollectionStepDuration(remoteID string, step string, elapsed float64) {
	if m.sharedChannelsSyncCollectionStepDuration != nil {
		m.sharedChannelsSyncCollectionStepDuration.With(prometheus.Labels{"remote_id": remoteID, "step": step}).Observe(elapsed)
	}
}

// ObserveSharedChannelsSyncSendStepDuration observes sync send step duration
func (m *SznMetrics) ObserveSharedChannelsSyncSendStepDuration(remoteID string, step string, elapsed float64) {
	if m.sharedChannelsSyncSendStepDuration != nil {
		m.sharedChannelsSyncSendStepDuration.With(prometheus.Labels{"remote_id": remoteID, "step": step}).Observe(elapsed)
	}
}
