package web

import (
	"html/template"
	"io/fs"
	"path/filepath"
	"regexp"
	"testing"
)

func TestEmbeddedWebAssetsStayConnected(t *testing.T) {
	t.Parallel()

	if _, err := template.ParseFS(webFiles, "templates/*.html"); err != nil {
		t.Fatalf("template.ParseFS() error = %v", err)
	}

	html, err := fs.ReadFile(webFiles, "templates/versions.html")
	if err != nil {
		t.Fatalf("read versions template: %v", err)
	}
	javascript, err := fs.ReadFile(webFiles, "static/js/versions.js")
	if err != nil {
		t.Fatalf("read versions JavaScript: %v", err)
	}

	templateIDs := make(map[string]struct{})
	for _, match := range regexp.MustCompile(`id="([^"]+)"`).FindAllStringSubmatch(string(html), -1) {
		templateIDs[match[1]] = struct{}{}
	}

	for _, match := range regexp.MustCompile(`getElementById\("([^"]+)"\)`).FindAllStringSubmatch(string(javascript), -1) {
		if _, exists := templateIDs[match[1]]; !exists {
			t.Errorf("JavaScript references missing template id %q", match[1])
		}
	}
}

func TestResolveTLSPathsContainsBothRuntimeProfiles(t *testing.T) {
	t.Setenv("APP_TLS_DIR", "/tmp/product-versions-tls-test")

	paths, err := resolveTLSPaths()
	if err != nil {
		t.Fatalf("resolveTLSPaths() error = %v", err)
	}

	for env, certificateName := range map[string]string{
		"qa":   "itsm_jsm_qa.pem",
		"prod": "itsm_jsm_prod.pem",
	} {
		profile, exists := paths.ClientProfiles[env]
		if !exists {
			t.Fatalf("missing %q client profile", env)
		}
		if got := filepath.Base(profile.ClientCert); got != certificateName {
			t.Fatalf("%s certificate = %q, want %q", env, got, certificateName)
		}
	}
}
