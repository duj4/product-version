package httpclient

import (
	"crypto/tls"
	"net/http"
	"time"
)

const defaultTimeout = 30 * time.Second

// NewClient creates a plain HTTP client with a pooled transport.
func NewClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout:   normalizeTimeout(timeout),
		Transport: newTransport(nil),
	}
}

func normalizeTimeout(timeout time.Duration) time.Duration {
	if timeout <= 0 {
		return defaultTimeout
	}
	return timeout
}

// newTransport creates a pooled transport shared by concurrent requests.
func newTransport(tlsConfig *tls.Config) *http.Transport {
	return &http.Transport{
		TLSClientConfig:     tlsConfig,
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
	}
}
