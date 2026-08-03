package model

const (
	// SourceStatusOK means the source was queried successfully.
	SourceStatusOK = "ok"

	// SourceStatusDisabled means the source is disabled in products.yaml.
	SourceStatusDisabled = "disabled"

	// SourceStatusSkipped means collection is excluded by the service environment.
	SourceStatusSkipped = "skipped"

	// SourceStatusError means the source failed while the overall response remains available.
	SourceStatusError = "error"

	// SourceStatusPartial means the source succeeded but some derived fields are unavailable.
	SourceStatusPartial = "partial"
)

// VersionResponse is the response returned by GET /api/versions.
type VersionResponse struct {
	Products []ProductVersion `json:"products"`
}

// ProductVersion represents the aggregated version view for one product.
type ProductVersion struct {
	Key      string          `json:"key"`
	Metadata ProductMetadata `json:"metadata"`
	Sources  VersionSources  `json:"sources"`
}

// ProductMetadata contains environment-independent product metadata.
type ProductMetadata struct {
	DisplayName     string `json:"display_name"`
	ApplicationType string `json:"application_type,omitempty"`
}

// VersionSources contains all source results for one product.
type VersionSources struct {
	CMDB    CMDBResult          `json:"cmdb"`
	Runtime RuntimeSourceResult `json:"runtime"`
	EOL     EOLResult           `json:"eol"`
}

// RuntimeSourceResult contains all environment-specific runtime results.
type RuntimeSourceResult struct {
	Deployments []RuntimeDeploymentResult `json:"deployments"`
}

// RuntimeDeploymentResult represents one environment's runtime version.
type RuntimeDeploymentResult struct {
	Env        string            `json:"env"`
	Type       string            `json:"type,omitempty"`
	Status     string            `json:"status"`
	Version    string            `json:"version,omitempty"`
	Candidates []string          `json:"candidates,omitempty"`
	Error      string            `json:"error,omitempty"`
	Reason     string            `json:"reason,omitempty"`
	Assessment VersionAssessment `json:"assessment"`
}

type CMDBResult struct {
	Status   string        `json:"status"`
	Version  string        `json:"version,omitempty"`
	Versions []CMDBVersion `json:"versions,omitempty"`
	Error    string        `json:"error,omitempty"`
}

type CMDBVersion struct {
	Version        string `json:"version"`
	LifecycleState string `json:"lifecycle_state"`
}

// EOLResult represents product-level lifecycle information from endoflife.date.
type EOLResult struct {
	Status string `json:"status"`
	// Product is the endoflife.date product name, such as "jira-software" or "grafana-loki".
	Product string `json:"product,omitempty"`

	// PreferLTS indicates that latest-release selection is limited to LTS cycles.
	PreferLTS bool `json:"prefer_lts"`

	// LatestOverall is the latest release selected by the product's EOL policy.
	// If prefer_lts=true, it is the latest release among LTS cycles only.
	// If prefer_lts=false, it is the latest release across all cycles.
	LatestOverall      string `json:"latest_overall,omitempty"`
	LatestOverallIsLTS bool   `json:"latest_overall_is_lts"`
	LatestOverallDate  string `json:"latest_overall_date,omitempty"`

	// LatestOverallCycle is the cycle of LatestOverall.
	LatestOverallCycle string `json:"latest_overall_cycle,omitempty"`

	Cycles []EOLCycle `json:"cycles,omitempty"`
	Error  string     `json:"error,omitempty"`
}

// EOLCycle contains one product release cycle returned by endoflife.date.
type EOLCycle struct {
	Cycle        string `json:"cycle"`
	Label        string `json:"label,omitempty"`
	ReleaseDate  string `json:"release_date,omitempty"`
	IsLTS        bool   `json:"is_lts"`
	IsEOL        bool   `json:"is_eol"`
	EOLFrom      string `json:"eol_from,omitempty"`
	IsMaintained bool   `json:"is_maintained"`
	Latest       string `json:"latest,omitempty"`
	LatestDate   string `json:"latest_date,omitempty"`
}

