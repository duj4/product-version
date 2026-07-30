package source

import (
	"sort"
	"strconv"
	"strings"
	"unicode"
)

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
