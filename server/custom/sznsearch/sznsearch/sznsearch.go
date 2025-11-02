package sznsearch

import (
	"crypto/tls"
	"crypto/x509"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/elastic/go-elasticsearch/v8"
	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/shared/mlog"
	"github.com/mattermost/mattermost/server/v8/channels/app/platform"
	"github.com/mattermost/mattermost/server/v8/custom/sznsearch/common"
)

const sznsearchMaxVersion = 8

// SznSearchImpl implements searchengine.SearchEngineInterface for Seznam Search (ElasticSearch)
type SznSearchImpl struct {
	Platform *platform.PlatformService

	client      *elasticsearch.Client
	ready       int32
	version     int
	fullVersion string
	plugins     []string

	mutex    *common.KeyedMutex
	stopChan chan struct{}

	// Indexer state
	messageQueue     map[string]*common.IndexedMessage // postId -> IndexedMessage
	workerState      string
	batchSize        int
	ignoreChannels   map[string]bool // channelId -> true (for fast O(1) lookup)
	ignoreTeams      map[string]bool // teamId -> true (for fast O(1) lookup)
	indexerWorkers   int
	reindexOnStartup bool

	// Reindex state tracking
	reindexMutex    sync.RWMutex
	runningReindex  *common.ReindexInfo // nil if no reindex is running
	reindexPoolSize int                 // Number of concurrent goroutines for channel reindex
}

// UpdateConfig updates the engine configuration
func (*SznSearchImpl) UpdateConfig(cfg *model.Config) {
	// Configuration is accessed via Platform.Config() which is always current
}

// GetName returns the engine name
func (*SznSearchImpl) GetName() string {
	return "sznsearch"
}

// IsEnabled checks if indexing is enabled in configuration
func (s *SznSearchImpl) IsEnabled() bool {
	return *s.Platform.Config().SznSearchSettings.EnableIndexing
}

// IsActive checks if the engine is ready and active
func (s *SznSearchImpl) IsActive() bool {
	return *s.Platform.Config().SznSearchSettings.EnableIndexing && atomic.LoadInt32(&s.ready) == 1
}

// IsIndexingEnabled checks if indexing is enabled
func (s *SznSearchImpl) IsIndexingEnabled() bool {
	return *s.Platform.Config().SznSearchSettings.EnableIndexing
}

// IsSearchEnabled checks if searching is enabled
func (s *SznSearchImpl) IsSearchEnabled() bool {
	return *s.Platform.Config().SznSearchSettings.EnableSearching
}

// IsAutocompletionEnabled checks if autocomplete is enabled
func (s *SznSearchImpl) IsAutocompletionEnabled() bool {
	return *s.Platform.Config().SznSearchSettings.EnableAutocomplete
}

// IsIndexingSync returns true if indexing should be synchronous
func (s *SznSearchImpl) IsIndexingSync() bool {
	return *s.Platform.Config().SznSearchSettings.LiveIndexingBatchSize <= 1
}

// GetFullVersion returns the full ElasticSearch version
func (s *SznSearchImpl) GetFullVersion() string {
	return s.fullVersion
}

// GetVersion returns the major version number
func (s *SznSearchImpl) GetVersion() int {
	return s.version
}

// GetPlugins returns the list of ElasticSearch plugins
func (s *SznSearchImpl) GetPlugins() []string {
	return s.plugins
}

