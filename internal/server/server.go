package server

import (
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"product-version/internal/cmdb"
	"product-version/internal/logger"
	"product-version/internal/versions"
	"product-version/internal/versions/source"

	"github.com/gin-gonic/gin"
)

//go:embed templates/*.html static
var webFiles embed.FS

const (
	defaultPort      = "8443"
	defaultConfigDir = "/d/d1/product-version/config"
)

// Run initializes and starts the Product Version web service.
//
// It loads configuration, prepares shared clients, registers embedded
// templates and static assets, and starts the HTTPS server.
func Run() error {
	// Resolve the runtime environment.
	env, err := resolveAppEnvironment()
	if err != nil {
		return err
	}

	// Resolve TLS assets for the server and outbound client requests.
	tlsPaths, err := resolveTLSPaths(env)
	if err != nil {
		return err
	}

	if err := validateTLSPaths(tlsPaths); err != nil {
		return err
	}
	logger.Info(
		"tls paths validated",
		"env", env,
		"server_cert", tlsPaths.ServerCert,
		"server_key", tlsPaths.ServerKey,
		"client_profiles", len(tlsPaths.ClientProfiles),
	)

	// Resolve the configuration directory for the single service deployment.
	configDir := resolveConfigDir()

	// Build paths to the configuration files used by this process.
	cmdbConfigPath := filepath.Join(configDir, "cmdb.json")
	productsConfigPath := filepath.Join(configDir, "products.yaml")

	logger.Info(
		"configuration files resolved",
		"env", env,
		"config_dir", configDir,
		"cmdb_config", cmdbConfigPath,
		"products_config", productsConfigPath,
	)

	// Load and validate CMDB configuration.
	cmdbConfig, err := cmdb.LoadConfig(cmdbConfigPath, env)
	if err != nil {
		return fmt.Errorf("failed to load cmdb config: %w", err)
	}

	// APP_ENV=qa switches both the CMDB URL and its client certificate for testing.
	cmdbTLSProfile, exists := tlsPaths.ClientProfiles[env]
	if !exists {
		return fmt.Errorf("TLS client profile for CMDB environment %q is not configured", env)
	}
	cmdbConfig.CACertPath = cmdbTLSProfile.CACert
	cmdbConfig.ClientCertPath = cmdbTLSProfile.ClientCert
	cmdbConfig.ClientKeyPath = cmdbTLSProfile.ClientKey
	logger.Info(
		"cmdb configuration loaded",
		"env", env,
		"url", cmdbConfig.VersionsAPIURL,
		"tls_profile", env,
	)

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
	logger.Info(
		"product configuration loaded",
		"path", productsConfigPath,
		"product_count", len(versionsConfig.Products),
	)

	runtimeProfiles := make(map[string]source.RuntimeTLSProfile, len(tlsPaths.ClientProfiles))
	for profileEnv, profile := range tlsPaths.ClientProfiles {
		runtimeProfiles[profileEnv] = source.RuntimeTLSProfile{
			CACertPath:     profile.CACert,
			ClientCertPath: profile.ClientCert,
			ClientKeyPath:  profile.ClientKey,
		}
	}

	kerberosProfiles := resolveRuntimeKerberosProfiles(env)
	runtimeSource, err := source.NewRuntimeSource(0, runtimeProfiles, kerberosProfiles)
	if err != nil {
		return fmt.Errorf("failed to create runtime source: %w", err)
	}

	versionsService := versions.NewService(versionsConfig, cmdbClient, runtimeSource, env)
	logger.Info("version cache warmup started")
	versionsService.WarmCache()

	// Set Gin mode before creating the engine.
	ginMode := gin.DebugMode
	if env == versions.EnvironmentProd {
		ginMode = gin.ReleaseMode
	}
	gin.SetMode(ginMode)
	logger.Info(
		"gin mode configured",
		"mode", ginMode,
		"env", env,
	)

	// Initialize the Gin engine and middleware stack.
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(ginLogger())

	if env == versions.EnvironmentProd {
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

// runTLSServer starts the Gin HTTPS server.
func runTLSServer(r *gin.Engine, env, certFilePath, keyFilePath string) error {
	port := resolvePort()
	listenAddr := ":" + port

	logger.Info(
		"https server starting",
		"listen_addr", listenAddr,
		"env", env,
		"server_cert", certFilePath,
		"server_key", keyFilePath,
	)

	if err := r.RunTLS(listenAddr, certFilePath, keyFilePath); err != nil {
		return fmt.Errorf("failed to start HTTPS server: %w", err)
	}

	return nil
}

// resolveConfigDir resolves the effective service configuration directory.
func resolveConfigDir() string {
	baseConfigDir, source := resolveEnvVarValue("APP_CONFIG_DIR", defaultConfigDir)
	logResolvedEnvValue("configuration directory resolved", "APP_CONFIG_DIR", baseConfigDir, source)

	return baseConfigDir
}

// resolveAppEnvironment resolves and validates the service runtime environment.
func resolveAppEnvironment() (string, error) {
	env, source := resolveEnvVarValue("APP_ENV", versions.EnvironmentProd)
	env = strings.ToLower(env)
	if env != versions.EnvironmentQA && env != versions.EnvironmentProd {
		return "", fmt.Errorf("unsupported APP_ENV %q, expected qa or prod", env)
	}

	logResolvedEnvValue("application environment resolved", "APP_ENV", env, source)
	return env, nil
}

// resolvePort resolves the HTTPS server port.
func resolvePort() string {
	port, source := resolveEnvVarValue("APP_PORT", defaultPort)
	logResolvedEnvValue("https port resolved", "APP_PORT", port, source)
	return port
}

// resolveEnvVarValue returns a trimmed environment variable value or its default.
func resolveEnvVarValue(name, defaultValue string) (string, string) {
	value := strings.TrimSpace(os.Getenv(name))
	if value != "" {
		return value, "environment"
	}

	return defaultValue, "default"
}

// logResolvedEnvValue logs an environment-backed setting in a consistent format.
func logResolvedEnvValue(message, envVar, value, source string) {
	logger.Info(
		message,
		"env_var", envVar,
		"value", value,
		"source", source,
	)
}
