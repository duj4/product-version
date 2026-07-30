package cmdb

import (
	"net/http"

	"product-version/internal/httpclient"
)

// Client calls the CMDB API over the configured HTTP transport.
type Client struct {
	httpClient *http.Client
	cfg        Config
}

// NewClient creates a CMDB client backed by an mTLS HTTP transport.
func NewClient(cfg Config) (*Client, error) {
	httpClient, err := httpclient.NewTLSClient(
		cfg.CACertPath,
		cfg.ClientCertPath,
		cfg.ClientKeyPath,
		cfg.HTTPTimeout(),
	)
	if err != nil {
		return nil, err
	}

	return &Client{
		httpClient: httpClient,
		cfg:        cfg,
	}, nil
}