// NewSznSearchEngine creates a new SznSearch engine instance
func NewSznSearchEngine(ps *platform.PlatformService) *SznSearchImpl {
	ps.Log().Info("SznSearch: Creating new engine instance")

	cfg := ps.Config().SznSearchSettings

	// Parse ignore channels (comma-separated channelIds)
	ignoreChannels := make(map[string]bool)
	if *cfg.IgnoreChannels != "" {
		for _, channelID := range strings.Split(*cfg.IgnoreChannels, ",") {
			trimmed := strings.TrimSpace(channelID)
			if trimmed != "" {
				ignoreChannels[trimmed] = true
			}
		}
	}

	// Parse ignore teams (comma-separated teamIds)
	ignoreTeams := make(map[string]bool)
	if *cfg.IgnoreTeams != "" {
		for _, teamID := range strings.Split(*cfg.IgnoreTeams, ",") {
			trimmed := strings.TrimSpace(teamID)
			if trimmed != "" {
				ignoreTeams[trimmed] = true
			}
		}
	}

	ps.Log().Info("SznSearch: Configuration loaded",
		mlog.Int("ignored_channels", len(ignoreChannels)),
		mlog.Int("ignored_teams", len(ignoreTeams)),
		mlog.Int("batch_size", *cfg.BatchSize),
		mlog.Int("request_timeout", *cfg.RequestTimeoutSeconds),
		mlog.Int("reindex_pool_size", *cfg.ReindexChannelPool),
	)

	return &SznSearchImpl{
		Platform:         ps,
		mutex:            common.NewKeyedMutex(),
		stopChan:         make(chan struct{}),
		messageQueue:     make(map[string]*common.IndexedMessage),
		workerState:      common.WorkerStateStopped,
		batchSize:        *cfg.BatchSize,
		ignoreChannels:   ignoreChannels,
		ignoreTeams:      ignoreTeams,
		indexerWorkers:   4,
		reindexOnStartup: false,
		reindexPoolSize:  *cfg.ReindexChannelPool,
	}
}

// createClient creates a new ElasticSearch client with proper TLS configuration
func (s *SznSearchImpl) createClient() (*elasticsearch.Client, error) {
	s.mutex.WLock(common.MutexNewESClient)
	defer s.mutex.WUnlock(common.MutexNewESClient)

	s.Platform.Log().Info("SznSearch: Creating ElasticSearch client")

	cfg := s.Platform.Config().SznSearchSettings
	var config elasticsearch.Config

	// Calculate connection limits based on reindex pool size (used for both TLS and username/password)
	// MaxConnsPerHost = max(10, reindexPoolSize + 10) to handle parallel reindex + live indexing
	// MaxIdleConnsPerHost = min(10, reindexPoolSize / 2) to keep some connections warm
	poolSize := *cfg.ReindexChannelPool
	maxConns := 10
	if poolSize+10 > maxConns {
		maxConns = poolSize + 10
	}
	maxIdleConns := poolSize / 2
	if maxIdleConns > 10 {
		maxIdleConns = 10
	}
	timeout := time.Duration(*cfg.RequestTimeoutSeconds) * time.Second

	// Check if TLS credentials are provided (client certificate authentication from config)
	if *cfg.ClientCert != "" && *cfg.ClientKey != "" {
		s.Platform.Log().Info("SznSearch: Using TLS client certificate authentication",
			mlog.String("client_cert", *cfg.ClientCert),
			mlog.String("connection_url", *cfg.ConnectionURL),
		)

		// Load client certificate and private key from config
		cert, err := tls.LoadX509KeyPair(*cfg.ClientCert, *cfg.ClientKey)
		if err != nil {
			return nil, model.NewAppError("SznSearch.createClient", "sznsearch.create_client.load_cert", nil, err.Error(), http.StatusInternalServerError)
		}

		// Create a certificate pool for the CA certificate if provided
		var caCertPool *x509.CertPool
		if *cfg.CA != "" {
			caCert, err := os.ReadFile(*cfg.CA)
			if err != nil {
				return nil, model.NewAppError("SznSearch.createClient", "sznsearch.create_client.load_ca", nil, err.Error(), http.StatusInternalServerError)
			}
			caCertPool = x509.NewCertPool()
			if !caCertPool.AppendCertsFromPEM(caCert) {
				return nil, model.NewAppError("SznSearch.createClient", "sznsearch.create_client.append_ca", nil, "", http.StatusInternalServerError)
			}
		}

		// Configure TLS settings
		tlsConfig := &tls.Config{
			Certificates:       []tls.Certificate{cert},
			RootCAs:            caCertPool,
			InsecureSkipVerify: *cfg.SkipTLSVerification,
		}

		// Create a custom HTTP transport with TLS and timeout
		transport := &http.Transport{
			TLSClientConfig:       tlsConfig,
			ResponseHeaderTimeout: timeout,
			DisableKeepAlives:     false,
			MaxConnsPerHost:       maxConns,
			MaxIdleConnsPerHost:   maxIdleConns,
		}

		// Set up the Elasticsearch client with the custom transport
		config = elasticsearch.Config{
			Addresses: []string{*cfg.ConnectionURL},
			Transport: transport,
		}
	} else {
		s.Platform.Log().Info("SznSearch: Using username/password authentication",
			mlog.String("username", *cfg.Username),
			mlog.String("connection_url", *cfg.ConnectionURL),
		)

		// Fallback to username and password authentication with timeout
		transport := &http.Transport{
			ResponseHeaderTimeout: timeout,
			DisableKeepAlives:     false,
			MaxConnsPerHost:       maxConns,
			MaxIdleConnsPerHost:   maxIdleConns,
		}

		config = elasticsearch.Config{
			Addresses: []string{*cfg.ConnectionURL},
			Username:  *cfg.Username,
			Password:  *cfg.Password,
			Transport: transport,
		}
	}

	s.Platform.Log().Info("SznSearch: ElasticSearch client configured",
		mlog.Duration("request_timeout", timeout),
		mlog.Int("max_conns_per_host", maxConns),
		mlog.Int("max_idle_conns_per_host", maxIdleConns),
	)

	client, err := elasticsearch.NewClient(config)
	if err != nil {
		return nil, err
	}

	s.Platform.Log().Info("SznSearch: ElasticSearch client created successfully")
	return client, nil
}

