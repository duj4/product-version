package versions

import (
	"context"
	"time"

	"product-version/internal/logger"
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

		if allowCached && s.cachedResponse != nil {
			response := s.cachedResponse
			if now.Before(s.cacheExpiresAt) {
				s.cacheMu.Unlock()
				return response
			}

			if s.refreshDone == nil {
				done := make(chan struct{})
				s.refreshDone = done
				s.cacheMu.Unlock()
				go s.refreshInBackground(done, "stale")
				return response
			}

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
		return s.collectAndStore(ctx, done)
	}
}

// WarmCache starts one asynchronous collection when the service cache is empty.
func (s *Service) WarmCache() {
	s.cacheMu.Lock()
	if s.cachedResponse != nil || s.refreshDone != nil {
		s.cacheMu.Unlock()
		return
	}

	done := make(chan struct{})
	s.refreshDone = done
	s.cacheMu.Unlock()

	go s.refreshInBackground(done, "warmup")
}

func (s *Service) refreshInBackground(done chan struct{}, trigger string) {
	startedAt := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), defaultSourceTimeout+5*time.Second)
	defer cancel()

	s.collectAndStore(ctx, done)

	args := []any{
		"trigger", trigger,
		"duration_ms", time.Since(startedAt).Milliseconds(),
	}
	if err := ctx.Err(); err != nil {
		logger.Warn("version cache background refresh failed", append(args, "error", err)...)
		return
	}

	logger.Info("version cache background refresh completed", args...)
}

func (s *Service) collectAndStore(ctx context.Context, done chan struct{}) *model.VersionResponse {
	response := s.collectResponse(ctx)

	s.cacheMu.Lock()
	if ctx.Err() == nil && s.cacheTTL > 0 {
		s.cachedResponse = response
		s.cacheExpiresAt = time.Now().Add(s.cacheTTL)
	}
	close(done)
	if s.refreshDone == done {
		s.refreshDone = nil
	}
	s.cacheMu.Unlock()

	return response
}

func (s *Service) newErrorResponse(err error) *model.VersionResponse {
	response := &model.VersionResponse{
		CollectedAt: time.Now().UTC().Format(time.RFC3339),
		Products:    []model.ProductVersion{},
	}

	if s.config == nil {
		return response
	}

	response.Products = make([]model.ProductVersion, len(s.config.Products))
	for i, product := range s.config.Products {
		response.Products[i] = s.newProductErrorResult(product, err)
	}

	return response
}
