package versions

import (
	"fmt"
	"os"

	"github.com/goccy/go-yaml"
)

const (
	// EnvironmentQA identifies the QA deployment of a product.
	EnvironmentQA = "qa"

	// EnvironmentProd identifies the production deployment of a product.
	EnvironmentProd = "prod"

	// AuthTypeNone means the runtime version endpoint does not require authentication.
	AuthTypeNone = "none"

	// AuthTypeMTLS means the runtime version endpoint requires mutual TLS.
	AuthTypeMTLS = "mtls"

	// AuthTypeKerberos means the runtime endpoint requires Kerberos SPNEGO.
	AuthTypeKerberos = "kerberos"

	// RuntimeTypeHTTP means the runtime version is fetched from a product HTTP endpoint.
	RuntimeTypeHTTP = "http"

	// RuntimeTypeMimir means the runtime version is fetched from Mimir/Prometheus query API.
	RuntimeTypeMimir = "mimir"

	// CycleStrategyMajorMinor means a runtime version such as 10.3.18 or 3.6.10
	// should be mapped to the EOL release cycle 10.3 or 3.6.
	CycleStrategyMajorMinor = "major_minor"
)

// Config represents the root structure of products.yaml.
type Config struct {
	Products []ProductConfig `yaml:"products"`
}

// ProductConfig describes one product entry in products.yaml.
//
// Key is the stable product ID and is unique within products.yaml. Environment
// is deliberately not encoded in the key; the QA and Prod runtime deployments
// are defined below the runtime source.
type ProductConfig struct {
	Key      string                `yaml:"key"`
	Metadata ProductMetadataConfig `yaml:"metadata"`
	CMDB     CMDBConfig            `yaml:"cmdb"`
	Runtime  RuntimeSourceConfig   `yaml:"runtime"`
	EOL      EOLConfig             `yaml:"eol"`
}

// ProductMetadataConfig contains environment-independent product metadata.
type ProductMetadataConfig struct {
	DisplayName     string `yaml:"display_name"`
	ApplicationType string `yaml:"application_type"`
}

// CMDBConfig defines how to query the CMDB registered version for a product.
type CMDBConfig struct {
	Enabled bool   `yaml:"enabled"`
	Name    string `yaml:"name"`
}

// RuntimeSourceConfig defines the environment-specific runtime deployments.
type RuntimeSourceConfig struct {
	Deployments []RuntimeDeploymentConfig `yaml:"deployments"`
}

// RuntimeDeploymentConfig defines how to query one environment's runtime version.
type RuntimeDeploymentConfig struct {
	Env     string `yaml:"env"`
	Enabled bool   `yaml:"enabled"`

	// Type controls how runtime version is fetched.
	Type string `yaml:"type"`

	// HTTP runtime fields.
	Endpoint         string       `yaml:"endpoint"`
	Method           string       `yaml:"method"`
	Auth             AuthConfig   `yaml:"auth"`
	AcceptedStatuses []int        `yaml:"accepted_statuses"`
	Parser           ParserConfig `yaml:"parser"`

	// Mimir runtime fields
	Mimir MimirRuntimeConfig `yaml:"mimir"`
}

// MimirRuntimeConfig defines how to query Mimir/Prometheus for runtime version.
type MimirRuntimeConfig struct {
	Endpoint     string     `yaml:"endpoint"`
	Auth         AuthConfig `yaml:"auth"`
	Query        string     `yaml:"query"`
	VersionLabel string     `yaml:"version_label"`
}

// AuthConfig defines the authentication method for the runtime version endpoint.
type AuthConfig struct {
	Type string `yaml:"type"`
}

// ParserConfig defines how to extract version fields from a runtime API JSON response.
type ParserConfig struct {
	VersionField string `yaml:"version_field"`
}

// EOLConfig defines how to query endoflife.date for product lifecycle data.
type EOLConfig struct {
	Enabled       bool   `yaml:"enabled"`
	Product       string `yaml:"product"`
	CycleStrategy string `yaml:"cycle_strategy"`
	PreferLTS     bool   `yaml:"prefer_lts"`
}

// LoadConfig reads and validates the versions product configuration.
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read versions config %s: %w", path, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse versions config %s: %w", path, err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid versions config %s: %w", path, err)
	}

	return &cfg, nil
}
