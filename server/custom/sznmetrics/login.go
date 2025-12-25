// Copyright (c) 2024-present Seznam.cz, a.s.
// All Rights Reserved.
//
// This file is part of custom extensions to Mattermost server
// developed by Seznam.cz, a.s. for internal use only.

package sznmetrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

func (m *SznMetrics) initLoginMetrics() {
	m.loginCounter = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace:   MetricsNamespace,
		Subsystem:   MetricsSubsystemLogin,
		Name:        "logins_total",
		Help:        "The total number of successful logins.",
		ConstLabels: m.additionalLabels,
	})
	m.Registry.MustRegister(m.loginCounter)

	m.loginFailCounter = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace:   MetricsNamespace,
		Subsystem:   MetricsSubsystemLogin,
		Name:        "logins_fail_total",
		Help:        "The total number of failed logins.",
		ConstLabels: m.additionalLabels,
	})
	m.Registry.MustRegister(m.loginFailCounter)
}

// IncrementLogin increments the login counter
func (m *SznMetrics) IncrementLogin() {
	if m.loginCounter != nil {
		m.loginCounter.Inc()
	}
}

// IncrementLoginFail increments the login failure counter
func (m *SznMetrics) IncrementLoginFail() {
	if m.loginFailCounter != nil {
		m.loginFailCounter.Inc()
	}
}
