package versions

import (
	"fmt"
	"net/http"
	"os"
	"strings"

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
	Endpoint     string            `yaml:"endpoint"`
	Auth         AuthConfig        `yaml:"auth"`
	Headers      map[string]string `yaml:"headers"`
	Query        string            `yaml:"query"`
	VersionLabel string            `yaml:"version_label"`
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

// Validate validates the full products.yaml configuration.
//
// An empty product list is allowed. This makes it possible to enable the
// versions framework before any product is registered.
func (c *Config) Validate() error {
	if len(c.Products) == 0 {
		return nil
	}

	seenKeys := make(map[string]struct{})

	for i := range c.Products {
		product := &c.Products[i]
		if err := product.normalizeAndValidate(); err != nil {
			return fmt.Errorf("product[%d]: %w", i, err)
		}

		if _, exists := seenKeys[product.Key]; exists {
			return fmt.Errorf("duplicate product key: %q", product.Key)
		}

		seenKeys[product.Key] = struct{}{}
	}

	return nil
}

// normalizeAndValidate normalizes and validates one product config.
func (p *ProductConfig) normalizeAndValidate() error {
	p.Key = strings.TrimSpace(p.Key)

	if p.Key == "" {
		return fmt.Errorf("key must not be empty")
	}

	if err := p.Metadata.normalizeAndValidate(p.Key); err != nil {
		return err
	}

	if err := p.CMDB.normalizeAndValidate(p.Key, p.Metadata.ApplicationType); err != nil {
		return err
	}

	if err := p.Runtime.normalizeAndValidate(p.Key); err != nil {
		return err
	}

	if err := p.EOL.normalizeAndValidate(p.Key); err != nil {
		return err
	}

	return nil
}

// normalizeAndValidate normalizes and validates product metadata.
func (m *ProductMetadataConfig) normalizeAndValidate(productKey string) error {
	m.DisplayName = strings.TrimSpace(m.DisplayName)
	if m.DisplayName == "" {
		return fmt.Errorf("product %q metadata.display_name must not be empty", productKey)
	}

	m.ApplicationType = strings.TrimSpace(m.ApplicationType)
	if m.ApplicationType == "" {
		return fmt.Errorf("product %q metadata.application_type must not be empty", productKey)
	}

	return nil
}

// normalizeAndValidate normalizes and validates the CMDB section.
// If cmdb.enabled=false, all other CMDB fields are ignored.
func (c *CMDBConfig) normalizeAndValidate(productKey, applicationType string) error {
	if !c.Enabled {
		return nil
	}

	c.Name = strings.TrimSpace(c.Name)
	if c.Name == "" {
		return fmt.Errorf("product %q cmdb.name must not be empty when cmdb.enabled=true", productKey)
	}

	if strings.TrimSpace(applicationType) == "" {
		return fmt.Errorf("product %q metadata.application_type must not be empty when cmdb.enabled=true", productKey)
	}

	return nil
}

// normalizeAndValidate validates that every product has exactly one QA and one
// Prod runtime deployment.
func (r *RuntimeSourceConfig) normalizeAndValidate(productKey string) error {
	if len(r.Deployments) == 0 {
		return fmt.Errorf("product %q runtime.deployments must contain QA and Prod", productKey)
	}

	seenEnvironments := make(map[string]struct{}, len(r.Deployments))
	for i := range r.Deployments {
		deployment := &r.Deployments[i]
		if err := deployment.normalizeAndValidate(productKey); err != nil {
			return fmt.Errorf("product %q runtime.deployments[%d]: %w", productKey, i, err)
		}

		if _, exists := seenEnvironments[deployment.Env]; exists {
			return fmt.Errorf("product %q has duplicate runtime deployment env %q", productKey, deployment.Env)
		}
		seenEnvironments[deployment.Env] = struct{}{}
	}

	for _, env := range []string{EnvironmentQA, EnvironmentProd} {
		if _, exists := seenEnvironments[env]; !exists {
			return fmt.Errorf("product %q runtime.deployments is missing env %q", productKey, env)
		}
	}

	if len(seenEnvironments) != 2 {
		return fmt.Errorf("product %q runtime.deployments must contain only QA and Prod", productKey)
	}

	return nil
}

// normalizeAndValidate normalizes and validates one runtime deployment.
//
// If enabled=false, all fields other than env are ignored.
func (r *RuntimeDeploymentConfig) normalizeAndValidate(productKey string) error {
	r.Env = strings.ToLower(strings.TrimSpace(r.Env))
	switch r.Env {
	case EnvironmentQA, EnvironmentProd:
		// Supported environments.
	default:
		return fmt.Errorf("env %q is not supported; expected qa or prod", r.Env)
	}

	if !r.Enabled {
		return nil
	}

	r.Type = strings.ToLower(strings.TrimSpace(r.Type))
	if r.Type == "" {
		r.Type = RuntimeTypeHTTP
	}

	switch r.Type {
	case RuntimeTypeHTTP:
		return r.normalizeAndValidateHTTP(productKey)
	case RuntimeTypeMimir:
		return r.Mimir.normalizeAndValidate(productKey)

	default:
		return fmt.Errorf("product %q runtime.type %q is not supported", productKey, r.Type)
	}
}

// normalizeAndValidateHTTP normalizes and validates the direct HTTP runtime source.
func (r *RuntimeDeploymentConfig) normalizeAndValidateHTTP(productKey string) error {
	r.Endpoint = strings.TrimSpace(r.Endpoint)
	if r.Endpoint == "" {
		return fmt.Errorf("product %q runtime.endpoint must not be empty when runtime.enabled=true", productKey)
	}

	r.Method = strings.ToUpper(strings.TrimSpace(r.Method))
	if r.Method == "" {
		r.Method = "GET"
	}
	if r.Method != "GET" {
		return fmt.Errorf("product %q runtime.method %q is not supported; only GET is supported", productKey, r.Method)
	}

	if err := normalizeAndValidate(&r.Auth, productKey, "runtime.auth", AuthTypeNone); err != nil {
		return err
	}

	if len(r.AcceptedStatuses) == 0 {
		r.AcceptedStatuses = []int{http.StatusOK}
	}

	for _, status := range r.AcceptedStatuses {
		if status < 100 || status > 599 {
			return fmt.Errorf("product %q runtime.accepted_statuses contains invalid HTTP status %d", productKey, status)
		}
	}

	r.Parser.VersionField = strings.TrimSpace(r.Parser.VersionField)
	if r.Parser.VersionField == "" {
		return fmt.Errorf("product %q runtime.parser.version_field must not be empty when runtime.enabled=true", productKey)
	}

	return nil
}

// normalizeAndValidate normalizes and validates the Mimir runtime source.
func (m *MimirRuntimeConfig) normalizeAndValidate(productKey string) error {
	m.Endpoint = strings.TrimSpace(m.Endpoint)
	if m.Endpoint == "" {
		return fmt.Errorf("product %q runtime.mimir.endpoint must not be empty when runtime.type=mimir", productKey)
	}

	if err := normalizeAndValidate(&m.Auth, productKey, "runtime.mimir.auth", AuthTypeMTLS); err != nil {
		return err
	}

	if len(m.Headers) > 0 {
		normalizeHeaders := make(map[string]string, len(m.Headers))
		for key, value := range m.Headers {
			key = strings.TrimSpace(key)
			value = strings.TrimSpace(value)
			if key == "" {
				return fmt.Errorf("product %q runtime.mimir.headers contains empty header name", productKey)
			}
			normalizeHeaders[key] = value
		}
		m.Headers = normalizeHeaders
	}

	m.Query = strings.TrimSpace(m.Query)
	if m.Query == "" {
		return fmt.Errorf("product %q runtime.mimir.query must not be empty when runtime.type=mimir", productKey)
	}

	m.VersionLabel = strings.TrimSpace(m.VersionLabel)
	if m.VersionLabel == "" {
		return fmt.Errorf("product %q runtime.mimir.version_label must not be empty when runtime.type=mimir", productKey)
	}

	return nil
}

// normalizeAuthConfig normalizes and validates auth config.
func normalizeAndValidate(auth *AuthConfig, productKey, fieldPath, defaultType string) error {
	auth.Type = strings.ToLower(strings.TrimSpace(auth.Type))
	if auth.Type == "" {
		auth.Type = defaultType
	}

	switch auth.Type {
	case AuthTypeNone:
		// No additional validation is required.
	case AuthTypeMTLS:
		// Certificate paths are selected centrally from the deployment env.
	default:
		return fmt.Errorf("product %q %s.type %q is not supported", productKey, fieldPath, auth.Type)
	}

	return nil
}

// normalizeAndValidate normalizes and validates the endoflife.date section.
//
// If eol.enabled=false, all other EOL fields are ignored.
func (e *EOLConfig) normalizeAndValidate(productKey string) error {
	if !e.Enabled {
		return nil
	}

	e.Product = strings.TrimSpace(e.Product)
	if e.Product == "" {
		return fmt.Errorf("product %q eol.product must not be empty when eol.enabled=true", productKey)
	}

	e.CycleStrategy = strings.TrimSpace(e.CycleStrategy)
	if e.CycleStrategy == "" {
		e.CycleStrategy = CycleStrategyMajorMinor
	}

	if e.CycleStrategy != CycleStrategyMajorMinor {
		return fmt.Errorf(
			"product %q eol.cycle_strategy %q is not supported; only %q is supported",
			productKey,
			e.CycleStrategy,
			CycleStrategyMajorMinor,
		)
	}

	return nil
}
