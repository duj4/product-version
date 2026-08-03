package versions

import (
	"context"
	"sync"
	"time"

	"product-version/internal/cmdb"
	"product-version/internal/versions/model"
	"product-version/internal/versions/source"
)

const (
	defaultSourceTimeout = 30 * time.Second
	defaultCacheTTL      = 30 * time.Second
	defaultEOLCacheTTL   = 6 * time.Hour

	defaultProductConcurrency = 8
	defaultCMDBConcurrency    = 4
	defaultRuntimeConcurrency = 8
	defaultEOLConcurrency     = 4
)

type eolCacheEntry struct {
	result    model.EOLResult
	expiresAt time.Time
}

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

	eolCacheTTL time.Duration
	eolCacheMu  sync.Mutex
	eolCache    map[string]eolCacheEntry

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

		cacheTTL:    defaultCacheTTL,
		eolCacheTTL: defaultEOLCacheTTL,
		eolCache:    make(map[string]eolCacheEntry),

		productLimit: make(chan struct{}, defaultProductConcurrency),
		cmdbLimit:    make(chan struct{}, defaultCMDBConcurrency),
		runtimeLimit: make(chan struct{}, defaultRuntimeConcurrency),
		eolLimit:     make(chan struct{}, defaultEOLConcurrency),
	}
}

// fetchEOL returns lifecycle data from the long-lived EOL cache when possible.
func (s *Service) fetchEOL(ctx context.Context, product source.EOLProduct) (model.EOLResult, bool, error) {
	cacheKey := product.Product + "\x00all"
	if product.PreferLTS {
		cacheKey = product.Product + "\x00lts"
	}

	now := time.Now()
	s.eolCacheMu.Lock()
	entry, exists := s.eolCache[cacheKey]
	if exists && now.Before(entry.expiresAt) {
		s.eolCacheMu.Unlock()
		return entry.result, true, nil
	}
	s.eolCacheMu.Unlock()

	result, err := s.eolSource.Fetch(ctx, product)
	if err != nil {
		return model.EOLResult{}, false, err
	}

	if s.eolCacheTTL > 0 {
		s.eolCacheMu.Lock()
		s.eolCache[cacheKey] = eolCacheEntry{
			result:    result,
			expiresAt: time.Now().Add(s.eolCacheTTL),
		}
		s.eolCacheMu.Unlock()
	}

	return result, false, nil
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