// Start initializes and starts the SznSearch engine
func (s *SznSearchImpl) Start() *model.AppError {
	s.Platform.Log().Info("SznSearch: Starting engine initialization")

	// Check if indexing is enabled in config (no license required for custom implementation)
	if !*s.Platform.Config().SznSearchSettings.EnableIndexing {
		s.Platform.Log().Info("SznSearch: Indexing is disabled in config, skipping initialization")
		return nil
	}

	s.mutex.WLock(common.MutexWorkerState)
	if atomic.LoadInt32(&s.ready) != 0 {
		s.mutex.WUnlock(common.MutexWorkerState)
		s.Platform.Log().Info("SznSearch: Engine already started")
		return nil // Already started
	}
	s.workerState = common.WorkerStateStarting
	s.mutex.WUnlock(common.MutexWorkerState)

	s.Platform.Log().Info("SznSearch: Creating ElasticSearch client connection")

	// Create ES client
	client, err := s.createClient()
	if err != nil {
		if appErr, ok := err.(*model.AppError); ok {
			s.Platform.Log().Error("SznSearch: Failed to create client", mlog.Err(appErr))
			return appErr
		}
		s.Platform.Log().Error("SznSearch: Failed to create client", mlog.Err(err))
		return model.NewAppError("SznSearch.Start", "sznsearch.start.create_client", nil, err.Error(), http.StatusInternalServerError)
	}

	s.Platform.Log().Info("SznSearch: Testing ElasticSearch connection")

	// Test connection
	info, esErr := client.Info()
	if esErr != nil {
		s.Platform.Log().Error("SznSearch: Connection test failed", mlog.Err(esErr))
		return model.NewAppError("SznSearch.Start", "sznsearch.start.connection_failed", nil, esErr.Error(), http.StatusInternalServerError)
	}
	defer info.Body.Close()

	if info.IsError() {
		s.Platform.Log().Error("SznSearch: Connection error",
			mlog.Int("status_code", info.StatusCode),
			mlog.String("response", info.String()),
		)
		return model.NewAppError("SznSearch.Start", "sznsearch.start.connection_error", nil, info.String(), http.StatusInternalServerError)
	}

	s.Platform.Log().Info("SznSearch: Connection test successful")

	s.client = client
	s.fullVersion = "8.0.0" // Would parse from info
	s.version = 8
	s.plugins = []string{} // Would get from cluster state

	s.Platform.Log().Info("SznSearch: Ensuring indices exist")

	// Ensure indices exist
	if appErr := s.ensureIndices(); appErr != nil {
		s.Platform.Log().Error("SznSearch: Failed to ensure indices", mlog.Err(appErr))
		return appErr
	}

	s.Platform.Log().Info("SznSearch: Indices verified successfully")

	atomic.StoreInt32(&s.ready, 1)
	s.mutex.WLock(common.MutexWorkerState)
	s.workerState = common.WorkerStateIdle
	s.mutex.WUnlock(common.MutexWorkerState)

	s.Platform.Log().Info("SznSearch: Engine started successfully, launching background indexer")

	// Start background indexer
	go s.startIndexer()

	// Register slash command
	s.RegisterCommand()

	return nil
}

