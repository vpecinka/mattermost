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
	// Web client metrics
	clientTimeToFirstByte        *prometheus.HistogramVec
	clientTimeToLastByte         *prometheus.HistogramVec
	clientTimeToDomInteractive   *prometheus.HistogramVec
	clientSplashScreenEnd        *prometheus.HistogramVec
	clientFirstContentfulPaint   *prometheus.HistogramVec
	clientLargestContentfulPaint *prometheus.HistogramVec
	clientInteractionToNextPaint *prometheus.HistogramVec
	clientCumulativeLayoutShift  *prometheus.HistogramVec
	clientLongTasks              *prometheus.CounterVec
	clientPageLoadDuration       *prometheus.HistogramVec
	clientChannelSwitchDuration  *prometheus.HistogramVec
	clientTeamSwitchDuration     *prometheus.HistogramVec
	clientRHSLoadDuration        *prometheus.HistogramVec
	globalThreadsLoadDuration    *prometheus.HistogramVec

	// Mobile client metrics
	mobileClientLoadDuration          *prometheus.HistogramVec
	mobileClientChannelSwitchDuration *prometheus.HistogramVec
	mobileClientTeamSwitchDuration    *prometheus.HistogramVec
	mobileClientNetworkMetrics        *prometheus.HistogramVec
	mobileClientSessionMetadata       *prometheus.GaugeVec

	// Desktop client metrics
	desktopCpuUsage    *prometheus.GaugeVec
	desktopMemoryUsage *prometheus.GaugeVec
)

