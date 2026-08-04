package source

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"product-version/internal/httpclient"
	"product-version/internal/versions/model"
)

const (
	runtimeAuthTypeNone     = "none"
	runtimeAuthTypeKerberos = "kerberos"
	runtimeAuthTypeMTLS     = "mtls"

	runtimeTypeHTTP  = "http"
	runtimeTypeMimir = "mimir"

	defaultRuntimeTimeout = 30 * time.Second
)

// RuntimeSource fetches versions from environment-specific runtime endpoints.
type RuntimeSource struct {
	plainClient *http.Client
	mtlsClients map[string]*http.Client
	kerberos    *kerberosClientPool
}

// NewRuntimeSource creates a runtime source with reusable plain, mTLS, and
// lazily initialized environment-specific Kerberos HTTP clients.
func NewRuntimeSource(timeout time.Duration, profiles map[string]RuntimeTLSProfile, kerberosProfiles map[string]RuntimeKerberosProfile) (*RuntimeSource, error) {
	if timeout <= 0 {
		timeout = defaultRuntimeTimeout
	}

	source := &RuntimeSource{
		plainClient: httpclient.NewClient(timeout),
		mtlsClients: make(map[string]*http.Client, len(profiles)),
		kerberos:    newKerberosClientPool(timeout, kerberosProfiles),
	}

	for env, profile := range profiles {
		client, err := httpclient.NewTLSClient(
			profile.CACertPath,
			profile.ClientCertPath,
			profile.ClientKeyPath,
			timeout,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to create runtime mTLS client for env %q: %w", env, err)
		}

		source.mtlsClients[env] = client
	}

	return source, nil
}

// Fetch queries the product runtime endpoint and extracts the version field configured in products.yaml.
func (s *RuntimeSource) Fetch(ctx context.Context, product RuntimeProduct) (model.RuntimeDeploymentResult, error) {
	switch product.Type {
	case "", runtimeTypeHTTP:
		return s.fetchHTTP(ctx, product)
	case runtimeTypeMimir:
		return s.fetchMimir(ctx, product)
	default:
		return model.RuntimeDeploymentResult{}, fmt.Errorf("product %q runtime type %q is not supported", product.Key, product.Type)
	}
}

// fetchHTTP queries a product HTTP endpoint and extracts the configured JSON version field.
func (s *RuntimeSource) fetchHTTP(ctx context.Context, product RuntimeProduct) (model.RuntimeDeploymentResult, error) {
	client, err := s.clientFor(product.Env, product.Auth)
	if err != nil {
		return model.RuntimeDeploymentResult{}, fmt.Errorf("product %q failed to create runtime HTTP client: %w", product.Key, err)
	}

	req, err := http.NewRequestWithContext(ctx, product.Method, product.Endpoint, nil)
	if err != nil {
		return model.RuntimeDeploymentResult{}, fmt.Errorf("product %q failed to create runtime request: %w", product.Key, err)
	}

	resp, err := client.Do(req)
	if err != nil {
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
		return model.RuntimeDeploymentResult{}, fmt.Errorf("product %q failed to query runtime endpoint: %w", product.Key, err)
	}
	defer resp.Body.Close()

	if !isAcceptedStatus(resp.StatusCode, product.AcceptedStatuses) {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return model.RuntimeDeploymentResult{}, fmt.Errorf("product %q runtime endpoint returned status %s:%s", product.Key, resp.Status, strings.TrimSpace(string(body)))
	}

	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return model.RuntimeDeploymentResult{}, fmt.Errorf("product %q failed to decode runtime response: %w", product.Key, err)
	}

	version, ok := getTopLevelString(body, product.VersionField)
	if !ok {
		return model.RuntimeDeploymentResult{}, fmt.Errorf(
			"product %q runtime version field %q not found or not a string",
			product.Key,
			product.VersionField,
		)
	}

	version = strings.TrimSpace(version)
	if version == "" {
		return model.RuntimeDeploymentResult{}, fmt.Errorf("product %q runtime version field %q is empty", product.Key, product.VersionField)
	}

	return model.NewOKRuntimeResult(product.Env, product.Type, version), nil
}

