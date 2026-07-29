package cmdb

import "strings"

// Default values used by the Product Versions CMDB query.
const (
	VersionSchemaName         = "IT Asset"
	VersionObjectType         = "Versions"
	VersionLifecycleStateGA   = "General Availability"
	VersionLifecycleStateEOL  = "End of Life"
	VersionAttrVersionNumber  = "Version Number"
	VersionAttrLifecycleState = "Lifecycle State"

	DefaultVersionPage     = 1
	DefaultVersionPageSize = 10
)

// VersionObjectQuery represents query parameters for the CMDB versions API.
type VersionObjectQuery struct {
	SchemaName   string
	ObjectType   string
	IQL          string
	ChildType    bool
	AttrsInclude []string
	Page         int
	PageSize     int
}

// Normalize fills default values for the standard Product Versions query.
func (q *VersionObjectQuery) Normalize() {
	if q.SchemaName == "" {
		q.SchemaName = VersionSchemaName
	}

	if q.ObjectType == "" {
		q.ObjectType = VersionObjectType
	}

	if len(q.AttrsInclude) == 0 {
		q.AttrsInclude = []string{
			VersionAttrVersionNumber,
			VersionAttrLifecycleState,
		}
	}

	if q.Page <= 0 {
		q.Page = DefaultVersionPage
	}

	if q.PageSize <= 0 {
		q.PageSize = DefaultVersionPageSize
	}
}

// VersionObjectResponse represents the CMDB versions API response.
type VersionObjectResponse struct {
	Meta    VersionObjectMeta `json:"meta"`
	Objects []VersionObject   `json:"objects"`
}

// VersionObjectMeta contains CMDB pagination metadata.
type VersionObjectMeta struct {
	FirstPage  int `json:"firstPage"`
	LastPage   int `json:"lastPage"`
	Page       int `json:"page"`
	Total      int `json:"total"`
	TotalPages int `json:"totalPages"`
}

// VersionObject represents one returned CMDB version object.
type VersionObject struct {
	Attrs      map[string]string `json:"attrs"`
	Label      string            `json:"label"`
	ObjectType string            `json:"objectType"`
}

// VersionInfo represents version and lifecyclestate object
type VersionInfo struct {
	Version        string
	LifecycleState string
}

// VersionInfos returns all non-empty version and its lifecycle state found in the response
func (r *VersionObjectResponse) VersionInfos() []VersionInfo {
	if r == nil || len(r.Objects) == 0 {
		return nil
	}

	infos := make([]VersionInfo, 0, len(r.Objects))
	for _, obj := range r.Objects {
		version := strings.TrimSpace(obj.Attrs[VersionAttrVersionNumber])
		if version == "" {
			continue
		}

		lifecycleState := strings.TrimSpace(obj.Attrs[VersionAttrLifecycleState])

		infos = append(infos, VersionInfo{
			Version:        version,
			LifecycleState: lifecycleState,
		})
	}

	return infos
}