func (m *SznMetrics) initClientMetrics() {
	// Web client metrics
	clientTimeToFirstByte = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace:   MetricsNamespace,
		Subsystem:   MetricsSubsystemClientsWeb,
		Name:        "ttfb_seconds",
		Help:        "Time to first byte in seconds.",
		ConstLabels: m.additionalLabels,
		Buckets:     []float64{0.1, 0.25, 0.5, 1, 2.5, 5, 10},
	}, []string{"platform", "agent", "user_id"})
	m.Registry.MustRegister(clientTimeToFirstByte)

	clientTimeToLastByte = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace:   MetricsNamespace,
		Subsystem:   MetricsSubsystemClientsWeb,
		Name:        "ttlb_seconds",
		Help:        "Time to last byte in seconds.",
		ConstLabels: m.additionalLabels,
		Buckets:     []float64{0.1, 0.25, 0.5, 1, 2.5, 5, 10},
	}, []string{"platform", "agent", "user_id"})
	m.Registry.MustRegister(clientTimeToLastByte)

	clientTimeToDomInteractive = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace:   MetricsNamespace,
		Subsystem:   MetricsSubsystemClientsWeb,
		Name:        "ttdi_seconds",
		Help:        "Time to DOM interactive in seconds.",
		ConstLabels: m.additionalLabels,
		Buckets:     []float64{0.1, 0.25, 0.5, 1, 2.5, 5, 10},
	}, []string{"platform", "agent", "user_id"})
	m.Registry.MustRegister(clientTimeToDomInteractive)

	clientSplashScreenEnd = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace:   MetricsNamespace,
		Subsystem:   MetricsSubsystemClientsWeb,
		Name:        "splash_screen_end_seconds",
		Help:        "Splash screen end time in seconds.",
		ConstLabels: m.additionalLabels,
		Buckets:     []float64{0.1, 0.25, 0.5, 1, 2.5, 5, 10},
	}, []string{"platform", "agent", "page_type", "user_id"})
	m.Registry.MustRegister(clientSplashScreenEnd)

	clientFirstContentfulPaint = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace:   MetricsNamespace,
		Subsystem:   MetricsSubsystemClientsWeb,
		Name:        "fcp_seconds",
		Help:        "First contentful paint in seconds.",
		ConstLabels: m.additionalLabels,
		Buckets:     []float64{0.1, 0.25, 0.5, 1, 2.5, 5, 10},
	}, []string{"platform", "agent", "user_id"})
	m.Registry.MustRegister(clientFirstContentfulPaint)

	clientLargestContentfulPaint = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace:   MetricsNamespace,
		Subsystem:   MetricsSubsystemClientsWeb,
		Name:        "lcp_seconds",
		Help:        "Largest contentful paint in seconds.",
		ConstLabels: m.additionalLabels,
		Buckets:     []float64{0.1, 0.25, 0.5, 1, 2.5, 5, 10},
	}, []string{"platform", "agent", "region", "user_id"})
	m.Registry.MustRegister(clientLargestContentfulPaint)

	clientInteractionToNextPaint = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace:   MetricsNamespace,
		Subsystem:   MetricsSubsystemClientsWeb,
		Name:        "inp_seconds",
		Help:        "Interaction to next paint in seconds.",
		ConstLabels: m.additionalLabels,
		Buckets:     []float64{0.01, 0.05, 0.1, 0.2, 0.5, 1, 2},
	}, []string{"platform", "agent", "interaction", "user_id"})
	m.Registry.MustRegister(clientInteractionToNextPaint)

	clientCumulativeLayoutShift = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace:   MetricsNamespace,
		Subsystem:   MetricsSubsystemClientsWeb,
		Name:        "cls",
		Help:        "Cumulative layout shift score.",
		ConstLabels: m.additionalLabels,
		Buckets:     []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1},
	}, []string{"platform", "agent", "user_id"})
	m.Registry.MustRegister(clientCumulativeLayoutShift)

	clientLongTasks = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace:   MetricsNamespace,
		Subsystem:   MetricsSubsystemClientsWeb,
		Name:        "long_tasks_total",
		Help:        "The total number of long tasks.",
		ConstLabels: m.additionalLabels,
	}, []string{"platform", "agent", "user_id"})
	m.Registry.MustRegister(clientLongTasks)

	clientPageLoadDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace:   MetricsNamespace,
		Subsystem:   MetricsSubsystemClientsWeb,
		Name:        "page_load_duration_seconds",
		Help:        "Page load duration in seconds.",
		ConstLabels: m.additionalLabels,
		Buckets:     []float64{0.1, 0.25, 0.5, 1, 2.5, 5, 10},
	}, []string{"platform", "agent", "user_id"})
	m.Registry.MustRegister(clientPageLoadDuration)

	clientChannelSwitchDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace:   MetricsNamespace,
		Subsystem:   MetricsSubsystemClientsWeb,
		Name:        "channel_switch_duration_seconds",
		Help:        "Channel switch duration in seconds.",
		ConstLabels: m.additionalLabels,
		Buckets:     []float64{0.05, 0.1, 0.25, 0.5, 1, 2.5, 5},
	}, []string{"platform", "agent", "fresh", "user_id"})
	m.Registry.MustRegister(clientChannelSwitchDuration)

	clientTeamSwitchDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace:   MetricsNamespace,
		Subsystem:   MetricsSubsystemClientsWeb,
		Name:        "team_switch_duration_seconds",
		Help:        "Team switch duration in seconds.",
		ConstLabels: m.additionalLabels,
		Buckets:     []float64{0.05, 0.1, 0.25, 0.5, 1, 2.5, 5},
	}, []string{"platform", "agent", "fresh", "user_id"})
	m.Registry.MustRegister(clientTeamSwitchDuration)

	clientRHSLoadDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace:   MetricsNamespace,
		Subsystem:   MetricsSubsystemClientsWeb,
		Name:        "rhs_load_duration_seconds",
		Help:        "RHS load duration in seconds.",
		ConstLabels: m.additionalLabels,
		Buckets:     []float64{0.05, 0.1, 0.25, 0.5, 1, 2.5, 5},
	}, []string{"platform", "agent", "user_id"})
	m.Registry.MustRegister(clientRHSLoadDuration)

	globalThreadsLoadDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace:   MetricsNamespace,
		Subsystem:   MetricsSubsystemClientsWeb,
		Name:        "global_threads_load_duration_seconds",
		Help:        "Global threads load duration in seconds.",
		ConstLabels: m.additionalLabels,
		Buckets:     []float64{0.05, 0.1, 0.25, 0.5, 1, 2.5, 5},
	}, []string{"platform", "agent", "user_id"})
	m.Registry.MustRegister(globalThreadsLoadDuration)

	// Mobile client metrics
	mobileClientLoadDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace:   MetricsNamespace,
		Subsystem:   MetricsSubsystemClientsMobileApp,
		Name:        "load_duration_seconds",
		Help:        "Mobile client load duration in seconds.",
		ConstLabels: m.additionalLabels,
		Buckets:     []float64{0.1, 0.25, 0.5, 1, 2.5, 5, 10},
	}, []string{"platform"})
	m.Registry.MustRegister(mobileClientLoadDuration)

	mobileClientChannelSwitchDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace:   MetricsNamespace,
		Subsystem:   MetricsSubsystemClientsMobileApp,
		Name:        "channel_switch_duration_seconds",
		Help:        "Mobile client channel switch duration in seconds.",
		ConstLabels: m.additionalLabels,
		Buckets:     []float64{0.05, 0.1, 0.25, 0.5, 1, 2.5, 5},
	}, []string{"platform"})
	m.Registry.MustRegister(mobileClientChannelSwitchDuration)

	mobileClientTeamSwitchDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace:   MetricsNamespace,
		Subsystem:   MetricsSubsystemClientsMobileApp,
		Name:        "team_switch_duration_seconds",
		Help:        "Mobile client team switch duration in seconds.",
		ConstLabels: m.additionalLabels,
		Buckets:     []float64{0.05, 0.1, 0.25, 0.5, 1, 2.5, 5},
	}, []string{"platform"})
	m.Registry.MustRegister(mobileClientTeamSwitchDuration)

	mobileClientNetworkMetrics = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace:   MetricsNamespace,
		Subsystem:   MetricsSubsystemClientsMobileApp,
		Name:        "network_metrics",
		Help:        "Mobile client network metrics.",
		ConstLabels: m.additionalLabels,
		Buckets:     []float64{1, 10, 100, 1000, 10000, 100000},
	}, []string{"platform", "agent", "group", "metric_type"})
	m.Registry.MustRegister(mobileClientNetworkMetrics)

	mobileClientSessionMetadata = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace:   MetricsNamespace,
		Subsystem:   MetricsSubsystemClientsMobileApp,
		Name:        "session_metadata",
		Help:        "Mobile client session metadata.",
		ConstLabels: m.additionalLabels,
	}, []string{"version", "platform", "notification_disabled"})
	m.Registry.MustRegister(mobileClientSessionMetadata)

	// Desktop client metrics
	desktopCpuUsage = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace:   MetricsNamespace,
		Subsystem:   MetricsSubsystemClientsDesktopApp,
		Name:        "cpu_usage_percent",
		Help:        "Desktop client CPU usage percentage.",
		ConstLabels: m.additionalLabels,
	}, []string{"platform", "version", "process"})
	m.Registry.MustRegister(desktopCpuUsage)

	desktopMemoryUsage = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace:   MetricsNamespace,
		Subsystem:   MetricsSubsystemClientsDesktopApp,
		Name:        "memory_usage_bytes",
		Help:        "Desktop client memory usage in bytes.",
		ConstLabels: m.additionalLabels,
	}, []string{"platform", "version", "process"})
	m.Registry.MustRegister(desktopMemoryUsage)
}

