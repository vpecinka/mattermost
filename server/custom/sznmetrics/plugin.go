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
	pluginHookDuration          *prometheus.HistogramVec
	pluginMultiHookIterDuration prometheus.Histogram
	pluginMultiHookDuration     prometheus.Histogram
	pluginAPIDuration           *prometheus.HistogramVec
)

func (m *SznMetrics) initPluginMetrics() {
	pluginHookDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace:   MetricsNamespace,
		Subsystem:   MetricsSubsystemPlugin,
		Name:        "hook_time_seconds",
		Help:        "Time taken to execute plugin hook in seconds.",
		ConstLabels: m.additionalLabels,
		Buckets:     []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5},
	}, []string{"plugin_id", "hook_name", "success"})
	m.Registry.MustRegister(pluginHookDuration)

	pluginMultiHookIterDuration = prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace:   MetricsNamespace,
		Subsystem:   MetricsSubsystemPlugin,
		Name:        "multi_hook_iteration_time_seconds",
		Help:        "Time taken for a single plugin iteration in multi-hook in seconds.",
		ConstLabels: m.additionalLabels,
		Buckets:     []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1},
	})
	m.Registry.MustRegister(pluginMultiHookIterDuration)

	pluginMultiHookDuration = prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace:   MetricsNamespace,
		Subsystem:   MetricsSubsystemPlugin,
		Name:        "multi_hook_time_seconds",
		Help:        "Time taken to execute all plugins in multi-hook in seconds.",
		ConstLabels: m.additionalLabels,
		Buckets:     []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5},
	})
	m.Registry.MustRegister(pluginMultiHookDuration)

	pluginAPIDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace:   MetricsNamespace,
		Subsystem:   MetricsSubsystemPlugin,
		Name:        "api_time_seconds",
		Help:        "Time taken to execute plugin API call in seconds.",
		ConstLabels: m.additionalLabels,
		Buckets:     []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5},
	}, []string{"plugin_id", "api_name", "success"})
	m.Registry.MustRegister(pluginAPIDuration)
}

// ObservePluginHookDuration observes plugin hook duration
func (m *SznMetrics) ObservePluginHookDuration(pluginID, hookName string, success bool, elapsed float64) {
	if pluginHookDuration != nil {
		successStr := "false"
		if success {
			successStr = "true"
		}
		pluginHookDuration.With(prometheus.Labels{
			"plugin_id": pluginID,
			"hook_name": hookName,
			"success":   successStr,
		}).Observe(elapsed)
	}
}

// ObservePluginMultiHookIterationDuration observes plugin multi-hook iteration duration
func (m *SznMetrics) ObservePluginMultiHookIterationDuration(pluginID string, elapsed float64) {
	if pluginMultiHookIterDuration != nil {
		pluginMultiHookIterDuration.Observe(elapsed)
	}
}

// ObservePluginMultiHookDuration observes plugin multi-hook duration
func (m *SznMetrics) ObservePluginMultiHookDuration(elapsed float64) {
	if pluginMultiHookDuration != nil {
		pluginMultiHookDuration.Observe(elapsed)
	}
}

// ObservePluginAPIDuration observes plugin API duration
func (m *SznMetrics) ObservePluginAPIDuration(pluginID, apiName string, success bool, elapsed float64) {
	if pluginAPIDuration != nil {
		successStr := "false"
		if success {
			successStr = "true"
		}
		pluginAPIDuration.With(prometheus.Labels{
			"plugin_id": pluginID,
			"api_name":  apiName,
			"success":   successStr,
		}).Observe(elapsed)
	}
}
