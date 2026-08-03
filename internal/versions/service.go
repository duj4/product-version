package versions

import (
	"sync"
	"time"

	"product-version/internal/cmdb"
	"product-version/internal/versions/model"
	"product-version/internal/versions/source"
)

const (
	defaultSourceTimeout = 30 * time.Second
	defaultCacheTTL      = 30 * time.Second

	defaultProductConcurrency = 8
	defaultCMDBConcurrency    = 4
	defaultRuntimeConcurrency = 8
	defaultEOLConcurrency     = 4
)

// Service builds product version views from CMDB, runtime, and EOL sources.
//
// The limiters are service-scoped, so concurrent API requests share the same
// outbound concurrency budget rather than multiplying it.
type Service struct {
	config      *Config
	environment string

	cmdbSource    *source.CMDBSource
	runtimeSource *source.RuntimeSource
	eolSource     *source.EOLSource

	cacheTTL       time.Duration
	cacheMu        sync.Mutex
	cachedResponse *model.VersionResponse
	cacheExpiresAt time.Time
	refreshDone    chan struct{}

	productLimit chan struct{}
	cmdbLimit    chan struct{}
	runtimeLimit chan struct{}
	eolLimit     chan struct{}
}

// NewService creates the long-lived versions service.
func NewService(config *Config, cmdbClient *cmdb.Client, runtimeSource *source.RuntimeSource, environment string) *Service {
	return &Service{
		config:      config,
		environment: environment,

		cmdbSource:    source.NewCMDBSource(cmdbClient),
		runtimeSource: runtimeSource,
		eolSource:     source.NewEOLSource(defaultSourceTimeout),

		cacheTTL:     defaultCacheTTL,
		productLimit: make(chan struct{}, defaultProductConcurrency),
		cmdbLimit:    make(chan struct{}, defaultCMDBConcurrency),
		runtimeLimit: make(chan struct{}, defaultRuntimeConcurrency),
		eolLimit:     make(chan struct{}, defaultEOLConcurrency),
	}
}

// collectsRuntimeEnvironment reports whether this service instance may query
// runtime deployments in the target environment.
func (s *Service) collectsRuntimeEnvironment(environment string) bool {
	switch s.environment {
	case EnvironmentQA:
		return environment == EnvironmentQA
	case EnvironmentProd:
		return environment == EnvironmentQA || environment == EnvironmentProd
	default:
		return false
	}
}
