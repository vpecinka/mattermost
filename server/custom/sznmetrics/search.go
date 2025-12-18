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
	postsSearchCounter  prometheus.Counter
	postsSearchDuration prometheus.Histogram
	filesSearchCounter  prometheus.Counter
	filesSearchDuration prometheus.Histogram
	fileIndexCounter    prometheus.Counter
	userIndexCounter    prometheus.Counter
	channelIndexCounter prometheus.Counter
	storeMethodDuration *prometheus.HistogramVec
)

func (m *SznMetrics) initSearchMetrics() {
	postsSearchCounter = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace:   MetricsNamespace,
		Subsystem:   MetricsSubsystemSearch,
		Name:        "posts_searches_total",
		Help:        "The total number of posts searches.",
		ConstLabels: m.additionalLabels,
	})
	m.Registry.MustRegister(postsSearchCounter)

	postsSearchDuration = prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace:   MetricsNamespace,
		Subsystem:   MetricsSubsystemSearch,
		Name:        "posts_search_duration_seconds",
		Help:        "Time taken to search posts in seconds.",
		ConstLabels: m.additionalLabels,
		Buckets:     []float64{0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
	})
	m.Registry.MustRegister(postsSearchDuration)

	filesSearchCounter = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace:   MetricsNamespace,
		Subsystem:   MetricsSubsystemSearch,
		Name:        "files_searches_total",
		Help:        "The total number of files searches.",
		ConstLabels: m.additionalLabels,
	})
	m.Registry.MustRegister(filesSearchCounter)

	filesSearchDuration = prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace:   MetricsNamespace,
		Subsystem:   MetricsSubsystemSearch,
		Name:        "files_search_duration_seconds",
		Help:        "Time taken to search files in seconds.",
		ConstLabels: m.additionalLabels,
		Buckets:     []float64{0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
	})
	m.Registry.MustRegister(filesSearchDuration)

	fileIndexCounter = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace:   MetricsNamespace,
		Subsystem:   MetricsSubsystemSearch,
		Name:        "files_indexed_total",
		Help:        "The total number of files indexed.",
		ConstLabels: m.additionalLabels,
	})
	m.Registry.MustRegister(fileIndexCounter)

	userIndexCounter = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace:   MetricsNamespace,
		Subsystem:   MetricsSubsystemSearch,
		Name:        "users_indexed_total",
		Help:        "The total number of users indexed.",
		ConstLabels: m.additionalLabels,
	})
	m.Registry.MustRegister(userIndexCounter)

	channelIndexCounter = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace:   MetricsNamespace,
		Subsystem:   MetricsSubsystemSearch,
		Name:        "channels_indexed_total",
		Help:        "The total number of channels indexed.",
		ConstLabels: m.additionalLabels,
	})
	m.Registry.MustRegister(channelIndexCounter)

	storeMethodDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace:   MetricsNamespace,
		Subsystem:   MetricsSubsystemDB,
		Name:        "store_time_seconds",
		Help:        "Time taken to execute store method in seconds.",
		ConstLabels: m.additionalLabels,
		Buckets:     []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
	}, []string{"method", "success"})
	m.Registry.MustRegister(storeMethodDuration)
}

// IncrementPostsSearchCounter increments posts search counter
func (m *SznMetrics) IncrementPostsSearchCounter() {
	if postsSearchCounter != nil {
		postsSearchCounter.Inc()
	}
}

// ObservePostsSearchDuration observes posts search duration
func (m *SznMetrics) ObservePostsSearchDuration(elapsed float64) {
	if postsSearchDuration != nil {
		postsSearchDuration.Observe(elapsed)
	}
}

// IncrementFilesSearchCounter increments files search counter
func (m *SznMetrics) IncrementFilesSearchCounter() {
	if filesSearchCounter != nil {
		filesSearchCounter.Inc()
	}
}

// ObserveFilesSearchDuration observes files search duration
func (m *SznMetrics) ObserveFilesSearchDuration(elapsed float64) {
	if filesSearchDuration != nil {
		filesSearchDuration.Observe(elapsed)
	}
}

// IncrementFileIndexCounter increments file index counter
func (m *SznMetrics) IncrementFileIndexCounter() {
	if fileIndexCounter != nil {
		fileIndexCounter.Inc()
	}
}

// IncrementUserIndexCounter increments user index counter
func (m *SznMetrics) IncrementUserIndexCounter() {
	if userIndexCounter != nil {
		userIndexCounter.Inc()
	}
}

// IncrementChannelIndexCounter increments channel index counter
func (m *SznMetrics) IncrementChannelIndexCounter() {
	if channelIndexCounter != nil {
		channelIndexCounter.Inc()
	}
}

// ObserveStoreMethodDuration observes store method duration
func (m *SznMetrics) ObserveStoreMethodDuration(method, success string, elapsed float64) {
	if storeMethodDuration != nil {
		storeMethodDuration.With(prometheus.Labels{
			"method":  method,
			"success": success,
		}).Observe(elapsed)
	}
}
