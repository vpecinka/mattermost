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

	// Additional labels for cloud installations
	additionalLabels map[string]string
}

type dbCollector struct {
	db   *sql.DB
	name string
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

	m.dbCollectors[name] = &dbCollector{
		db:   db,
		name: name,
	}

	m.logger.Info("SznMetrics: Database collector registered", mlog.String("name", name))
}

// UnregisterDBCollector unregisters a database connection pool
func (m *SznMetrics) UnregisterDBCollector(db *sql.DB, name string) {
	delete(m.dbCollectors, name)
	m.logger.Info("SznMetrics: Database collector unregistered", mlog.String("name", name))
}

// GetLoggerMetricsCollector returns the logger metrics collector
func (m *SznMetrics) GetLoggerMetricsCollector() mlog.MetricsCollector {
	// Return nil for now, can be implemented later if needed
	return nil
}
