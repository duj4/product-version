package versions

import (
	"strings"
	"testing"
)

func TestConfigValidateNormalizesDeployments(t *testing.T) {
	t.Parallel()

	cfg := Config{
		Products: []ProductConfig{
			{
				Key: " sample ",
				Metadata: ProductMetadataConfig{
					DisplayName:     " Sample Product ",
					ApplicationType: " generic ",
				},
				Runtime: RuntimeSourceConfig{
					Deployments: []RuntimeDeploymentConfig{
						{
							Env:      " QA ",
							Enabled:  true,
							Endpoint: " https://qa.example.test/version ",
							Auth: AuthConfig{
								Type: AuthTypeNone,
							},
							Parser: ParserConfig{
								VersionField: " version ",
							},
						},
						{
							Env:     "PROD",
							Enabled: false,
						},
					},
				},
			},
		},
	}

	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	product := cfg.Products[0]
	if product.Key != "sample" {
		t.Fatalf("Key = %q, want sample", product.Key)
	}
	if product.Metadata.DisplayName != "Sample Product" {
		t.Fatalf("DisplayName = %q", product.Metadata.DisplayName)
	}

	qa := product.Runtime.Deployments[0]
	if qa.Env != EnvironmentQA {
		t.Fatalf("QA Env = %q, want qa", qa.Env)
	}
	if qa.Type != RuntimeTypeHTTP {
		t.Fatalf("QA Type = %q, want http", qa.Type)
	}
	if qa.Method != "GET" {
		t.Fatalf("QA Method = %q, want GET", qa.Method)
	}
	if len(qa.AcceptedStatuses) != 1 || qa.AcceptedStatuses[0] != 200 {
		t.Fatalf("QA AcceptedStatuses = %v, want [200]", qa.AcceptedStatuses)
	}
}

func TestConfigValidateRequiresQAAndProd(t *testing.T) {
	t.Parallel()

	cfg := validConfig()
	cfg.Products[0].Runtime.Deployments = cfg.Products[0].Runtime.Deployments[:1]

	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), `missing env "prod"`) {
		t.Fatalf("Validate() error = %v, want missing prod error", err)
	}
}

func TestConfigValidateRejectsDuplicateDeploymentEnv(t *testing.T) {
	t.Parallel()

	cfg := validConfig()
	cfg.Products[0].Runtime.Deployments[1].Env = EnvironmentQA

	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "duplicate runtime deployment env") {
		t.Fatalf("Validate() error = %v, want duplicate env error", err)
	}
}

func TestConfigValidatePreservesMimirHeaders(t *testing.T) {
	t.Parallel()

	cfg := validConfig()
	cfg.Products[0].Runtime.Deployments[0] = RuntimeDeploymentConfig{
		Env:     EnvironmentQA,
		Enabled: true,
		Type:    RuntimeTypeMimir,
		Mimir: MimirRuntimeConfig{
			Endpoint: "https://mimir.example.test/prometheus/api/v1/query",
			Auth: AuthConfig{
				Type: AuthTypeNone,
			},
			Headers: map[string]string{
				" X-Scope-OrgID ": " qa ",
			},
			Query:        "build_info",
			VersionLabel: "version",
		},
	}

	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	headers := cfg.Products[0].Runtime.Deployments[0].Mimir.Headers
	if got := headers["X-Scope-OrgID"]; got != "qa" {
		t.Fatalf("normalized header = %q, want qa; headers=%v", got, headers)
	}
}

func TestProductsConfigLoads(t *testing.T) {
	t.Parallel()

	cfg, err := LoadConfig("../../config/products.yaml")
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if len(cfg.Products) == 0 {
		t.Fatal("products config is empty")
	}
}

func validConfig() Config {
	return Config{
		Products: []ProductConfig{
			{
				Key: "sample",
				Metadata: ProductMetadataConfig{
					DisplayName:     "Sample Product",
					ApplicationType: "generic",
				},
				CMDB: CMDBConfig{
					Enabled: false,
				},
				Runtime: RuntimeSourceConfig{
					Deployments: []RuntimeDeploymentConfig{
						{
							Env:     EnvironmentQA,
							Enabled: false,
						},
						{
							Env:     EnvironmentProd,
							Enabled: false,
						},
					},
				},
				EOL: EOLConfig{
					Enabled: false,
				},
			},
		},
	}
}