// Web client metric methods
func (m *SznMetrics) ObserveClientTimeToFirstByte(platform, agent, userID string, elapsed float64) {
	if clientTimeToFirstByte != nil {
		clientTimeToFirstByte.With(prometheus.Labels{"platform": platform, "agent": agent, "user_id": userID}).Observe(elapsed)
	}
}

func (m *SznMetrics) ObserveClientTimeToLastByte(platform, agent, userID string, elapsed float64) {
	if clientTimeToLastByte != nil {
		clientTimeToLastByte.With(prometheus.Labels{"platform": platform, "agent": agent, "user_id": userID}).Observe(elapsed)
	}
}

func (m *SznMetrics) ObserveClientTimeToDomInteractive(platform, agent, userID string, elapsed float64) {
	if clientTimeToDomInteractive != nil {
		clientTimeToDomInteractive.With(prometheus.Labels{"platform": platform, "agent": agent, "user_id": userID}).Observe(elapsed)
	}
}

func (m *SznMetrics) ObserveClientSplashScreenEnd(platform, agent, pageType, userID string, elapsed float64) {
	if clientSplashScreenEnd != nil {
		clientSplashScreenEnd.With(prometheus.Labels{"platform": platform, "agent": agent, "page_type": pageType, "user_id": userID}).Observe(elapsed)
	}
}

func (m *SznMetrics) ObserveClientFirstContentfulPaint(platform, agent, userID string, elapsed float64) {
	if clientFirstContentfulPaint != nil {
		clientFirstContentfulPaint.With(prometheus.Labels{"platform": platform, "agent": agent, "user_id": userID}).Observe(elapsed)
	}
}