// Stop stops the SznSearch engine
func (s *SznSearchImpl) Stop() *model.AppError {
	if atomic.LoadInt32(&s.ready) == 0 {
		return nil // Not running
	}

	s.Platform.Log().Info("Stopping SznSearch engine")

	s.mutex.WLock(common.MutexWorkerState)
	s.workerState = common.WorkerStateStopping
	s.mutex.WUnlock(common.MutexWorkerState)

	// Signal indexer to stop
	close(s.stopChan)

	atomic.StoreInt32(&s.ready, 0)

	s.mutex.WLock(common.MutexWorkerState)
	s.workerState = common.WorkerStateStopped
	s.mutex.WUnlock(common.MutexWorkerState)

	s.Platform.Log().Info("SznSearch engine stopped")

	return nil
}

// startReindex attempts to start a reindex operation
// Returns an error if another reindex is already running
func (s *SznSearchImpl) startReindex(info *common.ReindexInfo) *model.AppError {
	s.reindexMutex.Lock()
	defer s.reindexMutex.Unlock()

	if s.runningReindex != nil {
		return model.NewAppError(
			"SznSearch.startReindex",
			"sznsearch.reindex.already_running",
			map[string]any{
				"Type":      s.runningReindex.Type,
				"TargetID":  s.runningReindex.TargetID,
				"UserID":    s.runningReindex.UserID,
				"StartedAt": s.runningReindex.StartedAt,
			},
			"Another reindex operation is already running",
			http.StatusConflict,
		)
	}

	s.runningReindex = info
	s.Platform.Log().Info("SznSearch: Reindex started",
		mlog.String("type", string(info.Type)),
		mlog.String("target_id", info.TargetID),
		mlog.String("user_id", info.UserID),
	)

	return nil
}

// stopReindex marks the current reindex operation as completed
func (s *SznSearchImpl) stopReindex() {
	s.reindexMutex.Lock()
	defer s.reindexMutex.Unlock()

	if s.runningReindex != nil {
		s.Platform.Log().Info("SznSearch: Reindex completed",
			mlog.String("type", string(s.runningReindex.Type)),
			mlog.String("target_id", s.runningReindex.TargetID),
		)
		s.runningReindex = nil
	}
}

// getRunningReindex returns information about the currently running reindex
// Returns nil if no reindex is running
func (s *SznSearchImpl) getRunningReindex() *common.ReindexInfo {
	s.reindexMutex.RLock()
	defer s.reindexMutex.RUnlock()

	if s.runningReindex == nil {
		return nil
	}

	// Return a copy to avoid data races
	infoCopy := *s.runningReindex
	return &infoCopy
}
