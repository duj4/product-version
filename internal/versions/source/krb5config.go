package source

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jcmturner/gokrb5/v8/config"
)

const (
	maxKrb5IncludeDepth = 16
	maxKrb5ConfigBytes  = 4 << 20
)

type krb5ConfigLoader struct {
	active     map[string]struct{}
	loaded     map[string]struct{}
	loadedSize int
}

// loadKerberosConfig expands MIT Kerberos include directives before handing
// the resulting client configuration to gokrb5, whose parser does not expand
// include or includedir itself.
func loadKerberosConfig(path string) (*config.Config, int, error) {
	loader := &krb5ConfigLoader{
		active: make(map[string]struct{}),
		loaded: make(map[string]struct{}),
	}

	expanded, err := loader.expandFile(path, 0)
	if err != nil {
		return nil, 0, err
	}

	krb5Config, err := config.NewFromString(selectSupportedKrb5Sections(expanded))
	if err != nil {
		return nil, 0, fmt.Errorf("parse expanded Kerberos configuration %q: %w", path, err)
	}

	return krb5Config, len(loader.loaded), nil
}

func (l *krb5ConfigLoader) expandFile(path string, depth int) (string, error) {
	if depth > maxKrb5IncludeDepth {
		return "", fmt.Errorf("Kerberos configuration include depth exceeds %d at %q", maxKrb5IncludeDepth, path)
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve Kerberos configuration path %q: %w", path, err)
	}

	resolvedPath, err := filepath.EvalSymlinks(absPath)
	if err != nil {
		return "", fmt.Errorf("resolve Kerberos configuration file %q: %w", absPath, err)
	}
	if _, exists := l.active[resolvedPath]; exists {
		return "", fmt.Errorf("Kerberos configuration include cycle detected at %q", resolvedPath)
	}

	data, err := os.ReadFile(resolvedPath)
	if err != nil {
		return "", fmt.Errorf("read Kerberos configuration file %q: %w", resolvedPath, err)
	}
	l.loadedSize += len(data)
	if l.loadedSize > maxKrb5ConfigBytes {
		return "", fmt.Errorf("expanded Kerberos configuration exceeds %d bytes", maxKrb5ConfigBytes)
	}

	l.active[resolvedPath] = struct{}{}
	l.loaded[resolvedPath] = struct{}{}
	defer delete(l.active, resolvedPath)

	var expanded strings.Builder
	for _, line := range strings.Split(string(data), "\n") {
		kind, target, ok, err := parseKrb5IncludeDirective(line)
		if err != nil {
			return "", fmt.Errorf("parse Kerberos configuration file %q: %w", resolvedPath, err)
		}
		if !ok {
			expanded.WriteString(line)
			expanded.WriteByte('\n')
			continue
		}

		switch kind {
		case "include":
			included, err := l.expandFile(target, depth+1)
			if err != nil {
				return "", err
			}
			appendKrb5Config(&expanded, included)

		case "includedir":
			included, err := l.expandDirectory(target, depth+1)
			if err != nil {
				return "", err
			}
			appendKrb5Config(&expanded, included)
		}
	}

	return expanded.String(), nil
}

func (l *krb5ConfigLoader) expandDirectory(path string, depth int) (string, error) {
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("Kerberos includedir path %q must be absolute", path)
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		return "", fmt.Errorf("read Kerberos configuration directory %q: %w", path, err)
	}

	var expanded strings.Builder
	for _, entry := range entries {
		if !isKrb5IncludedFilename(entry.Name()) {
			continue
		}

		entryPath := filepath.Join(path, entry.Name())
		info, err := os.Stat(entryPath)
		if err != nil {
			return "", fmt.Errorf("inspect Kerberos configuration include %q: %w", entryPath, err)
		}
		if !info.Mode().IsRegular() {
			continue
		}

		included, err := l.expandFile(entryPath, depth)
		if err != nil {
			return "", err
		}
		appendKrb5Config(&expanded, included)
	}

	return expanded.String(), nil
}

func parseKrb5IncludeDirective(line string) (kind, target string, ok bool, err error) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, ";") {
		return "", "", false, nil
	}

	fields := strings.Fields(trimmed)
	if len(fields) == 0 {
		return "", "", false, nil
	}

	kind = strings.ToLower(fields[0])
	if kind != "include" && kind != "includedir" {
		return "", "", false, nil
	}
	if len(fields) != 2 {
		return "", "", false, fmt.Errorf("%s directive must contain exactly one path", kind)
	}

	target = strings.Trim(fields[1], "\"'")
	if !filepath.IsAbs(target) {
		return "", "", false, fmt.Errorf("%s path %q must be absolute", kind, target)
	}

	return kind, target, true, nil
}

func isKrb5IncludedFilename(name string) bool {
	if name == "" || strings.HasPrefix(name, ".") {
		return false
	}
	if strings.HasSuffix(name, ".conf") {
		return true
	}

	for i := 0; i < len(name); i++ {
		character := name[i]
		if isKrb5IncludedFilenameCharacter(character) {
			continue
		}
		return false
	}
	return true
}

func isKrb5IncludedFilenameCharacter(character byte) bool {
	return character >= 'a' && character <= 'z' ||
		character >= 'A' && character <= 'Z' ||
		character >= '0' && character <= '9' ||
		character == '-' || character == '_'
}

func appendKrb5Config(builder *strings.Builder, contents string) {
	if contents == "" {
		return
	}
	builder.WriteString(contents)
	if !strings.HasSuffix(contents, "\n") {
		builder.WriteByte('\n')
	}
}

// selectSupportedKrb5Sections combines repeated supported sections in source
// order. This preserves values from included files when the main distribution
// config later contains an empty [realms] section.
func selectSupportedKrb5Sections(contents string) string {
	sectionOrder := []string{"libdefaults", "realms", "domain_realm"}
	sections := make(map[string][]string, len(sectionOrder))
	currentSection := ""

	for _, line := range strings.Split(contents, "\n") {
		if section, isHeader := krb5SectionHeader(line); isHeader {
			switch section {
			case "libdefaults", "realms", "domain_realm":
				currentSection = section
			default:
				currentSection = ""
			}
			continue
		}
		if currentSection != "" {
			sections[currentSection] = append(sections[currentSection], line)
		}
	}

	var selected strings.Builder
	for _, section := range sectionOrder {
		lines := sections[section]
		if len(lines) == 0 {
			continue
		}
		selected.WriteByte('[')
		selected.WriteString(section)
		selected.WriteString("]\n")
		for _, line := range lines {
			selected.WriteString(line)
			selected.WriteByte('\n')
		}
	}
	return selected.String()
}

func krb5SectionHeader(line string) (string, bool) {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "[") {
		return "", false
	}
	closing := strings.IndexByte(trimmed, ']')
	if closing < 2 {
		return "", false
	}
	return strings.ToLower(strings.TrimSpace(trimmed[1:closing])), true
}
