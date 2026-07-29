package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"product-versions/internal/versions/model"
	"strings"
	"time"
)

const (
	eolAPIBaseURL              = "https://endoflife.date/api/v1/products"
	eolCycleStrategyMajorMinor = "major_minor"
	defaultEOLTimeout          = 30 * time.Second
)

// EOLSource fetches lifecycle and latest-version metadata from endoflife.date.
type EOLSource struct {
	client *http.Client
}

// NewEOLSource creates an EOL source.
func NewEOLSource(timeout time.Duration) *EOLSource {
	if timeout <= 0 {
		timeout = defaultEOLTimeout
	}

	return &EOLSource{
		client: &http.Client{
			Timeout: timeout,
			Transport: &http.Transport{
				Proxy: http.ProxyFromEnvironment,
			},
		},
	}
}

// Fetch queries endoflife.date once for one product and returns its release catalog.
func (s *EOLSource) Fetch(ctx context.Context, product EOLProduct) (model.EOLResult, error) {
	requestURL, err := buildEOLRequestURL(product.Product)
	if err != nil {
		return model.EOLResult{}, fmt.Errorf("product %q failed to build EOL request URL: %w", product.Key, err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return model.EOLResult{}, fmt.Errorf("product %q failed to create EOL request: %w", product.Key, err)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return model.EOLResult{}, fmt.Errorf("product %q failed to query EOL endpoint: %w", product.Key, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return model.EOLResult{}, fmt.Errorf("product %q EOL endpoint returned status %s", product.Key, resp.Status)
	}

	var body eolProductResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return model.EOLResult{}, fmt.Errorf("product %q failed to decode EOL response: %w", product.Key, err)
	}

	releases := body.Result.Releases
	if len(releases) == 0 {
		return model.EOLResult{}, fmt.Errorf("product %q EOL response contains no releases", product.Key)
	}

	latestOverallCycle, latestOverall, latestOverallDate, latestOverallIsLTS := findLatestOverall(releases, product.PreferLTS)
	result := model.EOLResult{
		Status:             model.SourceStatusOK,
		Product:            body.Result.Name,
		LatestOverall:      latestOverall,
		LatestOverallCycle: latestOverallCycle,
		LatestOverallIsLTS: latestOverallIsLTS,
		LatestOverallDate:  latestOverallDate,
		Cycles:             make([]model.EOLCycle, 0, len(releases)),
	}

	for _, release := range releases {
		result.Cycles = append(result.Cycles, model.EOLCycle{
			Cycle:        strings.TrimSpace(release.Name),
			Label:        strings.TrimSpace(release.Label),
			ReleaseDate:  strings.TrimSpace(release.ReleaseDate),
			IsLTS:        release.IsLTS,
			IsEOL:        release.IsEOL,
			EOLFrom:      strings.TrimSpace(release.EOLFrom),
			IsMaintained: release.IsMaintained,
			Latest:       strings.TrimSpace(release.Latest.Name),
			LatestDate:   strings.TrimSpace(release.Latest.Date),
		})
	}

	return result, nil
}

// AssessRuntime derives lifecycle information for one environment's runtime
// version from the product-level endoflife.date result. It performs no I/O.
func AssessRuntime(eol model.EOLResult, runtimeVersion, strategy, cmdbVersion string) model.VersionAssessment {
	if eol.Status == model.SourceStatusDisabled {
		return model.VersionAssessment{
			Status:       model.SourceStatusDisabled,
			CMDBMismatch: versionsDiffer(cmdbVersion, runtimeVersion),
		}
	}

	if eol.Status != model.SourceStatusOK {
		return model.VersionAssessment{
			Status:       model.SourceStatusError,
			CMDBMismatch: versionsDiffer(cmdbVersion, runtimeVersion),
			Error:        "endoflife.date data is unavailable",
		}
	}

	runtimeVersion = strings.TrimSpace(runtimeVersion)
	if runtimeVersion == "" {
		return model.VersionAssessment{
			Status:       model.SourceStatusPartial,
			CMDBMismatch: versionsDiffer(cmdbVersion, runtimeVersion),
			Error:        "runtime version is empty; cannot resolve current EOL cycle",
		}
	}

	currentCycle, err := resolveCurrentCycle(runtimeVersion, strategy)
	if err != nil {
		return model.VersionAssessment{
			Status:       model.SourceStatusPartial,
			CMDBMismatch: versionsDiffer(cmdbVersion, runtimeVersion),
			Error:        fmt.Sprintf("failed to resolve current EOL cycle from runtime version %q: %v", runtimeVersion, err),
		}
	}

	currentRelease, ok := findEOLCycle(eol.Cycles, currentCycle)
	if !ok {
		return model.VersionAssessment{
			Status:       model.SourceStatusPartial,
			CurrentCycle: currentCycle,
			CMDBMismatch: versionsDiffer(cmdbVersion, runtimeVersion),
			Error:        fmt.Sprintf("current EOL cycle %q not found", currentCycle),
		}
	}

	return model.VersionAssessment{
		Status:                   model.SourceStatusOK,
		CurrentCycle:             currentCycle,
		CurrentCycleLabel:        currentRelease.Label,
		CurrentCycleReleaseDate:  currentRelease.ReleaseDate,
		IsLTS:                    currentRelease.IsLTS,
		IsEOL:                    currentRelease.IsEOL,
		EOLFrom:                  currentRelease.EOLFrom,
		IsMaintained:             currentRelease.IsMaintained,
		LatestInCurrentCycle:     currentRelease.Latest,
		LatestInCurrentCycleDate: currentRelease.LatestDate,
		CMDBMismatch:             versionsDiffer(cmdbVersion, runtimeVersion),
		PatchAvailable:           versionIsBehind(runtimeVersion, currentRelease.Latest),
	}
}

// eolProductResponse represents the top-level response returned by endoflife.date API v1.
type eolProductResponse struct {
	Result eolProduct `json:"result"`
}

// eolProduct represents one product returned by endoflife.date.
type eolProduct struct {
	Name     string       `json:"name"`
	Releases []eolRelease `json:"releases"`
}

// eolRelease represents one lifecycle release cycle from endoflife.date.
type eolRelease struct {
	Name         string    `json:"name"`
	Label        string    `json:"label"`
	ReleaseDate  string    `json:"releaseDate"`
	IsLTS        bool      `json:"isLts"`
	IsEOL        bool      `json:"isEol"`
	EOLFrom      string    `json:"eolFrom"`
	IsMaintained bool      `json:"isMaintained"`
	Latest       eolLatest `json:"latest"`
}

// eolLatest represents the latest patch version inside one release cycle.
type eolLatest struct {
	Name string `json:"name"`
	Date string `json:"date"`
}

// buildEOLRequestURL builds the endoflife.date API URL for a product.
func buildEOLRequestURL(product string) (string, error) {
	product = strings.TrimSpace(product)
	if product == "" {
		return "", fmt.Errorf("EOL product must not be empty")
	}

	baseURL, err := url.Parse(eolAPIBaseURL)
	if err != nil {
		return "", err
	}

	baseURL.Path = strings.TrimRight(baseURL.Path, "/") + "/" + url.PathEscape(product)
	return baseURL.String(), nil
}

// findLatestOverall returns the latest selected version across release cycles.
func findLatestOverall(releases []eolRelease, preferLTS bool) (cycle, latest, date string, isLTS bool) {
	for _, release := range releases {
		if preferLTS && !release.IsLTS {
			continue
		}

		releaseLatest := strings.TrimSpace(release.Latest.Name)
		if releaseLatest == "" {
			continue
		}

		if latest == "" || compareVersion(releaseLatest, latest) > 0 {
			latest = releaseLatest
			cycle = strings.TrimSpace(release.Name)
			date = strings.TrimSpace(release.Latest.Date)
			isLTS = release.IsLTS
		}
	}

	return cycle, latest, date, isLTS
}

// resolveCurrentCycle maps a runtime version to an endoflife.date release cycle.
func resolveCurrentCycle(version string, strategy string) (string, error) {
	strategy = strings.TrimSpace(strategy)
	if strategy == "" {
		strategy = eolCycleStrategyMajorMinor
	}

	switch strategy {
	case eolCycleStrategyMajorMinor:
		return majorMinorCycle(version)

	default:
		return "", fmt.Errorf("unsupported cycle strategy %q", strategy)
	}
}

// majorMinorCycle returns the major.minor cycle from a version-like string.
func majorMinorCycle(version string) (string, error) {
	parts := numericParts(version)
	if len(parts) < 2 {
		return "", fmt.Errorf("version %q does not contain major and minor components", version)
	}

	return fmt.Sprintf("%d.%d", parts[0], parts[1]), nil
}

// findEOLCycle returns the release cycle matching the runtime version.
func findEOLCycle(cycles []model.EOLCycle, cycle string) (model.EOLCycle, bool) {
	for _, release := range cycles {
		if strings.TrimSpace(release.Cycle) == cycle {
			return release, true
		}
	}

	return model.EOLCycle{}, false
}

func versionsDiffer(cmdbVersion, runtimeVersion string) bool {
	cmdbVersion = strings.TrimSpace(cmdbVersion)
	runtimeVersion = strings.TrimSpace(runtimeVersion)
	if cmdbVersion == "" || runtimeVersion == "" {
		return false
	}

	return compareVersion(cmdbVersion, runtimeVersion) != 0
}

func versionIsBehind(runtimeVersion, latestVersion string) bool {
	runtimeVersion = strings.TrimSpace(runtimeVersion)
	latestVersion = strings.TrimSpace(latestVersion)
	if runtimeVersion == "" || latestVersion == "" {
		return false
	}

	return compareVersion(runtimeVersion, latestVersion) < 0
}
