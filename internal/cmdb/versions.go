package cmdb

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// QueryVersionObjects queries CMDB version objects using the supplied IQL.
func (c *Client) QueryVersionObjects(ctx context.Context, query VersionObjectQuery) (*VersionObjectResponse, error) {
	query.Normalize()
	if strings.TrimSpace(query.IQL) == "" {
		return nil, fmt.Errorf("cmdb version query iql must not be empty")
	}

	requestURL, err := c.buildVersionObjectsRequestURL(query)
	if err != nil {
		return nil, fmt.Errorf("failed to build cmdb versions request URL: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create cmdb versions request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to query cmdb version API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("cmdb versions API returned status %s", resp.Status)
	}

	var result VersionObjectResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode cmdb versions response: %w", err)
	}

	return &result, nil
}

// buildVersionObjectsRequestURL constructs the CMDB versions request URL.
func (c *Client) buildVersionObjectsRequestURL(query VersionObjectQuery) (string, error) {
	baseURL, err := url.Parse(c.cfg.VersionsAPIURL)
	if err != nil {
		return "", fmt.Errorf("failed to parse cmdb versions_api_url %q: %w", c.cfg.VersionsAPIURL, err)
	}

	params := url.Values{}
	params.Set("schemaName", query.SchemaName)
	params.Set("objectType", query.ObjectType)
	params.Set("iql", query.IQL)
	params.Set("childType", strconv.FormatBool(query.ChildType))
	params.Set("page", strconv.Itoa(query.Page))
	params.Set("pageSize", strconv.Itoa(query.PageSize))

	for _, attr := range query.AttrsInclude {
		attr = strings.TrimSpace(attr)
		if attr == "" {
			continue
		}
		params.Add("attrsInclude", attr)
	}

	baseURL.RawQuery = params.Encode()

	return baseURL.String(), nil
}