// fetchMimir queries Mimir/Prometheus query API and extracts versions from a metric label.
func (s *RuntimeSource) fetchMimir(ctx context.Context, product RuntimeProduct) (model.RuntimeDeploymentResult, error) {
	client, err := s.clientFor(product.Env, product.Mimir.Auth)
	if err != nil {
		return model.RuntimeDeploymentResult{}, fmt.Errorf("product %q failed to create Mimir HTTP client: %w", product.Key, err)
	}

	requestURL, err := buildMimirQueryURL(product.Mimir.Endpoint, product.Mimir.Query)
	if err != nil {
		return model.RuntimeDeploymentResult{}, fmt.Errorf("product %q failed to build Mimir query URL: %w", product.Key, err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return model.RuntimeDeploymentResult{}, fmt.Errorf("product %q failed to create Mimir request: %w", product.Key, err)
	}

	resp, err := client.Do(req)
	if err != nil {
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
		return model.RuntimeDeploymentResult{}, fmt.Errorf("product %q failed to query Mimir: %w", product.Key, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return model.RuntimeDeploymentResult{}, fmt.Errorf(
			"product %q Mimir query returned status %s: %s",
			product.Key,
			resp.Status,
			strings.TrimSpace(string(body)),
		)
	}

	var body mimirQueryResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return model.RuntimeDeploymentResult{}, fmt.Errorf("product %q failed to decode Mimir response: %w", product.Key, err)
	}

	if body.Status != "success" {
		return model.RuntimeDeploymentResult{}, fmt.Errorf("product %q Mimir query status is %q", product.Key, body.Status)
	}

	versions := extractMimirVersions(body.Data.Result, product.Mimir.VersionLabel)
	if len(versions) == 0 {
		return model.RuntimeDeploymentResult{}, fmt.Errorf(
			"product %q Mimir query returned no version label %q",
			product.Key,
			product.Mimir.VersionLabel,
		)
	}

	versions = uniqueAndSortVersions(versions)
	selected := versions[len(versions)-1]

	return model.NewOKRuntimeResultWithCandidates(product.Env, product.Type, selected, versions), nil
}

// clientFor returns a reusable runtime HTTP client for the auth type and env.
func (s *RuntimeSource) clientFor(env string, auth RuntimeAuth) (runtimeHTTPClient, error) {
	switch auth.Type {
	case runtimeAuthTypeNone:
		return s.plainClient, nil

	case runtimeAuthTypeMTLS:
		client, exists := s.mtlsClients[env]
		if !exists {
			return nil, fmt.Errorf("no runtime mTLS client configured for env %q", env)
		}
		return client, nil

	case runtimeAuthTypeKerberos:
		if s.kerberos == nil {
			return nil, fmt.Errorf("runtime Kerberos client pool is not configured")
		}
		return s.kerberos.clientFor(env)

	default:
		return nil, fmt.Errorf("unsupported runtime auth type %q", auth.Type)
	}
}

// isAcceptedStatus verifies if the response status code is in the list.
func isAcceptedStatus(status int, accepted []int) bool {
	if len(accepted) == 0 {
		return status == http.StatusOK
	}

	for _, acceptedStatus := range accepted {
		if status == acceptedStatus {
			return true
		}
	}

	return false
}

// getTopLevelString returns a string value from a top-level JSON object field.
func getTopLevelString(body map[string]any, field string) (string, bool) {
	field = strings.TrimSpace(field)
	if field == "" {
		return "", false
	}

	value, ok := body[field]
	if !ok {
		return "", false
	}

	switch v := value.(type) {
	case string:
		return v, true
	default:
		return "", false
	}
}

// buildMimirQueryURL builds the Mimir instant query API URL.
func buildMimirQueryURL(endpoint, query string) (string, error) {
	parsedURL, err := url.Parse(endpoint)
	if err != nil {
		return "", err
	}
	params := parsedURL.Query()
	params.Set("query", query)
	parsedURL.RawQuery = params.Encode()

	return parsedURL.String(), nil
}

// mimirQueryResponse represents the Prometheus-compatible instant query response.
type mimirQueryResponse struct {
	Status string         `json:"status"`
	Data   mimirQueryData `json:"data"`
}

// mimirQueryData represents the data section in a Mimir instant query response.
type mimirQueryData struct {
	ResultType string             `json:"resultType"`
	Result     []mimirQueryResult `json:"result"`
}

// mimirQueryResult represents one vector result item.
type mimirQueryResult struct {
	Metric map[string]string `json:"metric"`
	Value  []any             `json:"value"`
}

// extractMimirVersions extracts version label values from Mimir result items.
func extractMimirVersions(results []mimirQueryResult, versionLabel string) []string {
	versionLabel = strings.TrimSpace(versionLabel)
	if versionLabel == "" {
		return nil
	}

	versions := make([]string, 0, len(results))
	for _, result := range results {
		version := strings.TrimSpace(result.Metric[versionLabel])
		if version == "" {
			continue
		}
		versions = append(versions, version)
	}

	return versions
}
