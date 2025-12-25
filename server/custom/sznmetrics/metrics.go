// Copyright (c) 2024-present Seznam.cz, a.s.
// All Rights Reserved.
//
// This file is part of custom extensions to Mattermost server
// developed by Seznam.cz, a.s. for internal use only.
//
// This implementation provides Prometheus metrics support without
// requiring an Enterprise license, as it is a custom implementation
// built from scratch by Seznam.cz for internal use.

package sznmetrics

import (
	"database/sql"
	"os"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/mattermost/mattermost/server/public/shared/mlog"
	"github.com/mattermost/mattermost/server/v8/channels/app/platform"
)

const (
	MetricsNamespace = "mattermost"

	MetricsSubsystemPosts             = "post"
	MetricsSubsystemDB                = "db"
	MetricsSubsystemAPI               = "api"
	MetricsSubsystemPlugin            = "plugin"
	MetricsSubsystemHTTP              = "http"
	MetricsSubsystemCluster           = "cluster"
	MetricsSubsystemLogin             = "login"
	MetricsSubsystemCaching           = "cache"
	MetricsSubsystemWebsocket         = "websocket"
	MetricsSubsystemSearch            = "search"
	MetricsSubsystemLogging           = "logging"
	MetricsSubsystemRemoteCluster     = "remote_cluster"
	MetricsSubsystemSharedChannels    = "shared_channels"
	MetricsSubsystemSystem            = "system"
	MetricsSubsystemJobs              = "jobs"
	MetricsSubsystemNotifications     = "notifications"
	MetricsSubsystemClientsMobileApp  = "mobileapp"
	MetricsSubsystemClientsWeb        = "webapp"
	MetricsSubsystemClientsDesktopApp = "desktopapp"
	MetricsSubsystemAccessControl     = "access_control"
)

// SznMetrics implements the einterfaces.MetricsInterface
type SznMetrics struct {
	Platform *platform.PlatformService
	Registry *prometheus.Registry
	logger   *mlog.Logger

	// Database metrics
	dbCollectors map[string]*dbCollector
	dbMutex      sync.RWMutex

	// Additional labels for cloud installations
	additionalLabels map[string]string

	// Post metrics
	postCreateCounter     prometheus.Counter
	webhookPostCounter    prometheus.Counter
	postSentEmailCounter  prometheus.Counter
	postSentPushCounter   prometheus.Counter
	postBroadcastCounter  prometheus.Counter
	postFileAttachCounter prometheus.Counter
	postIndexCounter      prometheus.Counter

	// HTTP metrics
	httpRequestsCounter prometheus.Counter
	httpErrorsCounter   prometheus.Counter
	httpWebsocketsGauge *prometheus.GaugeVec
	apiEndpointDuration *prometheus.HistogramVec

	// Cluster metrics
	clusterRequestsDuration prometheus.Histogram
	clusterRequestsCounter  prometheus.Counter
	clusterEventTypeCounter *prometheus.CounterVec

	// Cache metrics
	memCacheHitCounters          *prometheus.CounterVec
	memCacheMissCounters         *prometheus.CounterVec
	memCacheInvalidationCounters *prometheus.CounterVec
	memCacheHitCounterSession    prometheus.Counter
	memCacheMissCounterSession   prometheus.Counter
	memCacheInvalidationSession  prometheus.Counter
	etagHitCounters              *prometheus.CounterVec
	etagMissCounters             *prometheus.CounterVec
	redisEndpointDuration        *prometheus.HistogramVec

	// Websocket metrics
	websocketEventCounters            *prometheus.CounterVec
	websocketBroadcastCounters        *prometheus.CounterVec
	websocketBroadcastBufferSize      *prometheus.GaugeVec
	websocketBroadcastUsersRegistered *prometheus.GaugeVec
	websocketReconnectCounter         *prometheus.CounterVec

	// Search metrics
	postsSearchCounter  prometheus.Counter
	postsSearchDuration prometheus.Histogram
	filesSearchCounter  prometheus.Counter
	filesSearchDuration prometheus.Histogram
	fileIndexCounter    prometheus.Counter
	userIndexCounter    prometheus.Counter
	channelIndexCounter prometheus.Counter
	storeMethodDuration *prometheus.HistogramVec

	// Plugin metrics
	pluginHookDuration          *prometheus.HistogramVec
	pluginMultiHookIterDuration *prometheus.HistogramVec
	pluginMultiHookDuration     prometheus.Histogram
	pluginAPIDuration           *prometheus.HistogramVec

	// Shared Channels metrics
	sharedChannelsSyncCounter                *prometheus.CounterVec
	sharedChannelsTaskInQueueDuration        prometheus.Histogram
	sharedChannelsQueueSize                  prometheus.Gauge
	sharedChannelsSyncCollectionDuration     *prometheus.HistogramVec
	sharedChannelsSyncSendDuration           *prometheus.HistogramVec
	sharedChannelsSyncCollectionStepDuration *prometheus.HistogramVec
	sharedChannelsSyncSendStepDuration       *prometheus.HistogramVec

	// Notification metrics
	notificationCounter            *prometheus.CounterVec
	notificationAckCounter         *prometheus.CounterVec
	notificationSuccessCounter     *prometheus.CounterVec
	notificationErrorCounter       *prometheus.CounterVec
	notificationNotSentCounter     *prometheus.CounterVec
	notificationUnsupportedCounter *prometheus.CounterVec

	// Job metrics
	jobActiveGauge     *prometheus.GaugeVec
	replicaLagAbsolute *prometheus.GaugeVec
	replicaLagTime     *prometheus.GaugeVec
	enabledUsersGauge  prometheus.Gauge

	// Login metrics
	loginCounter     prometheus.Counter
	loginFailCounter prometheus.Counter

	// Remote Cluster metrics
	remoteClusterMsgSent         *prometheus.CounterVec
	remoteClusterMsgReceived     *prometheus.CounterVec
	remoteClusterMsgErrors       *prometheus.CounterVec
	remoteClusterPingDuration    *prometheus.HistogramVec
	remoteClusterClockSkew       *prometheus.GaugeVec
	remoteClusterConnStateChange *prometheus.CounterVec

	// Access Control metrics
	accessControlSearchQueryDuration       prometheus.Histogram
	accessControlExpressionCompileDuration prometheus.Histogram
	accessControlEvaluateDuration          prometheus.Histogram
	accessControlCacheInvalidation         prometheus.Counter

	// Client metrics
	clientTimeToFirstByte             *prometheus.HistogramVec
	clientTimeToLastByte              *prometheus.HistogramVec
	clientTimeToDomInteractive        *prometheus.HistogramVec
	clientSplashScreenEnd             *prometheus.HistogramVec
	clientFirstContentfulPaint        *prometheus.HistogramVec
	clientLargestContentfulPaint      *prometheus.HistogramVec
	clientInteractionToNextPaint      *prometheus.HistogramVec
	clientCumulativeLayoutShift       *prometheus.HistogramVec
	clientLongTasks                   *prometheus.CounterVec
	clientPageLoadDuration            *prometheus.HistogramVec
	clientChannelSwitchDuration       *prometheus.HistogramVec
	clientTeamSwitchDuration          *prometheus.HistogramVec
	clientRHSLoadDuration             *prometheus.HistogramVec
	globalThreadsLoadDuration         *prometheus.HistogramVec
	mobileClientLoadDuration          *prometheus.HistogramVec
	mobileClientChannelSwitchDuration *prometheus.HistogramVec
	mobileClientTeamSwitchDuration    *prometheus.HistogramVec
	mobileClientNetworkMetrics        *prometheus.HistogramVec
	mobileClientSessionMetadata       *prometheus.GaugeVec
	desktopCpuUsage                   *prometheus.GaugeVec
	desktopMemoryUsage                *prometheus.GaugeVec
}

