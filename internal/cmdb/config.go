package cmdb

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

// Config holds settings for the CMDB client.
type Config struct {
	VersionsAPIURLs    map[string]string `json:"versions_api_urls"`
	VersionsAPIURL     string            `json:"-"`
	PageSize           int               `json:"page_size"`
	CACertPath         string            `json:"ca_cert_path"`
	ClientCertPath     string            `json:"client_cert_path"`
	ClientKeyPath      string            `json:"client_key_path"`
	HTTPTimeoutSeconds int               `json:"http_timeout_seconds"`
}

// LoadConfig reads CMDB settings, selects the URL for environment, and applies defaults.
func LoadConfig(path, environment string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}

	var cfg Config

	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, err
	}

	environment = strings.ToLower(strings.TrimSpace(environment))
	switch environment {
	case "qa", "prod":
		// Supported environments.
	default:
		return Config{}, fmt.Errorf("unsupported CMDB environment %q, expected qa or prod", environment)
	}

	cfg.VersionsAPIURL = strings.TrimSpace(cfg.VersionsAPIURLs[environment])
	if cfg.VersionsAPIURL == "" {
		return Config{}, fmt.Errorf("versions_api_urls.%s is empty", environment)
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
