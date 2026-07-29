package provider

import (
	"context"
	"fmt"
	"product-versions/internal/cmdb"
	"product-versions/internal/versions/model"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

// CMDBSource fetches registered product versions from CMDB.
//
// It is a versions data source adapter. It does not create or own the low-level
// CMDB HTTP client; instead, it reuses the cmdb.Client created during startup.
type CMDBSource struct {
	client *cmdb.Client
}

// NewCMDBSource creates a CMDB versions source using an existing CMDB client.
func NewCMDBSource(client *cmdb.Client) *CMDBSource {
	return &CMDBSource{
		client: client,
	}
}

// Fetch queries CMDB for registered versions of a product.
//
// CMDB may return multiple version objects across supported lifecycle states.
// The source returns all supported versions and selects the highest General
// Availability version as the primary CMDB version. If no GA version is
// available, it falls back to the highest returned version.
func (s *CMDBSource) Fetch(ctx context.Context, product CMDBProduct) (model.CMDBResult, error) {
	iql := buildVersionIQL(product.Name, product.ApplicationType)
	resp, err := s.client.QueryVersionObjects(ctx, cmdb.VersionObjectQuery{
		IQL:       iql,
		ChildType: true,
	})
	if err != nil {
		return model.CMDBResult{}, err
	}

	versions := normalizeAndSortCMDBVersions(resp.VersionInfos())
	if len(versions) == 0 {
		return model.CMDBResult{}, fmt.Errorf("product %q cmdb returned no usable version values", product.Key)
	}

	primary := selectPrimaryCMDBVersion(versions)

	return model.NewOKCMDBResult(primary, versions), nil
}

// buildVersionIQL builds the CMDB IQL used to query version objects.
func buildVersionIQL(name, applicationType string) string {
	name = strings.TrimSpace(name)
	applicationType = strings.TrimSpace(applicationType)

	return fmt.Sprintf(
		`"Linked to".Name="%s" AND "Linked to"."Application Type"="%s" AND ("Lifecycle State"="%s" OR "Lifecycle State"="%s")`,
		escapeIQLString(name),
		escapeIQLString(applicationType),
		escapeIQLString(cmdb.VersionLifecycleStateGA),
		escapeIQLString(cmdb.VersionLifecycleStateEOL),
	)
}

// escapeIQLString escapes double quotes in IQL string values.
func escapeIQLString(value string) string {
	return strings.ReplaceAll(value, `"`, `\"`)
}

// normalizeAndSortCMDBVersions normalizes, deduplicates, and sorts CMDB version items.
//
// Sort order:
// 1. General Availability first.
// 2. End of Life second.
// 3. Unknown or other lifecycle states last.
// 4. Within the same lifecycle state, higher version comes first.
func normalizeAndSortCMDBVersions(infos []cmdb.VersionInfo) []model.CMDBVersion {
	if len(infos) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(infos))
	versions := make([]model.CMDBVersion, 0, len(infos))

	for _, info := range infos {
		if _, exists := seen[info.Version]; exists {
			continue
		}

		seen[info.Version] = struct{}{}
		versions = append(versions, model.CMDBVersion{
			Version:        info.Version,
			LifecycleState: info.LifecycleState,
		})
	}

	sort.Slice(versions, func(i, j int) bool {
		left := versions[i]
		right := versions[j]

		leftRank := lifecycleStateRank(left.LifecycleState)
		rightRank := lifecycleStateRank(right.LifecycleState)

		if leftRank != rightRank {
			return leftRank < rightRank
		}

		// Higher version first within the same lifecycle state.
		return compareVersion(left.Version, right.Version) > 0
	})

	return versions
}

// lifecycleStateRank defines display and selection grouping order.
//
// Lower rank means higher priority.
func lifecycleStateRank(state string) int {
	switch state {
	case cmdb.VersionLifecycleStateGA:
		return 0
	case cmdb.VersionLifecycleStateEOL:
		return 1
	default:
		return 2
	}
}

// uniqueAndSortVersions deduplicates and sorts version strings.
func uniqueAndSortVersions(values []string) []string {
	seen := make(map[string]struct{})
	versions := make([]string, 0, len(values))

	for _, value := range values {
		if _, exists := seen[value]; exists {
			continue
		}

		seen[value] = struct{}{}
		versions = append(versions, value)
	}

	sort.Slice(versions, func(i, j int) bool {
		return compareVersion(versions[i], versions[j]) < 0
	})

	return versions
}

// compareVersion compares version-like strings using numeric components.
//
// If numeric components are equal, it falls back to strings.Compare to keep
// ordering deterministic for non-standard version strings.
func compareVersion(a, b string) int {
	aParts := numericParts(a)
	bParts := numericParts(b)

	maxLen := len(aParts)
	if len(bParts) > maxLen {
		maxLen = len(bParts)
	}

	for i := 0; i < maxLen; i++ {
		aPart := 0
		bPart := 0

		if i < len(aParts) {
			aPart = aParts[i]
		}

		if i < len(bParts) {
			bPart = bParts[i]
		}

		if aPart < bPart {
			return -1
		}

		if aPart > bPart {
			return 1
		}
	}

	return strings.Compare(a, b)
}

// numericParts extracts numeric components from a version-like string.
func numericParts(version string) []int {
	fields := strings.FieldsFunc(version, func(r rune) bool {
		return !unicode.IsDigit(r)
	})

	parts := make([]int, 0, len(fields))
	for _, field := range fields {
		if field == "" {
			continue
		}

		n, err := strconv.Atoi(field)
		if err != nil {
			continue
		}

		parts = append(parts, n)
	}

	return parts
}

// selectPrimaryCMDBVersion selects the CMDB version used for comparison.
func selectPrimaryCMDBVersion(versions []model.CMDBVersion) string {
	if len(versions) == 0 {
		return ""
	}

	return versions[0].Version
}
