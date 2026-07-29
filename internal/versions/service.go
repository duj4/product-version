package versions

import (
	"context"
	"fmt"
	"sync"
	"time"

	"product-versions/internal/cmdb"
	"product-versions/internal/logger"
	"product-versions/internal/versions/model"
	"product-versions/internal/versions/provider"
)

const (
	defaultSourceTimeout = 30 * time.Second

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
	config *Config

	cmdbSource    *provider.CMDBSource
	runtimeSource *provider.RuntimeSource
	eolSource     *provider.EOLSource

	productLimit chan struct{}
	cmdbLimit    chan struct{}
	runtimeLimit chan struct{}
	eolLimit     chan struct{}
}

// NewService creates the long-lived versions service.
func NewService(config *Config, cmdbClient *cmdb.Client, runtimeSource *provider.RuntimeSource) *Service {
	return &Service{
		config: config,

		cmdbSource:    provider.NewCMDBSource(cmdbClient),
		runtimeSource: runtimeSource,
		eolSource:     provider.NewEOLSource(defaultSourceTimeout),

		productLimit: make(chan struct{}, defaultProductConcurrency),
		cmdbLimit:    make(chan struct{}, defaultCMDBConcurrency),
		runtimeLimit: make(chan struct{}, defaultRuntimeConcurrency),
		eolLimit:     make(chan struct{}, defaultEOLConcurrency),
	}
}

// List returns version information for all configured products.
func (s *Service) List(ctx context.Context) *model.VersionResponse {
	resp := &model.VersionResponse{
		Products: []model.ProductVersion{},
	}

	if s.config == nil || len(s.config.Products) == 0 {
		return resp
	}

	products := make([]model.ProductVersion, len(s.config.Products))

	var wg sync.WaitGroup
	for i, product := range s.config.Products {
		wg.Add(1)

		go func(index int, product ProductConfig) {
			defer wg.Done()

			if err := acquire(ctx, s.productLimit); err != nil {
				products[index] = newProductResult(product)
				return
			}
			defer release(s.productLimit)

			products[index] = s.collectProduct(ctx, product)
		}(i, product)
	}

	wg.Wait()
	resp.Products = products

	return resp
}

