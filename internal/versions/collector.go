package versions

import (
	"context"
	"fmt"
	"sync"

	"product-version/internal/logger"
	"product-version/internal/versions/model"
	"product-version/internal/versions/source"
)

// collectResponse fetches version information for all configured products.
func (s *Service) collectResponse(ctx context.Context) *model.VersionResponse {
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
				products[index] = newProductErrorResult(product, err)
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
				cmdbResult, err := s.cmdbSource.Fetch(ctx, source.CMDBProduct{
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
				eolResult, err := s.eolSource.Fetch(ctx, source.EOLProduct{
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

		runtimeResult.Assessment = source.AssessRuntime(
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

func newProductErrorResult(product ProductConfig, err error) model.ProductVersion {
	result := newProductResult(product)

	if product.CMDB.Enabled {
		result.Sources.CMDB = model.NewErrorCMDBResult(err)
	}

	if product.EOL.Enabled {
		result.Sources.EOL = model.NewErrorEOLResult(err)
	}

	for i, deployment := range product.Runtime.Deployments {
		if !deployment.Enabled {
			continue
		}
		result.Sources.Runtime.Deployments[i] = model.NewErrorRuntimeResult(deployment.Env, deployment.Type, err)
	}

	return result
}
