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

func (m *SznMetrics) initWebsocketMetrics() {
	m.websocketEventCounters = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace:   MetricsNamespace,
		Subsystem:   MetricsSubsystemWebsocket,
		Name:        "event_total",
		Help:        "The total number of websocket events sent.",
		ConstLabels: m.additionalLabels,
	}, []string{"type"})
	m.Registry.MustRegister(m.websocketEventCounters)

	m.websocketBroadcastCounters = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace:   MetricsNamespace,
		Subsystem:   MetricsSubsystemWebsocket,
		Name:        "broadcasts_total",
		Help:        "The total number of websocket broadcasts by type.",
		ConstLabels: m.additionalLabels,
	}, []string{"type"})
	m.Registry.MustRegister(m.websocketBroadcastCounters)

	m.websocketBroadcastBufferSize = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace:   MetricsNamespace,
		Subsystem:   MetricsSubsystemWebsocket,
		Name:        "broadcast_buffer_size",
		Help:        "The size of the websocket broadcast buffer.",
		ConstLabels: m.additionalLabels,
	}, []string{"hub"})
	m.Registry.MustRegister(m.websocketBroadcastBufferSize)

	m.websocketBroadcastUsersRegistered = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace:   MetricsNamespace,
		Subsystem:   MetricsSubsystemWebsocket,
		Name:        "broadcast_users_registered",
		Help:        "The number of users registered for websocket broadcasts.",
		ConstLabels: m.additionalLabels,
	}, []string{"hub"})
	m.Registry.MustRegister(m.websocketBroadcastUsersRegistered)

	m.websocketReconnectCounter = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace:   MetricsNamespace,
		Subsystem:   MetricsSubsystemWebsocket,
		Name:        "reconnect_total",
		Help:        "The total number of websocket reconnections.",
		ConstLabels: m.additionalLabels,
	}, []string{"type", "disconnect_error_code"})
	m.Registry.MustRegister(m.websocketReconnectCounter)
}

// IncrementWebsocketEvent increments websocket event counter
func (m *SznMetrics) IncrementWebsocketEvent(eventType model.WebsocketEventType) {
	if m.websocketEventCounters != nil {
		m.websocketEventCounters.With(prometheus.Labels{"type": string(eventType)}).Inc()
	}
}

// IncrementWebSocketBroadcast increments websocket broadcast counter
func (m *SznMetrics) IncrementWebSocketBroadcast(eventType model.WebsocketEventType) {
	if m.websocketBroadcastCounters != nil {
		m.websocketBroadcastCounters.With(prometheus.Labels{"type": string(eventType)}).Inc()
	}
}

// IncrementWebSocketBroadcastBufferSize increments websocket buffer size
func (m *SznMetrics) IncrementWebSocketBroadcastBufferSize(hub string, amount float64) {
	if m.websocketBroadcastBufferSize != nil {
		m.websocketBroadcastBufferSize.With(prometheus.Labels{"hub": hub}).Add(amount)
	}
}

// DecrementWebSocketBroadcastBufferSize decrements websocket buffer size
func (m *SznMetrics) DecrementWebSocketBroadcastBufferSize(hub string, amount float64) {
	if m.websocketBroadcastBufferSize != nil {
		m.websocketBroadcastBufferSize.With(prometheus.Labels{"hub": hub}).Sub(amount)
	}
}

// IncrementWebSocketBroadcastUsersRegistered increments registered users
func (m *SznMetrics) IncrementWebSocketBroadcastUsersRegistered(hub string, amount float64) {
	if m.websocketBroadcastUsersRegistered != nil {
		m.websocketBroadcastUsersRegistered.With(prometheus.Labels{"hub": hub}).Add(amount)
	}
}

// DecrementWebSocketBroadcastUsersRegistered decrements registered users
func (m *SznMetrics) DecrementWebSocketBroadcastUsersRegistered(hub string, amount float64) {
	if m.websocketBroadcastUsersRegistered != nil {
		m.websocketBroadcastUsersRegistered.With(prometheus.Labels{"hub": hub}).Sub(amount)
	}
}

// IncrementWebsocketReconnectEvent increments websocket reconnect counter
func (m *SznMetrics) IncrementWebsocketReconnectEvent(eventType string) {
	if m.websocketReconnectCounter != nil {
		m.websocketReconnectCounter.With(prometheus.Labels{"type": eventType, "disconnect_error_code": ""}).Inc()
	}
}

// IncrementWebsocketReconnectEventWithDisconnectErrCode increments reconnect counter with error code
func (m *SznMetrics) IncrementWebsocketReconnectEventWithDisconnectErrCode(eventType string, disconnectErrCode string) {
	if m.websocketReconnectCounter != nil {
		m.websocketReconnectCounter.With(prometheus.Labels{"type": eventType, "disconnect_error_code": disconnectErrCode}).Inc()
	}
}
