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
	loginCounter     prometheus.Counter
	loginFailCounter prometheus.Counter
)

func (m *SznMetrics) initLoginMetrics() {
	loginCounter = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace:   MetricsNamespace,
		Subsystem:   MetricsSubsystemLogin,
		Name:        "logins_total",
		Help:        "The total number of successful logins.",
		ConstLabels: m.additionalLabels,
	})
	m.Registry.MustRegister(loginCounter)

	loginFailCounter = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace:   MetricsNamespace,
		Subsystem:   MetricsSubsystemLogin,
		Name:        "logins_fail_total",
		Help:        "The total number of failed logins.",
		ConstLabels: m.additionalLabels,
	})
	m.Registry.MustRegister(loginFailCounter)
}

// IncrementLogin increments the login counter
func (m *SznMetrics) IncrementLogin() {
	if loginCounter != nil {
		loginCounter.Inc()
	}
}

// IncrementLoginFail increments the login failure counter
func (m *SznMetrics) IncrementLoginFail() {
	if loginFailCounter != nil {
		loginFailCounter.Inc()
	}
}
