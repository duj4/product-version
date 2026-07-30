package versions

import (
	"context"
	"time"

	"product-version/internal/versions/model"
)

// List returns the cached product versions or starts one shared collection.
func (s *Service) List(ctx context.Context) *model.VersionResponse {
	return s.list(ctx, false)
}

// Refresh bypasses a valid cache while still joining an in-flight collection.
func (s *Service) Refresh(ctx context.Context) *model.VersionResponse {
	return s.list(ctx, true)
}

func (s *Service) list(ctx context.Context, forceRefresh bool) *model.VersionResponse {
	allowCached := !forceRefresh

	for {
		now := time.Now()
		s.cacheMu.Lock()
		if allowCached && s.cachedResponse != nil && now.Before(s.cacheExpiresAt) {
			response := s.cachedResponse
			s.cacheMu.Unlock()
			return response
		}

		if s.refreshDone != nil {
			done := s.refreshDone
			s.cacheMu.Unlock()

			select {
			case <-done:
				allowCached = true
				continue
			case <-ctx.Done():
				return s.newErrorResponse(ctx.Err())
			}
		}

		done := make(chan struct{})
		s.refreshDone = done
		s.cacheMu.Unlock()

		response := s.collectResponse(ctx)

		s.cacheMu.Lock()
		if ctx.Err() == nil && s.cacheTTL > 0 {
			s.cachedResponse = response
			s.cacheExpiresAt = time.Now().Add(s.cacheTTL)
		}
		close(done)
		s.refreshDone = nil
		s.cacheMu.Unlock()

		return response
	}
}

func (s *Service) newErrorResponse(err error) *model.VersionResponse {
	response := &model.VersionResponse{
		Products: []model.ProductVersion{},
	}

	if s.config == nil {
		return response
	}

	response.Products = make([]model.ProductVersion, len(s.config.Products))
	for i, product := range s.config.Products {
		response.Products[i] = newProductErrorResult(product, err)
	}

	return response
}
