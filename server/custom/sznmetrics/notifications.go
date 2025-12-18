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

var (
	notificationCounter            *prometheus.CounterVec
	notificationAckCounter         *prometheus.CounterVec
	notificationSuccessCounter     *prometheus.CounterVec
	notificationErrorCounter       *prometheus.CounterVec
	notificationNotSentCounter     *prometheus.CounterVec
	notificationUnsupportedCounter *prometheus.CounterVec
)

func (m *SznMetrics) initNotificationMetrics() {
	notificationCounter = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace:   MetricsNamespace,
		Subsystem:   MetricsSubsystemNotifications,
		Name:        "total",
		Help:        "The total number of notifications sent.",
		ConstLabels: m.additionalLabels,
	}, []string{"type", "platform"})
	m.Registry.MustRegister(notificationCounter)

	notificationAckCounter = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace:   MetricsNamespace,
		Subsystem:   MetricsSubsystemNotifications,
		Name:        "ack_total",
		Help:        "The total number of notifications acknowledged.",
		ConstLabels: m.additionalLabels,
	}, []string{"type", "platform"})
	m.Registry.MustRegister(notificationAckCounter)

	notificationSuccessCounter = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace:   MetricsNamespace,
		Subsystem:   MetricsSubsystemNotifications,
		Name:        "success_total",
		Help:        "The total number of successful notifications.",
		ConstLabels: m.additionalLabels,
	}, []string{"type", "platform"})
	m.Registry.MustRegister(notificationSuccessCounter)

	notificationErrorCounter = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace:   MetricsNamespace,
		Subsystem:   MetricsSubsystemNotifications,
		Name:        "error_total",
		Help:        "The total number of notification errors.",
		ConstLabels: m.additionalLabels,
	}, []string{"type", "reason", "platform"})
	m.Registry.MustRegister(notificationErrorCounter)

	notificationNotSentCounter = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace:   MetricsNamespace,
		Subsystem:   MetricsSubsystemNotifications,
		Name:        "not_sent_total",
		Help:        "The total number of notifications not sent.",
		ConstLabels: m.additionalLabels,
	}, []string{"type", "reason", "platform"})
	m.Registry.MustRegister(notificationNotSentCounter)

	notificationUnsupportedCounter = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace:   MetricsNamespace,
		Subsystem:   MetricsSubsystemNotifications,
		Name:        "unsupported_total",
		Help:        "The total number of unsupported notifications.",
		ConstLabels: m.additionalLabels,
	}, []string{"type", "reason", "platform"})
	m.Registry.MustRegister(notificationUnsupportedCounter)
}

// IncrementNotificationCounter increments notification counter
func (m *SznMetrics) IncrementNotificationCounter(notificationType model.NotificationType, platform string) {
	if notificationCounter != nil {
		notificationCounter.With(prometheus.Labels{"type": string(notificationType), "platform": platform}).Inc()
	}
}

// IncrementNotificationAckCounter increments notification ack counter
func (m *SznMetrics) IncrementNotificationAckCounter(notificationType model.NotificationType, platform string) {
	if notificationAckCounter != nil {
		notificationAckCounter.With(prometheus.Labels{"type": string(notificationType), "platform": platform}).Inc()
	}
}

// IncrementNotificationSuccessCounter increments notification success counter
func (m *SznMetrics) IncrementNotificationSuccessCounter(notificationType model.NotificationType, platform string) {
	if notificationSuccessCounter != nil {
		notificationSuccessCounter.With(prometheus.Labels{"type": string(notificationType), "platform": platform}).Inc()
	}
}

// IncrementNotificationErrorCounter increments notification error counter
func (m *SznMetrics) IncrementNotificationErrorCounter(notificationType model.NotificationType, errorReason model.NotificationReason, platform string) {
	if notificationErrorCounter != nil {
		notificationErrorCounter.With(prometheus.Labels{
			"type":     string(notificationType),
			"reason":   string(errorReason),
			"platform": platform,
		}).Inc()
	}
}

// IncrementNotificationNotSentCounter increments notification not sent counter
func (m *SznMetrics) IncrementNotificationNotSentCounter(notificationType model.NotificationType, notSentReason model.NotificationReason, platform string) {
	if notificationNotSentCounter != nil {
		notificationNotSentCounter.With(prometheus.Labels{
			"type":     string(notificationType),
			"reason":   string(notSentReason),
			"platform": platform,
		}).Inc()
	}
}

// IncrementNotificationUnsupportedCounter increments notification unsupported counter
func (m *SznMetrics) IncrementNotificationUnsupportedCounter(notificationType model.NotificationType, notSentReason model.NotificationReason, platform string) {
	if notificationUnsupportedCounter != nil {
		notificationUnsupportedCounter.With(prometheus.Labels{
			"type":     string(notificationType),
			"reason":   string(notSentReason),
			"platform": platform,
		}).Inc()
	}
}
