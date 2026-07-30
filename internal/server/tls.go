package server

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"product-version/internal/logger"
	"product-version/internal/versions"
)

const (
	defaultTLSDir     = "/d/d1/product-version/tls"
	defaultCACertFile = "/etc/pki/ca-trust/source/anchors/katello-server-ca.pem"
)

// TLSPaths groups the inbound server certificate and environment-specific
// outbound client certificate profiles.
type TLSPaths struct {
	ServerCert     string
	ServerKey      string
	ClientProfiles map[string]ClientTLSPaths
}

// ClientTLSPaths contains the mTLS assets for one outbound environment.
type ClientTLSPaths struct {
	CACert     string
	ClientCert string
	ClientKey  string
}

// resolveTLSPaths returns the server certificate and both supported outbound
// client profiles.
func resolveTLSPaths() (TLSPaths, error) {
	tlsDir := strings.TrimSpace(os.Getenv("APP_TLS_DIR"))
	if tlsDir == "" {
		tlsDir = defaultTLSDir
		logger.Info("APP_TLS_DIR not set, using default", "path", tlsDir)
	}
	logger.Info("APP_TLS_DIR set", "path", tlsDir)

	return TLSPaths{
		ServerCert: filepath.Join(tlsDir, "tls.pem"),
		ServerKey:  filepath.Join(tlsDir, "tls.key"),
		ClientProfiles: map[string]ClientTLSPaths{
			versions.EnvironmentQA: {
				ClientCert: filepath.Join(tlsDir, "itsm_jsm_qa.pem"),
				ClientKey:  filepath.Join(tlsDir, "itsm_jsm_qa.key"),
				CACert:     defaultCACertFile,
			},
			versions.EnvironmentProd: {
				ClientCert: filepath.Join(tlsDir, "itsm_jsm_prod.pem"),
				ClientKey:  filepath.Join(tlsDir, "itsm_jsm_prod.key"),
				CACert:     defaultCACertFile,
			},
		},
	}, nil
}

// validateTLSPaths verifies that every required TLS path is set and accessible.
func validateTLSPaths(paths TLSPaths) error {
	checks := map[string]string{
		"server cert": paths.ServerCert,
		"server key":  paths.ServerKey,
	}

	for env, profile := range paths.ClientProfiles {
		checks[env+" client cert"] = profile.ClientCert
		checks[env+" client key"] = profile.ClientKey
		checks[env+" CA cert"] = profile.CACert
	}

	for name, path := range checks {
		if strings.TrimSpace(path) == "" {
			return fmt.Errorf("%s path is empty", name)
		}

		if _, err := os.Stat(path); err != nil {
			return fmt.Errorf("%s %q is not accessible: %w", name, path, err)
		}
	}

	return nil
}
