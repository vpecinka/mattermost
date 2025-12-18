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
	sharedChannelsSyncCounter                *prometheus.CounterVec
	sharedChannelsTaskInQueueDuration        prometheus.Histogram
	sharedChannelsQueueSize                  prometheus.Gauge
	sharedChannelsSyncCollectionDuration     *prometheus.HistogramVec
	sharedChannelsSyncSendDuration           *prometheus.HistogramVec
	sharedChannelsSyncCollectionStepDuration *prometheus.HistogramVec
	sharedChannelsSyncSendStepDuration       *prometheus.HistogramVec
)

func (m *SznMetrics) initSharedChannelsMetrics() {
	sharedChannelsSyncCounter = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace:   MetricsNamespace,
		Subsystem:   MetricsSubsystemSharedChannels,
		Name:        "sync_total",
		Help:        "The total number of shared channels syncs.",
		ConstLabels: m.additionalLabels,
	}, []string{"remote_id"})
	m.Registry.MustRegister(sharedChannelsSyncCounter)

	sharedChannelsTaskInQueueDuration = prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace:   MetricsNamespace,
		Subsystem:   MetricsSubsystemSharedChannels,
		Name:        "task_in_queue_duration_seconds",
		Help:        "Time a shared channels task spent in queue in seconds.",
		ConstLabels: m.additionalLabels,
		Buckets:     []float64{0.1, 0.5, 1, 5, 10, 30, 60, 300, 600},
	})
	m.Registry.MustRegister(sharedChannelsTaskInQueueDuration)

	sharedChannelsQueueSize = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace:   MetricsNamespace,
		Subsystem:   MetricsSubsystemSharedChannels,
		Name:        "queue_size",
		Help:        "The size of the shared channels queue.",
		ConstLabels: m.additionalLabels,
	})
	m.Registry.MustRegister(sharedChannelsQueueSize)

	sharedChannelsSyncCollectionDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace:   MetricsNamespace,
		Subsystem:   MetricsSubsystemSharedChannels,
		Name:        "sync_collection_duration_seconds",
		Help:        "Time taken to collect data for shared channels sync in seconds.",
		ConstLabels: m.additionalLabels,
		Buckets:     []float64{0.1, 0.5, 1, 5, 10, 30, 60},
	}, []string{"remote_id"})
	m.Registry.MustRegister(sharedChannelsSyncCollectionDuration)

	sharedChannelsSyncSendDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace:   MetricsNamespace,
		Subsystem:   MetricsSubsystemSharedChannels,
		Name:        "sync_send_duration_seconds",
		Help:        "Time taken to send data for shared channels sync in seconds.",
		ConstLabels: m.additionalLabels,
		Buckets:     []float64{0.1, 0.5, 1, 5, 10, 30, 60},
	}, []string{"remote_id"})
	m.Registry.MustRegister(sharedChannelsSyncSendDuration)

	sharedChannelsSyncCollectionStepDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace:   MetricsNamespace,
		Subsystem:   MetricsSubsystemSharedChannels,
		Name:        "sync_collection_step_duration_seconds",
		Help:        "Time taken for each step in shared channels sync collection in seconds.",
		ConstLabels: m.additionalLabels,
		Buckets:     []float64{0.01, 0.05, 0.1, 0.5, 1, 5, 10},
	}, []string{"remote_id", "step"})
	m.Registry.MustRegister(sharedChannelsSyncCollectionStepDuration)

	sharedChannelsSyncSendStepDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace:   MetricsNamespace,
		Subsystem:   MetricsSubsystemSharedChannels,
		Name:        "sync_send_step_duration_seconds",
		Help:        "Time taken for each step in shared channels sync send in seconds.",
		ConstLabels: m.additionalLabels,
		Buckets:     []float64{0.01, 0.05, 0.1, 0.5, 1, 5, 10},
	}, []string{"remote_id", "step"})
	m.Registry.MustRegister(sharedChannelsSyncSendStepDuration)
}

// IncrementSharedChannelsSyncCounter increments shared channels sync counter
func (m *SznMetrics) IncrementSharedChannelsSyncCounter(remoteID string) {
	if sharedChannelsSyncCounter != nil {
		sharedChannelsSyncCounter.With(prometheus.Labels{"remote_id": remoteID}).Inc()
	}
}

// ObserveSharedChannelsTaskInQueueDuration observes task in queue duration
func (m *SznMetrics) ObserveSharedChannelsTaskInQueueDuration(elapsed float64) {
	if sharedChannelsTaskInQueueDuration != nil {
		sharedChannelsTaskInQueueDuration.Observe(elapsed)
	}
}

// ObserveSharedChannelsQueueSize observes queue size
func (m *SznMetrics) ObserveSharedChannelsQueueSize(size int64) {
	if sharedChannelsQueueSize != nil {
		sharedChannelsQueueSize.Set(float64(size))
	}
}

// ObserveSharedChannelsSyncCollectionDuration observes sync collection duration
func (m *SznMetrics) ObserveSharedChannelsSyncCollectionDuration(remoteID string, elapsed float64) {
	if sharedChannelsSyncCollectionDuration != nil {
		sharedChannelsSyncCollectionDuration.With(prometheus.Labels{"remote_id": remoteID}).Observe(elapsed)
	}
}

// ObserveSharedChannelsSyncSendDuration observes sync send duration
func (m *SznMetrics) ObserveSharedChannelsSyncSendDuration(remoteID string, elapsed float64) {
	if sharedChannelsSyncSendDuration != nil {
		sharedChannelsSyncSendDuration.With(prometheus.Labels{"remote_id": remoteID}).Observe(elapsed)
	}
}

// ObserveSharedChannelsSyncCollectionStepDuration observes sync collection step duration
func (m *SznMetrics) ObserveSharedChannelsSyncCollectionStepDuration(remoteID string, step string, elapsed float64) {
	if sharedChannelsSyncCollectionStepDuration != nil {
		sharedChannelsSyncCollectionStepDuration.With(prometheus.Labels{"remote_id": remoteID, "step": step}).Observe(elapsed)
	}
}

// ObserveSharedChannelsSyncSendStepDuration observes sync send step duration
func (m *SznMetrics) ObserveSharedChannelsSyncSendStepDuration(remoteID string, step string, elapsed float64) {
	if sharedChannelsSyncSendStepDuration != nil {
		sharedChannelsSyncSendStepDuration.With(prometheus.Labels{"remote_id": remoteID, "step": step}).Observe(elapsed)
	}
}