// VersionAssessment is a derived view of one runtime version using the
// product-level CMDB and EOL source results.
type VersionAssessment struct {
	Status string `json:"status"`

	CurrentCycle             string `json:"current_cycle,omitempty"`
	CurrentCycleLabel        string `json:"current_cycle_label,omitempty"`
	CurrentCycleReleaseDate  string `json:"current_cycle_release_date,omitempty"`
	IsLTS                    bool   `json:"is_lts"`
	IsEOL                    bool   `json:"is_eol"`
	EOLFrom                  string `json:"eol_from,omitempty"`
	IsMaintained             bool   `json:"is_maintained"`
	LatestInCurrentCycle     string `json:"latest_in_current_cycle,omitempty"`
	LatestInCurrentCycleDate string `json:"latest_in_current_cycle_date,omitempty"`

	CMDBMismatch   bool `json:"cmdb_mismatch"`
	PatchAvailable bool `json:"patch_available"`

	Error  string `json:"error,omitempty"`
	Reason string `json:"reason,omitempty"`
}

// NewDisabledRuntimeResult creates a disabled runtime deployment result.
func NewDisabledRuntimeResult(env, runtimeType string) RuntimeDeploymentResult {
	return RuntimeDeploymentResult{
		Env:    env,
		Type:   runtimeType,
		Status: SourceStatusDisabled,
		Assessment: VersionAssessment{
			Status: SourceStatusDisabled,
		},
	}
}

// NewSkippedRuntimeResult creates a runtime result excluded by environment policy.
func NewSkippedRuntimeResult(env, runtimeType, reason string) RuntimeDeploymentResult {
	return RuntimeDeploymentResult{
		Env:    env,
		Type:   runtimeType,
		Status: SourceStatusSkipped,
		Reason: reason,
		Assessment: VersionAssessment{
			Status: SourceStatusSkipped,
			Reason: reason,
		},
	}
}

// NewErrorRuntimeResult creates a runtime deployment result for a failed source.
func NewErrorRuntimeResult(env, runtimeType string, err error) RuntimeDeploymentResult {
	msg := ""
	if err != nil {
		msg = err.Error()
	}

	return RuntimeDeploymentResult{
		Env:    env,
		Type:   runtimeType,
		Status: SourceStatusError,
		Error:  msg,
		Assessment: VersionAssessment{
			Status: SourceStatusPartial,
			Error:  "runtime version is unavailable",
		},
	}
}

// NewOKRuntimeResult creates a successful runtime source result.
func NewOKRuntimeResult(env, runtimeType, version string) RuntimeDeploymentResult {
	return RuntimeDeploymentResult{
		Env:     env,
		Type:    runtimeType,
		Status:  SourceStatusOK,
		Version: version,
		Assessment: VersionAssessment{
			Status: SourceStatusPartial,
			Error:  "lifecycle assessment has not been calculated",
		},
	}
}

// NewOKRuntimeResultWithCandidates creates a successful runtime source result with candidate versions.
func NewOKRuntimeResultWithCandidates(env, runtimeType, version string, candidates []string) RuntimeDeploymentResult {
	result := RuntimeDeploymentResult{
		Env:     env,
		Type:    runtimeType,
		Status:  SourceStatusOK,
		Version: version,
		Assessment: VersionAssessment{
			Status: SourceStatusPartial,
			Error:  "lifecycle assessment has not been calculated",
		},
	}

	if len(candidates) > 1 {
		result.Candidates = candidates
	}

	return result
}

// NewDisabledCMDBResult creates a disabled CMDB source result.
func NewDisabledCMDBResult() CMDBResult {
	return CMDBResult{
		Status: SourceStatusDisabled,
	}
}

// NewErrorCMDBResult creates a CMDB source result for a failed source.
func NewErrorCMDBResult(err error) CMDBResult {
	msg := ""
	if err != nil {
		msg = err.Error()
	}

	return CMDBResult{
		Status: SourceStatusError,
		Error:  msg,
	}
}

// NewOKCMDBResult creates a successful CMDB source result.
func NewOKCMDBResult(version string, versions []CMDBVersion) CMDBResult {
	return CMDBResult{
		Status:   SourceStatusOK,
		Version:  version,
		Versions: versions,
	}
}

// NewDisabledEOLResult creates a disabled EOL source result.
func NewDisabledEOLResult() EOLResult {
	return EOLResult{
		Status: SourceStatusDisabled,
	}
}

// NewErrorEOLResult creates an EOL source result for a failed source.
func NewErrorEOLResult(err error) EOLResult {
	msg := ""
	if err != nil {
		msg = err.Error()
	}

	return EOLResult{
		Status: SourceStatusError,
		Error:  msg,
	}
}