// collectProduct fetches independent product sources and runtime deployments
// concurrently, then performs lightweight in-memory assessments.
func (s *Service) collectProduct(ctx context.Context, product ProductConfig) model.ProductVersion {
	result := newProductResult(product)

	var wg sync.WaitGroup

	if product.CMDB.Enabled {
		wg.Add(1)
		go func() {
			defer wg.Done()

			err := withLimit(ctx, s.cmdbLimit, func() error {
				cmdbResult, err := s.cmdbSource.Fetch(ctx, provider.CMDBProduct{
					Key:             product.Key,
					Name:            product.CMDB.Name,
					ApplicationType: product.Metadata.ApplicationType,
				})
				if err != nil {
					return err
				}

				result.Sources.CMDB = cmdbResult
				return nil
			})
			if err != nil {
				logger.Error(
					"version source failed",
					"product", product.Key,
					"source", "cmdb",
					"error", err,
				)
				result.Sources.CMDB = model.NewErrorCMDBResult(err)
			}
		}()
	}

	if product.EOL.Enabled {
		wg.Add(1)
		go func() {
			defer wg.Done()

			err := withLimit(ctx, s.eolLimit, func() error {
				eolResult, err := s.eolSource.Fetch(ctx, provider.EOLProduct{
					Key:       product.Key,
					Product:   product.EOL.Product,
					PreferLTS: product.EOL.PreferLTS,
				})
				if err != nil {
					return err
				}

				result.Sources.EOL = eolResult
				return nil
			})
			if err != nil {
				logger.Error(
					"version source failed",
					"product", product.Key,
					"source", "eol",
					"eol_product", product.EOL.Product,
					"error", err,
				)
				result.Sources.EOL = model.NewErrorEOLResult(err)
			}
		}()
	}

	for i, deployment := range product.Runtime.Deployments {
		if !deployment.Enabled {
			continue
		}

		wg.Add(1)
		go func(index int, deployment RuntimeDeploymentConfig) {
			defer wg.Done()

			err := withLimit(ctx, s.runtimeLimit, func() error {
				if s.runtimeSource == nil {
					return fmt.Errorf("runtime source is not configured")
				}

				runtimeResult, err := s.runtimeSource.Fetch(ctx, toRuntimeProduct(product.Key, deployment))
				if err != nil {
					return err
				}

				result.Sources.Runtime.Deployments[index] = runtimeResult
				return nil
			})
			if err != nil {
				endpoint := deployment.Endpoint
				if deployment.Type == RuntimeTypeMimir {
					endpoint = deployment.Mimir.Endpoint
				}

				logger.Error(
					"version source failed",
					"product", product.Key,
					"env", deployment.Env,
					"source", "runtime",
					"type", deployment.Type,
					"endpoint", endpoint,
					"error", err,
				)
				result.Sources.Runtime.Deployments[index] = model.NewErrorRuntimeResult(
					deployment.Env,
					deployment.Type,
					err,
				)
			}
		}(i, deployment)
	}

	wg.Wait()

	for i := range result.Sources.Runtime.Deployments {
		runtimeResult := &result.Sources.Runtime.Deployments[i]
		if runtimeResult.Status != model.SourceStatusOK {
			continue
		}

		runtimeResult.Assessment = provider.AssessRuntime(
			result.Sources.EOL,
			runtimeResult.Version,
			product.EOL.CycleStrategy,
			result.Sources.CMDB.Version,
		)
	}

	return result
}

func newProductResult(product ProductConfig) model.ProductVersion {
	deployments := make([]model.RuntimeDeploymentResult, len(product.Runtime.Deployments))
	for i, deployment := range product.Runtime.Deployments {
		deployments[i] = model.NewDisabledRuntimeResult(deployment.Env, deployment.Type)
	}

	return model.ProductVersion{
		Key: product.Key,
		Metadata: model.ProductMetadata{
			DisplayName:     product.Metadata.DisplayName,
			ApplicationType: product.Metadata.ApplicationType,
		},
		Sources: model.VersionSources{
			CMDB: model.NewDisabledCMDBResult(),
			Runtime: model.RuntimeSourceResult{
				Deployments: deployments,
			},
			EOL: model.NewDisabledEOLResult(),
		},
	}
}

func toRuntimeProduct(productKey string, deployment RuntimeDeploymentConfig) provider.RuntimeProduct {
	return provider.RuntimeProduct{
		Key:              productKey,
		Env:              deployment.Env,
		Type:             deployment.Type,
		Endpoint:         deployment.Endpoint,
		Method:           deployment.Method,
		Auth:             toRuntimeAuth(deployment.Auth),
		AcceptedStatuses: deployment.AcceptedStatuses,
		VersionField:     deployment.Parser.VersionField,
		Mimir: provider.RuntimeMimir{
			Endpoint:     deployment.Mimir.Endpoint,
			Auth:         toRuntimeAuth(deployment.Mimir.Auth),
			Headers:      deployment.Mimir.Headers,
			Query:        deployment.Mimir.Query,
			VersionLabel: deployment.Mimir.VersionLabel,
		},
	}
}

func toRuntimeAuth(auth AuthConfig) provider.RuntimeAuth {
	return provider.RuntimeAuth{
		Type: auth.Type,
	}
}

func withLimit(ctx context.Context, limit chan struct{}, fn func() error) error {
	if err := acquire(ctx, limit); err != nil {
		return err
	}
	defer release(limit)

	return fn()
}

func acquire(ctx context.Context, limit chan struct{}) error {
	select {
	case limit <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func release(limit chan struct{}) {
	<-limit
}
