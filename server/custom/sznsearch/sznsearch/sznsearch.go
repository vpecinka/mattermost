package sznsearch

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strconv"
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

// SznSearchImpl implements searchengine.SearchEngineInterface for Seznam Search (ElasticSearch)
type SznSearchImpl struct {
	Platform *platform.PlatformService

	client         *elasticsearch.Client
	ready          int32 // 0=stopped, 1=running (health tracked by circuitBreaker)
	circuitBreaker *common.CircuitBreaker
	version        int
	fullVersion    string
	plugins        []string

	mutex    *common.KeyedMutex
	stopChan chan struct{}

	// Indexer state
	messageQueue     map[string]*common.IndexedMessage // postId -> IndexedMessage
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
	return atomic.LoadInt32(&s.ready) > 0
}

// IsIndexingEnabled checks if indexing is enabled
func (s *SznSearchImpl) IsIndexingEnabled() bool {
	return atomic.LoadInt32(&s.ready) > 0 && *s.Platform.Config().SznSearchSettings.EnableIndexing
}

// IsSearchEnabled checks if searching is enabled
func (s *SznSearchImpl) IsSearchEnabled() bool {
	return atomic.LoadInt32(&s.ready) > 0 && *s.Platform.Config().SznSearchSettings.EnableSearching
}

// isBackendHealthy checks if ES backend is available via circuit breaker
func (s *SznSearchImpl) isBackendHealthy() bool {
	return atomic.LoadInt32(&s.ready) == 1 && s.circuitBreaker.AllowRequest()
}

// IsIndexingSync returns true if indexing should be synchronous
// Always returns false - indexing is always async via message queue
func (s *SznSearchImpl) IsIndexingSync() bool {
	return false
}

