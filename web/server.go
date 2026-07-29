package web

import (
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"product-version/internal/api"
	"product-version/internal/cmdb"
	"product-version/internal/logger"
	"product-version/internal/middleware"
	"product-version/internal/versions"
	"product-version/internal/versions/provider"

	"github.com/gin-gonic/gin"
)

//go:embed templates/*.html static
var webFiles embed.FS

const (
	defaultListenAddr = ":8443"
	defaultTLSDir     = "/d/d1/product-version/tls"
	defaultCACertFile = "/etc/pki/ca-trust/source/anchors/katello-server-ca.pem"
	defaultConfigDir  = "/d/d1/product-version/config"
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

// Run initializes and starts the Product Version web service.
//
// It loads configuration, prepares shared clients, registers embedded
// templates and static assets, and starts the HTTPS server.
func Run() error {
	// Resolve the runtime environment.
	env := strings.ToLower(strings.TrimSpace(os.Getenv("APP_ENV")))
	if env == "" {
		env = "prod"
	}

	if env != "qa" && env != "prod" {
		return fmt.Errorf("unsupported APP_ENV %q, expected qa or prod", env)
	}

	// Resolve TLS assets for the server and outbound client requests.
	tlsPaths, err := resolveTLSPaths()
	if err != nil {
		return err
	}

	if err := validateTLSPaths(tlsPaths); err != nil {
		return err
	}

	// Resolve the configuration directory for the single service deployment.
	configDir := resolveConfigDir()

	// Build paths to the configuration files used by this process.
	cmdbConfigPath := filepath.Join(configDir, "cmdb.json")
	productsConfigPath := filepath.Join(configDir, "products.yaml")

	logger.Info(
		"loading configuration",
		"env", env,
		"config_dir", configDir,
		"cmdb_config", cmdbConfigPath,
		"products_config", productsConfigPath,
	)

	// Load and validate CMDB configuration.
	cmdbConfig, err := cmdb.LoadConfig(cmdbConfigPath)
	if err != nil {
		return fmt.Errorf("failed to load cmdb config: %w", err)
	}

	// CMDB is always queried in Prod, regardless of runtime deployment env.
	prodTLSProfile := tlsPaths.ClientProfiles[versions.EnvironmentProd]
	cmdbConfig.CACertPath = prodTLSProfile.CACert
	cmdbConfig.ClientCertPath = prodTLSProfile.ClientCert
	cmdbConfig.ClientKeyPath = prodTLSProfile.ClientKey

	// Create the shared CMDB client.
	cmdbClient, err := cmdb.NewClient(cmdbConfig)
	if err != nil {
		return fmt.Errorf("failed to create CMDB client: %w", err)
	}

	if cmdbClient == nil {
		return fmt.Errorf("failed to create CMDB client: client is nil")
	}

	// Load version-tracking product configuration.
	versionsConfig, err := versions.LoadConfig(productsConfigPath)
	if err != nil {
		return fmt.Errorf("failed to load versions config: %w", err)
	}

	runtimeProfiles := make(map[string]provider.RuntimeTLSProfile, len(tlsPaths.ClientProfiles))
	for profileEnv, profile := range tlsPaths.ClientProfiles {
		runtimeProfiles[profileEnv] = provider.RuntimeTLSProfile{
			CACertPath:     profile.CACert,
			ClientCertPath: profile.ClientCert,
			ClientKeyPath:  profile.ClientKey,
		}
	}

	runtimeSource, err := provider.NewRuntimeSource(0, runtimeProfiles)
	if err != nil {
		return fmt.Errorf("failed to create runtime source: %w", err)
	}

	versionsService := versions.NewService(versionsConfig, cmdbClient, runtimeSource)

	// Set Gin mode before creating the engine.
	if env == "prod" {
		gin.SetMode(gin.ReleaseMode)
		logger.Info("gin running in release mode (Prod)")
	} else {
		logger.Info("gin running in debug mode (QA)")
	}

	// Initialize the Gin engine and middleware stack.
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(middleware.GinLogger())

	if env == "prod" {
		if err := r.SetTrustedProxies(nil); err != nil {
			return fmt.Errorf("failed to set trusted proxies: %w", err)
		}
	}

	// Load embedded HTML templates.
	tmpl, err := template.ParseFS(webFiles, "templates/*.html")
	if err != nil {
		return fmt.Errorf("failed to parse embedded templates: %w", err)
	}
	r.SetHTMLTemplate(tmpl)

	// Load embedded static assets.
	staticFS, err := fs.Sub(webFiles, "static")
	if err != nil {
		return fmt.Errorf("failed to load embedded static files: %w", err)
	}

	// Expose static assets only under /static.
	r.StaticFS("/static", http.FS(staticFS))

	// Register pages and API routes.
	registerRoutes(r, versionsService)

	// Start the HTTPS server.
	return runTLSServer(r, env, tlsPaths.ServerCert, tlsPaths.ServerKey)

}

// registerRoutes registers all page and API routes for the web service.
//
// The API handler shares the long-lived versions service and its client pools.
func registerRoutes(r *gin.Engine, versionsService *versions.Service) {
	// Health check.
	r.GET("/healthz", api.HealthHandler)

	r.GET("/", func(c *gin.Context) {
		c.Redirect(http.StatusFound, "/versions")
	})

	// Product versions.
	r.GET("/api/versions", api.ListVersionsHandler(versionsService))

	r.GET("/versions", func(c *gin.Context) {
		c.HTML(http.StatusOK, "versions.html", nil)
	})
}

// runTLSServer starts the Gin HTTPS server.
func runTLSServer(r *gin.Engine, env, certFilePath, keyFilePath string) error {
	listenAddr := strings.TrimSpace(os.Getenv("APP_LISTEN_ADDR"))
	if listenAddr == "" {
		listenAddr = defaultListenAddr
	}

	logger.Info(
		"starting service",
		"listen", listenAddr,
		"env", env,
		"cert", certFilePath,
		"key", keyFilePath,
	)

	if err := r.RunTLS(listenAddr, certFilePath, keyFilePath); err != nil {
		return fmt.Errorf("failed to start HTTPS server: %w", err)
	}

	return nil
}

// resolveConfigDir resolves the effective service configuration directory.
func resolveConfigDir() string {
	baseConfigDir := strings.TrimSpace(os.Getenv("APP_CONFIG_DIR"))
	if baseConfigDir == "" {
		baseConfigDir = defaultConfigDir
		logger.Info("APP_CONFIG_DIR not set, using default", "path", baseConfigDir)
	}

	return baseConfigDir
}

// resolveTLSPaths returns the server certificate and both supported outbound
// client profiles.
func resolveTLSPaths() (TLSPaths, error) {
	tlsDir := strings.TrimSpace(os.Getenv("APP_TLS_DIR"))
	if tlsDir == "" {
		tlsDir = defaultTLSDir
	}

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
