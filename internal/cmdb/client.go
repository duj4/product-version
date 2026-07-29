package cmdb

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"product-version/internal/httpclient"
)

// LoadConfig reads CMDB settings from a JSON file and applies defaults.
func LoadConfig(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}

	var cfg Config

	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, err
	}

	if cfg.VersionsAPIURL == "" {
		return Config{}, fmt.Errorf("versions_api_url is empty")
	}

	if cfg.PageSize == 0 {
		cfg.PageSize = 100
	}

	if cfg.HTTPTimeoutSeconds == 0 {
		cfg.HTTPTimeoutSeconds = 15
	}

	return cfg, nil
}

// HTTPTimeout returns the configured HTTP timeout as a time.Duration.
func (cfg *Config) HTTPTimeout() time.Duration {
	return time.Duration(cfg.HTTPTimeoutSeconds) * time.Second
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
