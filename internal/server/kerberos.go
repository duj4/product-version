package server

import (
	"path/filepath"

	"product-version/internal/logger"
	"product-version/internal/versions"
	"product-version/internal/versions/source"
)

const (
	defaultKerberosDir    = "/d/d1/product-version/kerberos"
	defaultKrb5Config     = "/etc/krb5.conf"
	keytabPrincipalPrefix = "itsm_version_"
)

// resolveRuntimeKerberosProfiles builds environment-specific profiles without
// reading optional keytabs or krb5.conf during service startup.
func resolveRuntimeKerberosProfiles(serviceEnv string) map[string]source.RuntimeKerberosProfile {
	kerberosDir, dirSource := resolveEnvVarValue("APP_KERBEROS_DIR", defaultKerberosDir)
	logResolvedEnvValue("kerberos directory resolved", "APP_KERBEROS_DIR", kerberosDir, dirSource)

	configPath, configSource := resolveEnvVarValue("APP_KRB5_CONFIG", defaultKrb5Config)
	logResolvedEnvValue("kerberos configuration resolved", "APP_KRB5_CONFIG", configPath, configSource)

	environments := []string{versions.EnvironmentQA}
	if serviceEnv == versions.EnvironmentProd {
		environments = append(environments, versions.EnvironmentProd)
	}

	profiles := make(map[string]source.RuntimeKerberosProfile, len(environments))
	for _, env := range environments {
		principal := keytabPrincipalPrefix + env
		profiles[env] = source.RuntimeKerberosProfile{
			Principal:  principal,
			KeytabPath: filepath.Join(kerberosDir, principal+".keytab"),
			ConfigPath: configPath,
		}
	}

	logger.Info(
		"kerberos runtime profiles resolved",
		"env", serviceEnv,
		"profile_count", len(profiles),
		"file_validation", "deferred",
	)

	return profiles
}