// IsAutocompletionEnabled checks if autocomplete is enabled
func (s *SznSearchImpl) IsAutocompletionEnabled() bool {
	return *s.Platform.Config().SznSearchSettings.EnableAutocomplete
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

// parseVersionFromInfo parses ES version from client.Info() response
// Returns fullVersion string and major version int
// Returns "0.0.0" and 0 if parsing fails
func parseVersionFromInfo(infoBody io.Reader) (string, int) {
	var info struct {
		Version struct {
			Number string `json:"number"`
		} `json:"version"`
	}

	if err := json.NewDecoder(infoBody).Decode(&info); err != nil {
		return "0.0.0", 0
	}

	fullVersion := info.Version.Number
	if fullVersion == "" {
		return "0.0.0", 0
	}

	// Parse major version (e.g., "8.10.2" -> 8)
	parts := strings.Split(fullVersion, ".")
	if len(parts) == 0 {
		return fullVersion, 0
	}

	majorVersion, err := strconv.Atoi(parts[0])
	if err != nil {
		return fullVersion, 0
	}

	return fullVersion, majorVersion
}

// NewSznSearchEngine creates a new SznSearch engine instance
func NewSznSearchEngine(ps *platform.PlatformService) *SznSearchImpl {
	ps.Log().Debug("SznSearch: Creating new engine instance")

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

	ps.Log().Debug("SznSearch: Configuration loaded",
		mlog.Int("ignored_channels", len(ignoreChannels)),
		mlog.Int("ignored_teams", len(ignoreTeams)),
		mlog.Int("batch_size", *cfg.BatchSize),
		mlog.Int("request_timeout", *cfg.RequestTimeoutSeconds),
		mlog.Int("reindex_pool_size", *cfg.ReindexChannelPool),
	)

	// Create circuit breaker with config
	circuitBreaker := common.NewCircuitBreaker(
		*cfg.CircuitBreakerMaxFailures,
		time.Duration(*cfg.CircuitBreakerCooldownSec)*time.Second,
	)

	return &SznSearchImpl{
		Platform:         ps,
		circuitBreaker:   circuitBreaker,
		mutex:            common.NewKeyedMutex(),
		stopChan:         make(chan struct{}),
		messageQueue:     make(map[string]*common.IndexedMessage),
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
	s.Platform.Log().Debug("SznSearch: Creating ElasticSearch client")

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
		s.Platform.Log().Debug("SznSearch: Using TLS client certificate authentication",
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
	s.Platform.Log().Debug("SznSearch: Starting engine initialization")

	// Check if indexing is enabled in config (no license required for custom implementation)
	if !*s.Platform.Config().SznSearchSettings.EnableIndexing {
		s.Platform.Log().Info("SznSearch: Indexing is disabled in config, skipping initialization")
		return nil
	}

	if atomic.LoadInt32(&s.ready) != 0 {
		s.Platform.Log().Info("SznSearch: Engine already started")
		return nil // Already started
	}

	// Keep ready=0 until client is successfully created
	s.Platform.Log().Debug("SznSearch: Creating ElasticSearch client connection")

	// Create ES client with retry - CRITICAL: must succeed to set ready=1
	err := common.RetryWithBackoff(3, 500*time.Millisecond, 5*time.Second, s.Platform.Log(), func() error {
		client, createErr := s.createClient()
		if createErr != nil {
			return createErr
		}
		s.client = client
		return nil
	})

	if err != nil {
		s.Platform.Log().Error("SznSearch: FATAL - Failed to create client, engine will not start", mlog.Err(err))
		// Network/timeout errors should trigger circuit breaker
		// Note: 4xx errors are wrapped in NonRetryableError by retry wrapper
		var appErr *model.AppError
		if errors.As(err, &appErr) && appErr.StatusCode >= 500 {
			s.circuitBreaker.RecordFailure()
		} else if !errors.As(err, &appErr) {
			s.circuitBreaker.RecordFailure()
		}
		return model.NewAppError("SznSearch.Start", "sznsearch.start.client_creation_failed", nil, err.Error(), http.StatusInternalServerError)
	}

	// Client created successfully - now set ready=1
	atomic.StoreInt32(&s.ready, 1)
	s.Platform.Log().Info("SznSearch: Client created successfully, engine is now READY")

	// Test connection with retry (non-fatal if it fails)
	s.Platform.Log().Debug("SznSearch: Testing ElasticSearch connection")
	testErr := common.RetryWithBackoff(3, 500*time.Millisecond, 5*time.Second, s.Platform.Log(), func() error {
		info, esErr := s.client.Info()
		if esErr != nil {
			return esErr
		}
		defer info.Body.Close()

		if info.IsError() {
			return model.NewAppError("Start", "es_connection_error", nil, info.String(), info.StatusCode)
		}

		// Parse version from Info response
		fullVer, majorVer := parseVersionFromInfo(info.Body)
		s.fullVersion = fullVer
		s.version = majorVer
		s.plugins = []string{}
		return nil
	})

	if testErr != nil {
		s.Platform.Log().Warn("SznSearch: Connection test failed - circuit breaker will handle recovery", mlog.Err(testErr))
		// Network/timeout errors should trigger circuit breaker
		var appErr *model.AppError
		if errors.As(testErr, &appErr) {
			if appErr.StatusCode < 400 || appErr.StatusCode >= 500 {
				s.circuitBreaker.RecordFailure()
			}
		} else {
			s.circuitBreaker.RecordFailure()
		}
		s.fullVersion = "0.0.0"
		s.version = 0
		s.plugins = []string{}
	} else {
		s.Platform.Log().Info("SznSearch: Connection test successful",
			mlog.String("es_version", s.fullVersion),
			mlog.Int("es_major_version", s.version),
		)

		s.Platform.Log().Debug("SznSearch: Ensuring indices exist")

		// Ensure indices exist with retry (non-fatal if it fails)
		indicesErr := common.RetryWithBackoff(3, 500*time.Millisecond, 5*time.Second, s.Platform.Log(), func() error {
			return s.ensureIndices()
		})

		if indicesErr != nil {
			s.Platform.Log().Warn("SznSearch: Failed to ensure indices - circuit breaker will handle recovery", mlog.Err(indicesErr))
			// Network/timeout errors should trigger circuit breaker
			var appErr *model.AppError
			if errors.As(indicesErr, &appErr) {
				if appErr.StatusCode < 400 || appErr.StatusCode >= 500 {
					s.circuitBreaker.RecordFailure()
				}
			} else {
				s.circuitBreaker.RecordFailure()
			}
		} else {
			s.Platform.Log().Info("SznSearch: Indices verified successfully - marking circuit breaker as healthy")
			s.circuitBreaker.RecordSuccess()
		}
	}

	cbState := s.circuitBreaker.GetState()
	s.Platform.Log().Info("SznSearch: Engine started, launching background workers",
		mlog.Int("ready_state", int(atomic.LoadInt32(&s.ready))),
		mlog.String("circuit_breaker_state", string(cbState)))

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

	// Signal indexer to stop
	close(s.stopChan)

	atomic.StoreInt32(&s.ready, 0)

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
	s.Platform.Log().Debug("SznSearch: Reindex started",
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
		s.Platform.Log().Debug("SznSearch: Reindex completed",
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
