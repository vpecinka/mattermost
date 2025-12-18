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
	httpRequestsCounter prometheus.Counter
	httpErrorsCounter   prometheus.Counter
	httpWebsocketsGauge *prometheus.GaugeVec
	apiEndpointDuration *prometheus.HistogramVec
)

func (m *SznMetrics) initHTTPMetrics() {
	httpRequestsCounter = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace:   MetricsNamespace,
		Subsystem:   MetricsSubsystemHTTP,
		Name:        "requests_total",
		Help:        "The total number of HTTP requests.",
		ConstLabels: m.additionalLabels,
	})
	m.Registry.MustRegister(httpRequestsCounter)

	httpErrorsCounter = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace:   MetricsNamespace,
		Subsystem:   MetricsSubsystemHTTP,
		Name:        "errors_total",
		Help:        "The total number of HTTP errors.",
		ConstLabels: m.additionalLabels,
	})
	m.Registry.MustRegister(httpErrorsCounter)

	httpWebsocketsGauge = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace:   MetricsNamespace,
		Subsystem:   MetricsSubsystemHTTP,
		Name:        "websockets",
		Help:        "The number of active websocket connections.",
		ConstLabels: m.additionalLabels,
	}, []string{"origin_client"})
	m.Registry.MustRegister(httpWebsocketsGauge)

	apiEndpointDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace:   MetricsNamespace,
		Subsystem:   MetricsSubsystemAPI,
		Name:        "time_seconds",
		Help:        "Time taken to execute API endpoint in seconds.",
		ConstLabels: m.additionalLabels,
		Buckets:     []float64{0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60},
	}, []string{"handler", "method", "status_code", "origin_client", "page_load_context"})
	m.Registry.MustRegister(apiEndpointDuration)
}

// IncrementHTTPRequest increments the HTTP request counter
func (m *SznMetrics) IncrementHTTPRequest() {
	if httpRequestsCounter != nil {
		httpRequestsCounter.Inc()
	}
}

// IncrementHTTPError increments the HTTP error counter
func (m *SznMetrics) IncrementHTTPError() {
	if httpErrorsCounter != nil {
		httpErrorsCounter.Inc()
	}
}

// IncrementHTTPWebSockets increments the websocket gauge
func (m *SznMetrics) IncrementHTTPWebSockets(originClient string) {
	if httpWebsocketsGauge != nil {
		httpWebsocketsGauge.With(prometheus.Labels{"origin_client": originClient}).Inc()
	}
}

// DecrementHTTPWebSockets decrements the websocket gauge
func (m *SznMetrics) DecrementHTTPWebSockets(originClient string) {
	if httpWebsocketsGauge != nil {
		httpWebsocketsGauge.With(prometheus.Labels{"origin_client": originClient}).Dec()
	}
}

// ObserveAPIEndpointDuration observes API endpoint duration
func (m *SznMetrics) ObserveAPIEndpointDuration(endpoint, method, statusCode, originClient, pageLoadContext string, elapsed float64) {
	if apiEndpointDuration != nil {
		apiEndpointDuration.With(prometheus.Labels{
			"handler":           endpoint,
			"method":            method,
			"status_code":       statusCode,
			"origin_client":     originClient,
			"page_load_context": pageLoadContext,
		}).Observe(elapsed)
	}
}
