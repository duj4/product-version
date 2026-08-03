package versions

import (
	"context"
	"fmt"
	"time"

	"product-version/internal/logger"
	"product-version/internal/versions/model"
)

type refreshState struct {
	done     chan struct{}
	response *model.VersionResponse
	err      error
}

// List returns a fresh cached response or waits for one shared collection.
func (s *Service) List(ctx context.Context) *model.VersionResponse {
	return s.list(ctx, false)
}

// Refresh bypasses a valid cache while still joining an in-flight collection.
func (s *Service) Refresh(ctx context.Context) *model.VersionResponse {
	return s.list(ctx, true)
}

func (s *Service) list(ctx context.Context, forceRefresh bool) *model.VersionResponse {
	now := time.Now()
	s.cacheMu.Lock()

	if !forceRefresh && s.cachedResponse != nil && now.Before(s.cacheExpiresAt) {
		response := s.cachedResponse
		s.cacheMu.Unlock()
		return response
	}

	refresh := s.refresh
	if refresh == nil {
		trigger := "expired"
		if forceRefresh {
			trigger = "manual"
		}

		refresh = s.newRefreshLocked()
		s.cacheMu.Unlock()
		go s.runRefresh(refresh, trigger)
	} else {
		s.cacheMu.Unlock()
	}

	select {
	case <-refresh.done:
		if refresh.err == nil && refresh.response != nil {
			return refresh.response
		}

		s.cacheMu.Lock()
		fallback := s.cachedResponse
		s.cacheMu.Unlock()
		if fallback != nil {
			return fallback
		}

		err := refresh.err
		if err == nil {
			err = fmt.Errorf("version collection completed without a response")
		}
		return s.newErrorResponse(err)
	case <-ctx.Done():
		return s.newErrorResponse(ctx.Err())
	}
}

// WarmCache starts one asynchronous collection when the service cache is empty.
func (s *Service) WarmCache() {
	s.cacheMu.Lock()
	if s.cachedResponse != nil || s.refresh != nil {
		s.cacheMu.Unlock()
		return
	}

	refresh := s.newRefreshLocked()
	s.cacheMu.Unlock()

	go s.runRefresh(refresh, "warmup")
}

// newRefreshLocked creates the shared refresh state. The caller must hold cacheMu.
func (s *Service) newRefreshLocked() *refreshState {
	refresh := &refreshState{done: make(chan struct{})}
	s.refresh = refresh
	return refresh
}

func (s *Service) runRefresh(refresh *refreshState, trigger string) {
	startedAt := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), defaultSourceTimeout+5*time.Second)
	defer cancel()

	response := s.collectResponse(ctx)
	err := ctx.Err()

	s.cacheMu.Lock()
	refresh.response = response
	refresh.err = err
	hasFallback := s.cachedResponse != nil
	if err == nil && response != nil && s.cacheTTL > 0 {
		s.cachedResponse = response
		s.cacheExpiresAt = time.Now().Add(s.cacheTTL)
	}
	if s.refresh == refresh {
		s.refresh = nil
	}
	close(refresh.done)
	s.cacheMu.Unlock()

	args := []any{
		"trigger", trigger,
		"duration_ms", time.Since(startedAt).Milliseconds(),
	}
	if err != nil {
		logger.Warn(
			"version cache refresh failed",
			append(args, "fallback_available", hasFallback, "error", err)...,
		)
		return
	}

	logger.Info("version cache refresh completed", args...)
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
		response.Products[i] = s.newProductErrorResult(product, err)
	}

	return response
}
