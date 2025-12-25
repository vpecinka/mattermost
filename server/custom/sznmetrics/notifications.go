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

func (m *SznMetrics) initNotificationMetrics() {
	m.notificationCounter = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace:   MetricsNamespace,
		Subsystem:   MetricsSubsystemNotifications,
		Name:        "total",
		Help:        "The total number of notifications sent.",
		ConstLabels: m.additionalLabels,
	}, []string{"type", "platform"})
	m.Registry.MustRegister(m.notificationCounter)

	m.notificationAckCounter = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace:   MetricsNamespace,
		Subsystem:   MetricsSubsystemNotifications,
		Name:        "ack_total",
		Help:        "The total number of notifications acknowledged.",
		ConstLabels: m.additionalLabels,
	}, []string{"type", "platform"})
	m.Registry.MustRegister(m.notificationAckCounter)

	m.notificationSuccessCounter = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace:   MetricsNamespace,
		Subsystem:   MetricsSubsystemNotifications,
		Name:        "success_total",
		Help:        "The total number of successful notifications.",
		ConstLabels: m.additionalLabels,
	}, []string{"type", "platform"})
	m.Registry.MustRegister(m.notificationSuccessCounter)

	m.notificationErrorCounter = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace:   MetricsNamespace,
		Subsystem:   MetricsSubsystemNotifications,
		Name:        "error_total",
		Help:        "The total number of notification errors.",
		ConstLabels: m.additionalLabels,
	}, []string{"type", "reason", "platform"})
	m.Registry.MustRegister(m.notificationErrorCounter)

	m.notificationNotSentCounter = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace:   MetricsNamespace,
		Subsystem:   MetricsSubsystemNotifications,
		Name:        "not_sent_total",
		Help:        "The total number of notifications not sent.",
		ConstLabels: m.additionalLabels,
	}, []string{"type", "reason", "platform"})
	m.Registry.MustRegister(m.notificationNotSentCounter)

	m.notificationUnsupportedCounter = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace:   MetricsNamespace,
		Subsystem:   MetricsSubsystemNotifications,
		Name:        "unsupported_total",
		Help:        "The total number of unsupported notifications.",
		ConstLabels: m.additionalLabels,
	}, []string{"type", "reason", "platform"})
	m.Registry.MustRegister(m.notificationUnsupportedCounter)
}

// IncrementNotificationCounter increments notification counter
func (m *SznMetrics) IncrementNotificationCounter(notificationType model.NotificationType, platform string) {
	if m.notificationCounter != nil {
		m.notificationCounter.With(prometheus.Labels{"type": string(notificationType), "platform": platform}).Inc()
	}
}

// IncrementNotificationAckCounter increments notification ack counter
func (m *SznMetrics) IncrementNotificationAckCounter(notificationType model.NotificationType, platform string) {
	if m.notificationAckCounter != nil {
		m.notificationAckCounter.With(prometheus.Labels{"type": string(notificationType), "platform": platform}).Inc()
	}
}

// IncrementNotificationSuccessCounter increments notification success counter
func (m *SznMetrics) IncrementNotificationSuccessCounter(notificationType model.NotificationType, platform string) {
	if m.notificationSuccessCounter != nil {
		m.notificationSuccessCounter.With(prometheus.Labels{"type": string(notificationType), "platform": platform}).Inc()
	}
}

// IncrementNotificationErrorCounter increments notification error counter
func (m *SznMetrics) IncrementNotificationErrorCounter(notificationType model.NotificationType, errorReason model.NotificationReason, platform string) {
	if m.notificationErrorCounter != nil {
		m.notificationErrorCounter.With(prometheus.Labels{
			"type":     string(notificationType),
			"reason":   string(errorReason),
			"platform": platform,
		}).Inc()
	}
}

// IncrementNotificationNotSentCounter increments notification not sent counter
func (m *SznMetrics) IncrementNotificationNotSentCounter(notificationType model.NotificationType, notSentReason model.NotificationReason, platform string) {
	if m.notificationNotSentCounter != nil {
		m.notificationNotSentCounter.With(prometheus.Labels{
			"type":     string(notificationType),
			"reason":   string(notSentReason),
			"platform": platform,
		}).Inc()
	}
}

// IncrementNotificationUnsupportedCounter increments notification unsupported counter
func (m *SznMetrics) IncrementNotificationUnsupportedCounter(notificationType model.NotificationType, notSentReason model.NotificationReason, platform string) {
	if m.notificationUnsupportedCounter != nil {
		m.notificationUnsupportedCounter.With(prometheus.Labels{
			"type":     string(notificationType),
			"reason":   string(notSentReason),
			"platform": platform,
		}).Inc()
	}
}
