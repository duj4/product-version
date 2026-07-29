package cmdb

import (
	"net/http"
)

// Config holds settings for the CMDB client.
type Config struct {
	VersionsAPIURL     string `json:"versions_api_url"`
	PageSize           int    `json:"page_size"`
	CACertPath         string `json:"ca_cert_path"`
	ClientCertPath     string `json:"client_cert_path"`
	ClientKeyPath      string `json:"client_key_path"`
	HTTPTimeoutSeconds int    `json:"http_timeout_seconds"`
}

// Client calls the CMDB API over the configured HTTP transport.
type Client struct {
	httpClient *http.Client
	cfg        Config
}
