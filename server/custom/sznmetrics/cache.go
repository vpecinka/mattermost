// Copyright (c) 2024-present Seznam.cz, a.s.
// All Rights Reserved.
//
// This file is part of custom extensions to Mattermost server
// developed by Seznam.cz, a.s. for internal use only.

package sznmetrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

func (m *SznMetrics) initCacheMetrics() {
	m.memCacheHitCounters = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace:   MetricsNamespace,
		Subsystem:   MetricsSubsystemCaching,
		Name:        "mem_cache_hit_total",
		Help:        "The total number of cache hits.",
		ConstLabels: m.additionalLabels,
	}, []string{"name"})
	m.Registry.MustRegister(m.memCacheHitCounters)

	m.memCacheMissCounters = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace:   MetricsNamespace,
		Subsystem:   MetricsSubsystemCaching,
		Name:        "mem_cache_miss_total",
		Help:        "The total number of cache misses.",
		ConstLabels: m.additionalLabels,
	}, []string{"name"})
	m.Registry.MustRegister(m.memCacheMissCounters)

	m.memCacheInvalidationCounters = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace:   MetricsNamespace,
		Subsystem:   MetricsSubsystemCaching,
		Name:        "mem_cache_invalidation_total",
		Help:        "The total number of cache invalidations.",
		ConstLabels: m.additionalLabels,
	}, []string{"name"})
	m.Registry.MustRegister(m.memCacheInvalidationCounters)

	m.memCacheHitCounterSession = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace:   MetricsNamespace,
		Subsystem:   MetricsSubsystemCaching,
		Name:        "mem_cache_hit_session_total",
		Help:        "The total number of session cache hits.",
		ConstLabels: m.additionalLabels,
	})
	m.Registry.MustRegister(m.memCacheHitCounterSession)

	m.memCacheMissCounterSession = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace:   MetricsNamespace,
		Subsystem:   MetricsSubsystemCaching,
		Name:        "mem_cache_miss_session_total",
		Help:        "The total number of session cache misses.",
		ConstLabels: m.additionalLabels,
	})
	m.Registry.MustRegister(m.memCacheMissCounterSession)

	m.memCacheInvalidationSession = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace:   MetricsNamespace,
		Subsystem:   MetricsSubsystemCaching,
		Name:        "mem_cache_invalidation_session_total",
		Help:        "The total number of session cache invalidations.",
		ConstLabels: m.additionalLabels,
	})
	m.Registry.MustRegister(m.memCacheInvalidationSession)

	m.etagHitCounters = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace:   MetricsNamespace,
		Subsystem:   MetricsSubsystemCaching,
		Name:        "etag_hit_total",
		Help:        "The total number of ETag hits.",
		ConstLabels: m.additionalLabels,
	}, []string{"route"})
	m.Registry.MustRegister(m.etagHitCounters)

	m.etagMissCounters = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace:   MetricsNamespace,
		Subsystem:   MetricsSubsystemCaching,
		Name:        "etag_miss_total",
		Help:        "The total number of ETag misses.",
		ConstLabels: m.additionalLabels,
	}, []string{"route"})
	m.Registry.MustRegister(m.etagMissCounters)

	m.redisEndpointDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace:   MetricsNamespace,
		Subsystem:   MetricsSubsystemCaching,
		Name:        "redis_time_seconds",
		Help:        "Time taken to execute Redis operations in seconds.",
		ConstLabels: m.additionalLabels,
		Buckets:     []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1},
	}, []string{"cache_name", "operation"})
	m.Registry.MustRegister(m.redisEndpointDuration)
}

// IncrementMemCacheHitCounter increments cache hit counter
func (m *SznMetrics) IncrementMemCacheHitCounter(cacheName string) {
	if m.memCacheHitCounters != nil {
		m.memCacheHitCounters.With(prometheus.Labels{"name": cacheName}).Inc()
	}
}

// IncrementMemCacheMissCounter increments cache miss counter
func (m *SznMetrics) IncrementMemCacheMissCounter(cacheName string) {
	if m.memCacheMissCounters != nil {
		m.memCacheMissCounters.With(prometheus.Labels{"name": cacheName}).Inc()
	}
}

// IncrementMemCacheInvalidationCounter increments cache invalidation counter
func (m *SznMetrics) IncrementMemCacheInvalidationCounter(cacheName string) {
	if m.memCacheInvalidationCounters != nil {
		m.memCacheInvalidationCounters.With(prometheus.Labels{"name": cacheName}).Inc()
	}
}

// IncrementMemCacheHitCounterSession increments session cache hit counter
func (m *SznMetrics) IncrementMemCacheHitCounterSession() {
	if m.memCacheHitCounterSession != nil {
		m.memCacheHitCounterSession.Inc()
	}
}

// IncrementMemCacheMissCounterSession increments session cache miss counter
func (m *SznMetrics) IncrementMemCacheMissCounterSession() {
	if m.memCacheMissCounterSession != nil {
		m.memCacheMissCounterSession.Inc()
	}
}

// IncrementMemCacheInvalidationCounterSession increments session cache invalidation counter
func (m *SznMetrics) IncrementMemCacheInvalidationCounterSession() {
	if m.memCacheInvalidationSession != nil {
		m.memCacheInvalidationSession.Inc()
	}
}

// AddMemCacheHitCounter adds value to cache hit counter
func (m *SznMetrics) AddMemCacheHitCounter(cacheName string, amount float64) {
	if m.memCacheHitCounters != nil {
		m.memCacheHitCounters.With(prometheus.Labels{"name": cacheName}).Add(amount)
	}
}

// AddMemCacheMissCounter adds value to cache miss counter
func (m *SznMetrics) AddMemCacheMissCounter(cacheName string, amount float64) {
	if m.memCacheMissCounters != nil {
		m.memCacheMissCounters.With(prometheus.Labels{"name": cacheName}).Add(amount)
	}
}

// IncrementEtagHitCounter increments ETag hit counter
func (m *SznMetrics) IncrementEtagHitCounter(route string) {
	if m.etagHitCounters != nil {
		m.etagHitCounters.With(prometheus.Labels{"route": route}).Inc()
	}
}

// IncrementEtagMissCounter increments ETag miss counter
func (m *SznMetrics) IncrementEtagMissCounter(route string) {
	if m.etagMissCounters != nil {
		m.etagMissCounters.With(prometheus.Labels{"route": route}).Inc()
	}
}

// ObserveRedisEndpointDuration observes Redis operation duration
func (m *SznMetrics) ObserveRedisEndpointDuration(cacheName, operation string, elapsed float64) {
	if m.redisEndpointDuration != nil {
		m.redisEndpointDuration.With(prometheus.Labels{
			"cache_name": cacheName,
			"operation":  operation,
		}).Observe(elapsed)
	}
}
