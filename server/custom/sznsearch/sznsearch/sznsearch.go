package sznsearch

import (
	"crypto/tls"
	"crypto/x509"
	"net/http"
	"os"
	"sync/atomic"

	"github.com/elastic/go-elasticsearch/v8"
	"github.com/mattermost/mattermost/server/public/model"
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

	mutex        *common.KeyedMutex
	channelMutex *common.KeyedMutex
	stopChan     chan struct{}

	// Indexer state
	messageQueue     map[string][]*common.IndexedMessage
	workerState      string
	batchSize        int
	limitToTeams     map[string]bool
	ignoreChannels   map[string]bool
	indexerWorkers   int
	reindexOnStartup bool
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
	return *s.Platform.Config().ElasticsearchSettings.EnableIndexing
}

// IsActive checks if the engine is ready and active
func (s *SznSearchImpl) IsActive() bool {
	return *s.Platform.Config().ElasticsearchSettings.EnableIndexing && atomic.LoadInt32(&s.ready) == 1
}

// IsIndexingEnabled checks if indexing is enabled
func (s *SznSearchImpl) IsIndexingEnabled() bool {
	return *s.Platform.Config().ElasticsearchSettings.EnableIndexing
}

// IsSearchEnabled checks if searching is enabled
func (s *SznSearchImpl) IsSearchEnabled() bool {
	return *s.Platform.Config().ElasticsearchSettings.EnableSearching
}

// IsAutocompletionEnabled checks if autocomplete is enabled
func (s *SznSearchImpl) IsAutocompletionEnabled() bool {
	return *s.Platform.Config().ElasticsearchSettings.EnableAutocomplete
}

// IsIndexingSync returns true if indexing should be synchronous
func (s *SznSearchImpl) IsIndexingSync() bool {
	return *s.Platform.Config().ElasticsearchSettings.LiveIndexingBatchSize <= 1
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
	return &SznSearchImpl{
		Platform:         ps,
		mutex:            common.NewKeyedMutex(),
		channelMutex:     common.NewKeyedMutex(),
		stopChan:         make(chan struct{}),
		messageQueue:     make(map[string][]*common.IndexedMessage),
		workerState:      common.WorkerStateStopped,
		batchSize:        100,
		limitToTeams:     make(map[string]bool),
		ignoreChannels:   make(map[string]bool),
		indexerWorkers:   4,
		reindexOnStartup: false,
	}
}

// createClient creates a new ElasticSearch client with proper TLS configuration
func (s *SznSearchImpl) createClient() (*elasticsearch.Client, error) {
	s.mutex.WLock(common.MutexNewESClient)
	defer s.mutex.WUnlock(common.MutexNewESClient)

	cfg := s.Platform.Config().ElasticsearchSettings
	var config elasticsearch.Config

	// Check if TLS credentials are provided (client certificate authentication from config)
	if *cfg.ClientCert != "" && *cfg.ClientKey != "" {
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

		// Create a custom HTTP transport
		transport := &http.Transport{
			TLSClientConfig:     tlsConfig,
			DisableKeepAlives:   false,
			MaxConnsPerHost:     10,
			MaxIdleConnsPerHost: 10,
		}

		// Set up the Elasticsearch client with the custom transport
		config = elasticsearch.Config{
			Addresses: []string{*cfg.ConnectionURL},
			Transport: transport,
		}
	} else {
		// Fallback to username and password authentication
		config = elasticsearch.Config{
			Addresses: []string{*cfg.ConnectionURL},
			Username:  *cfg.Username,
			Password:  *cfg.Password,
		}
	}

	return elasticsearch.NewClient(config)
}

// Start initializes and starts the SznSearch engine
func (s *SznSearchImpl) Start() *model.AppError {
	// Check if indexing is enabled in config (no license required for custom implementation)
	if !*s.Platform.Config().ElasticsearchSettings.EnableIndexing {
		return nil
	}

	s.mutex.WLock(common.MutexWorkerState)
	if atomic.LoadInt32(&s.ready) != 0 {
		s.mutex.WUnlock(common.MutexWorkerState)
		return nil // Already started
	}
	s.workerState = common.WorkerStateStarting
	s.mutex.WUnlock(common.MutexWorkerState)

	// Create ES client
	client, err := s.createClient()
	if err != nil {
		if appErr, ok := err.(*model.AppError); ok {
			return appErr
		}
		return model.NewAppError("SznSearch.Start", "sznsearch.start.create_client", nil, err.Error(), http.StatusInternalServerError)
	}

	// Test connection
	info, esErr := client.Info()
	if esErr != nil {
		return model.NewAppError("SznSearch.Start", "sznsearch.start.connection_failed", nil, esErr.Error(), http.StatusInternalServerError)
	}
	defer info.Body.Close()

	if info.IsError() {
		return model.NewAppError("SznSearch.Start", "sznsearch.start.connection_error", nil, info.String(), http.StatusInternalServerError)
	}

	s.client = client
	s.fullVersion = "8.0.0" // Would parse from info
	s.version = 8
	s.plugins = []string{} // Would get from cluster state

	// Ensure indices exist
	if appErr := s.ensureIndices(); appErr != nil {
		return appErr
	}

	atomic.StoreInt32(&s.ready, 1)
	s.mutex.WLock(common.MutexWorkerState)
	s.workerState = common.WorkerStateIdle
	s.mutex.WUnlock(common.MutexWorkerState)

	s.Platform.Log().Info("SznSearch engine started successfully")

	// Start background indexer
	go s.startIndexer()

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