type dbCollector struct {
	db        *sql.DB
	name      string
	collector prometheus.Collector
}

// New creates a new SznMetrics instance
func New(ps *platform.PlatformService, driver, dataSource string) *SznMetrics {
	m := &SznMetrics{
		Platform:         ps,
		logger:           ps.Log().(*mlog.Logger),
		dbCollectors:     make(map[string]*dbCollector),
		additionalLabels: make(map[string]string),
	}

	// Initialize Prometheus registry
	m.Registry = prometheus.NewRegistry()

	// Register standard collectors
	options := collectors.ProcessCollectorOpts{
		Namespace: MetricsNamespace,
	}
	m.Registry.MustRegister(collectors.NewProcessCollector(options))
	m.Registry.MustRegister(collectors.NewGoCollector())

	// Check for cloud installation labels
	if installationID := os.Getenv("MM_CLOUD_INSTALLATION_ID"); installationID != "" {
		m.additionalLabels["installationId"] = installationID
		if groupID := os.Getenv("MM_CLOUD_GROUP_ID"); groupID != "" {
			m.additionalLabels["installationGroupId"] = groupID
		}
	}

	m.logger.Info("SznMetrics: Initializing Prometheus metrics", mlog.Int("additional_labels", len(m.additionalLabels)))

	// Initialize all metric collectors
	m.initPostMetrics()
	m.initHTTPMetrics()
	m.initClusterMetrics()
	m.initLoginMetrics()
	m.initCacheMetrics()
	m.initWebsocketMetrics()
	m.initSearchMetrics()
	m.initPluginMetrics()
	m.initRemoteClusterMetrics()
	m.initSharedChannelsMetrics()
	m.initJobMetrics()
	m.initNotificationMetrics()
	m.initClientMetrics()
	m.initAccessControlMetrics()

	return m
}

// Register registers the metrics HTTP handler
func (m *SznMetrics) Register() {
	m.logger.Info("SznMetrics: Registering /metrics endpoint")
	m.Platform.HandleMetrics("/metrics", promhttp.HandlerFor(m.Registry, promhttp.HandlerOpts{}))
}

// RegisterDBCollector registers a database connection pool for metrics collection
func (m *SznMetrics) RegisterDBCollector(db *sql.DB, name string) {
	if db == nil {
		m.logger.Warn("SznMetrics: Attempted to register nil database collector", mlog.String("name", name))
		return
	}

	collector := collectors.NewDBStatsCollector(db, name)
	if err := m.Registry.Register(collector); err != nil {
		m.logger.Error("SznMetrics: Failed to register DB collector", mlog.String("name", name), mlog.Err(err))
		return
	}

	m.dbMutex.Lock()
	defer m.dbMutex.Unlock()
	m.dbCollectors[name] = &dbCollector{
		db:        db,
		name:      name,
		collector: collector,
	}

	m.logger.Info("SznMetrics: Database collector registered", mlog.String("name", name))
}

// UnregisterDBCollector unregisters a database connection pool
func (m *SznMetrics) UnregisterDBCollector(db *sql.DB, name string) {
	m.dbMutex.Lock()
	defer m.dbMutex.Unlock()

	if collector, ok := m.dbCollectors[name]; ok {
		m.Registry.Unregister(collector.collector)
		delete(m.dbCollectors, name)
		m.logger.Info("SznMetrics: Database collector unregistered", mlog.String("name", name))
	}
}

// GetLoggerMetricsCollector returns the logger metrics collector
func (m *SznMetrics) GetLoggerMetricsCollector() mlog.MetricsCollector {
	// Return nil for now, can be implemented later if needed
	return nil
}