func (m *SznMetrics) ObserveClientLargestContentfulPaint(platform, agent, region, userID string, elapsed float64) {
	if clientLargestContentfulPaint != nil {
		clientLargestContentfulPaint.With(prometheus.Labels{"platform": platform, "agent": agent, "region": region, "user_id": userID}).Observe(elapsed)
	}
}

func (m *SznMetrics) ObserveClientInteractionToNextPaint(platform, agent, interaction, userID string, elapsed float64) {
	if clientInteractionToNextPaint != nil {
		clientInteractionToNextPaint.With(prometheus.Labels{"platform": platform, "agent": agent, "interaction": interaction, "user_id": userID}).Observe(elapsed)
	}
}

func (m *SznMetrics) ObserveClientCumulativeLayoutShift(platform, agent, userID string, elapsed float64) {
	if clientCumulativeLayoutShift != nil {
		clientCumulativeLayoutShift.With(prometheus.Labels{"platform": platform, "agent": agent, "user_id": userID}).Observe(elapsed)
	}
}

func (m *SznMetrics) IncrementClientLongTasks(platform, agent, userID string, inc float64) {
	if clientLongTasks != nil {
		clientLongTasks.With(prometheus.Labels{"platform": platform, "agent": agent, "user_id": userID}).Add(inc)
	}
}

func (m *SznMetrics) ObserveClientPageLoadDuration(platform, agent, userID string, elapsed float64) {
	if clientPageLoadDuration != nil {
		clientPageLoadDuration.With(prometheus.Labels{"platform": platform, "agent": agent, "user_id": userID}).Observe(elapsed)
	}
}

func (m *SznMetrics) ObserveClientChannelSwitchDuration(platform, agent, fresh, userID string, elapsed float64) {
	if clientChannelSwitchDuration != nil {
		clientChannelSwitchDuration.With(prometheus.Labels{"platform": platform, "agent": agent, "fresh": fresh, "user_id": userID}).Observe(elapsed)
	}
}

func (m *SznMetrics) ObserveClientTeamSwitchDuration(platform, agent, fresh, userID string, elapsed float64) {
	if clientTeamSwitchDuration != nil {
		clientTeamSwitchDuration.With(prometheus.Labels{"platform": platform, "agent": agent, "fresh": fresh, "user_id": userID}).Observe(elapsed)
	}
}

func (m *SznMetrics) ObserveClientRHSLoadDuration(platform, agent, userID string, elapsed float64) {
	if clientRHSLoadDuration != nil {
		clientRHSLoadDuration.With(prometheus.Labels{"platform": platform, "agent": agent, "user_id": userID}).Observe(elapsed)
	}
}

func (m *SznMetrics) ObserveGlobalThreadsLoadDuration(platform, agent, userID string, elapsed float64) {
	if globalThreadsLoadDuration != nil {
		globalThreadsLoadDuration.With(prometheus.Labels{"platform": platform, "agent": agent, "user_id": userID}).Observe(elapsed)
	}
}

// Mobile client metric methods
func (m *SznMetrics) ObserveMobileClientLoadDuration(platform string, elapsed float64) {
	if mobileClientLoadDuration != nil {
		mobileClientLoadDuration.With(prometheus.Labels{"platform": platform}).Observe(elapsed)
	}
}

func (m *SznMetrics) ObserveMobileClientChannelSwitchDuration(platform string, elapsed float64) {
	if mobileClientChannelSwitchDuration != nil {
		mobileClientChannelSwitchDuration.With(prometheus.Labels{"platform": platform}).Observe(elapsed)
	}
}

func (m *SznMetrics) ObserveMobileClientTeamSwitchDuration(platform string, elapsed float64) {
	if mobileClientTeamSwitchDuration != nil {
		mobileClientTeamSwitchDuration.With(prometheus.Labels{"platform": platform}).Observe(elapsed)
	}
}

func (m *SznMetrics) ObserveMobileClientNetworkRequestsAverageSpeed(platform, agent, networkRequestGroup string, speed float64) {
	if mobileClientNetworkMetrics != nil {
		mobileClientNetworkMetrics.With(prometheus.Labels{"platform": platform, "agent": agent, "group": networkRequestGroup, "metric_type": "avg_speed"}).Observe(speed)
	}
}

