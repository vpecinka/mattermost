// Copyright (c) 2024-present Seznam.cz, a.s.
// All Rights Reserved.
//
// This file is part of custom extensions to Mattermost server
// developed by Seznam.cz, a.s. for internal use only.

package sznmetrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

func (m *SznMetrics) initPostMetrics() {
	m.postCreateCounter = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace:   MetricsNamespace,
		Subsystem:   MetricsSubsystemPosts,
		Name:        "total",
		Help:        "The total number of posts created.",
		ConstLabels: m.additionalLabels,
	})
	m.Registry.MustRegister(m.postCreateCounter)

	m.webhookPostCounter = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace:   MetricsNamespace,
		Subsystem:   MetricsSubsystemPosts,
		Name:        "webhooks_total",
		Help:        "Total number of webhook posts created.",
		ConstLabels: m.additionalLabels,
	})
	m.Registry.MustRegister(m.webhookPostCounter)

	m.postSentEmailCounter = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace:   MetricsNamespace,
		Subsystem:   MetricsSubsystemPosts,
		Name:        "emails_sent_total",
		Help:        "The total number of emails sent because a post was created.",
		ConstLabels: m.additionalLabels,
	})
	m.Registry.MustRegister(m.postSentEmailCounter)

	m.postSentPushCounter = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace:   MetricsNamespace,
		Subsystem:   MetricsSubsystemPosts,
		Name:        "pushes_sent_total",
		Help:        "The total number of mobile push notifications sent because a post was created.",
		ConstLabels: m.additionalLabels,
	})
	m.Registry.MustRegister(m.postSentPushCounter)

	m.postBroadcastCounter = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace:   MetricsNamespace,
		Subsystem:   MetricsSubsystemPosts,
		Name:        "broadcasts_total",
		Help:        "The total number of websocket broadcasts sent because a post was created.",
		ConstLabels: m.additionalLabels,
	})
	m.Registry.MustRegister(m.postBroadcastCounter)

	m.postFileAttachCounter = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace:   MetricsNamespace,
		Subsystem:   MetricsSubsystemPosts,
		Name:        "file_attachments_total",
		Help:        "The total number of file attachments created because a post was created.",
		ConstLabels: m.additionalLabels,
	})
	m.Registry.MustRegister(m.postFileAttachCounter)

	m.postIndexCounter = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace:   MetricsNamespace,
		Subsystem:   MetricsSubsystemSearch,
		Name:        "posts_indexed_total",
		Help:        "The total number of posts indexed.",
		ConstLabels: m.additionalLabels,
	})
	m.Registry.MustRegister(m.postIndexCounter)
}

// IncrementPostCreate increments the post creation counter
func (m *SznMetrics) IncrementPostCreate() {
	if m.postCreateCounter != nil {
		m.postCreateCounter.Inc()
	}
}

// IncrementWebhookPost increments the webhook post counter
func (m *SznMetrics) IncrementWebhookPost() {
	if m.webhookPostCounter != nil {
		m.webhookPostCounter.Inc()
	}
}

// IncrementPostSentEmail increments the post email counter
func (m *SznMetrics) IncrementPostSentEmail() {
	if m.postSentEmailCounter != nil {
		m.postSentEmailCounter.Inc()
	}
}

// IncrementPostSentPush increments the post push notification counter
func (m *SznMetrics) IncrementPostSentPush() {
	if m.postSentPushCounter != nil {
		m.postSentPushCounter.Inc()
	}
}

// IncrementPostBroadcast increments the post broadcast counter
func (m *SznMetrics) IncrementPostBroadcast() {
	if m.postBroadcastCounter != nil {
		m.postBroadcastCounter.Inc()
	}
}

// IncrementPostFileAttachment increments the file attachment counter
func (m *SznMetrics) IncrementPostFileAttachment(count int) {
	if m.postFileAttachCounter != nil {
		m.postFileAttachCounter.Add(float64(count))
	}
}

// IncrementPostIndexCounter increments the post index counter
func (m *SznMetrics) IncrementPostIndexCounter() {
	if m.postIndexCounter != nil {
		m.postIndexCounter.Inc()
	}
}
