// Copyright (c) 2024-present Seznam.cz, a.s.
// All Rights Reserved.
//
// This file is part of custom extensions to Mattermost server
// developed by Seznam.cz, a.s. for internal use only.

package sznmetrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

func (m *SznMetrics) initHTTPMetrics() {
	m.httpRequestsCounter = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace:   MetricsNamespace,
		Subsystem:   MetricsSubsystemHTTP,
		Name:        "requests_total",
		Help:        "The total number of HTTP requests.",
		ConstLabels: m.additionalLabels,
	})
	m.Registry.MustRegister(m.httpRequestsCounter)

	m.httpErrorsCounter = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace:   MetricsNamespace,
		Subsystem:   MetricsSubsystemHTTP,
		Name:        "errors_total",
		Help:        "The total number of HTTP errors.",
		ConstLabels: m.additionalLabels,
	})
	m.Registry.MustRegister(m.httpErrorsCounter)

	m.httpWebsocketsGauge = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace:   MetricsNamespace,
		Subsystem:   MetricsSubsystemHTTP,
		Name:        "websockets",
		Help:        "The number of active websocket connections.",
		ConstLabels: m.additionalLabels,
	}, []string{"origin_client"})
	m.Registry.MustRegister(m.httpWebsocketsGauge)

	m.apiEndpointDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace:   MetricsNamespace,
		Subsystem:   MetricsSubsystemAPI,
		Name:        "time_seconds",
		Help:        "Time taken to execute API endpoint in seconds.",
		ConstLabels: m.additionalLabels,
		Buckets:     []float64{0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60},
	}, []string{"handler", "method", "status_code", "origin_client", "page_load_context"})
	m.Registry.MustRegister(m.apiEndpointDuration)
}

// IncrementHTTPRequest increments the HTTP request counter
func (m *SznMetrics) IncrementHTTPRequest() {
	if m.httpRequestsCounter != nil {
		m.httpRequestsCounter.Inc()
	}
}

// IncrementHTTPError increments the HTTP error counter
func (m *SznMetrics) IncrementHTTPError() {
	if m.httpErrorsCounter != nil {
		m.httpErrorsCounter.Inc()
	}
}

// IncrementHTTPWebSockets increments the websocket gauge
func (m *SznMetrics) IncrementHTTPWebSockets(originClient string) {
	if m.httpWebsocketsGauge != nil {
		m.httpWebsocketsGauge.With(prometheus.Labels{"origin_client": originClient}).Inc()
	}
}

// DecrementHTTPWebSockets decrements the websocket gauge
func (m *SznMetrics) DecrementHTTPWebSockets(originClient string) {
	if m.httpWebsocketsGauge != nil {
		m.httpWebsocketsGauge.With(prometheus.Labels{"origin_client": originClient}).Dec()
	}
}

// ObserveAPIEndpointDuration observes API endpoint duration
func (m *SznMetrics) ObserveAPIEndpointDuration(endpoint, method, statusCode, originClient, pageLoadContext string, elapsed float64) {
	if m.apiEndpointDuration != nil {
		m.apiEndpointDuration.With(prometheus.Labels{
			"handler":           endpoint,
			"method":            method,
			"status_code":       statusCode,
			"origin_client":     originClient,
			"page_load_context": pageLoadContext,
		}).Observe(elapsed)
	}
}