func (m *SznMetrics) ObserveMobileClientNetworkRequestsEffectiveLatency(platform, agent, networkRequestGroup string, latency float64) {
	if mobileClientNetworkMetrics != nil {
		mobileClientNetworkMetrics.With(prometheus.Labels{"platform": platform, "agent": agent, "group": networkRequestGroup, "metric_type": "eff_latency"}).Observe(latency)
	}
}

func (m *SznMetrics) ObserveMobileClientNetworkRequestsElapsedTime(platform, agent, networkRequestGroup string, elapsedTime float64) {
	if mobileClientNetworkMetrics != nil {
		mobileClientNetworkMetrics.With(prometheus.Labels{"platform": platform, "agent": agent, "group": networkRequestGroup, "metric_type": "elapsed_time"}).Observe(elapsedTime)
	}
}

func (m *SznMetrics) ObserveMobileClientNetworkRequestsLatency(platform, agent, networkRequestGroup string, latency float64) {
	if mobileClientNetworkMetrics != nil {
		mobileClientNetworkMetrics.With(prometheus.Labels{"platform": platform, "agent": agent, "group": networkRequestGroup, "metric_type": "latency"}).Observe(latency)
	}
}

func (m *SznMetrics) ObserveMobileClientNetworkRequestsTotalCompressedSize(platform, agent, networkRequestGroup string, size float64) {
	if mobileClientNetworkMetrics != nil {
		mobileClientNetworkMetrics.With(prometheus.Labels{"platform": platform, "agent": agent, "group": networkRequestGroup, "metric_type": "compressed_size"}).Observe(size)
	}
}

func (m *SznMetrics) ObserveMobileClientNetworkRequestsTotalParallelRequests(platform, agent, networkRequestGroup string, count float64) {
	if mobileClientNetworkMetrics != nil {
		mobileClientNetworkMetrics.With(prometheus.Labels{"platform": platform, "agent": agent, "group": networkRequestGroup, "metric_type": "parallel_requests"}).Observe(count)
	}
}

func (m *SznMetrics) ObserveMobileClientNetworkRequestsTotalRequests(platform, agent, networkRequestGroup string, count float64) {
	if mobileClientNetworkMetrics != nil {
		mobileClientNetworkMetrics.With(prometheus.Labels{"platform": platform, "agent": agent, "group": networkRequestGroup, "metric_type": "total_requests"}).Observe(count)
	}
}

func (m *SznMetrics) ObserveMobileClientNetworkRequestsTotalSequentialRequests(platform, agent, networkRequestGroup string, count float64) {
	if mobileClientNetworkMetrics != nil {
		mobileClientNetworkMetrics.With(prometheus.Labels{"platform": platform, "agent": agent, "group": networkRequestGroup, "metric_type": "sequential_requests"}).Observe(count)
	}
}

func (m *SznMetrics) ObserveMobileClientNetworkRequestsTotalSize(platform, agent, networkRequestGroup string, size float64) {
	if mobileClientNetworkMetrics != nil {
		mobileClientNetworkMetrics.With(prometheus.Labels{"platform": platform, "agent": agent, "group": networkRequestGroup, "metric_type": "total_size"}).Observe(size)
	}
}

func (m *SznMetrics) ClearMobileClientSessionMetadata() {
	if mobileClientSessionMetadata != nil {
		mobileClientSessionMetadata.Reset()
	}
}

func (m *SznMetrics) ObserveMobileClientSessionMetadata(version string, platform string, value float64, notificationDisabled string) {
	if mobileClientSessionMetadata != nil {
		mobileClientSessionMetadata.With(prometheus.Labels{"version": version, "platform": platform, "notification_disabled": notificationDisabled}).Set(value)
	}
}

// Desktop client metric methods
func (m *SznMetrics) ObserveDesktopCpuUsage(platform, version, process string, usage float64) {
	if desktopCpuUsage != nil {
		desktopCpuUsage.With(prometheus.Labels{"platform": platform, "version": version, "process": process}).Set(usage)
	}
}

func (m *SznMetrics) ObserveDesktopMemoryUsage(platform, version, process string, usage float64) {
	if desktopMemoryUsage != nil {
		desktopMemoryUsage.With(prometheus.Labels{"platform": platform, "version": version, "process": process}).Set(usage)
	}
}
