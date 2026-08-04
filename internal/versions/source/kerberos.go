package source

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"product-version/internal/httpclient"
	"product-version/internal/logger"

	"github.com/jcmturner/gokrb5/v8/client"
	"github.com/jcmturner/gokrb5/v8/config"
	"github.com/jcmturner/gokrb5/v8/keytab"
	"github.com/jcmturner/gokrb5/v8/spnego"
	"github.com/jcmturner/gokrb5/v8/types"
)

type runtimeHTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

type kerberosClientPool struct {
	timeout  time.Duration
	profiles map[string]RuntimeKerberosProfile

	mu      sync.Mutex
	clients map[string]*synchronizedSPNEGOClient
}

type synchronizedSPNEGOClient struct {
	mu     sync.Mutex
	client *spnego.Client
}

func newKerberosClientPool(timeout time.Duration, profiles map[string]RuntimeKerberosProfile) *kerberosClientPool {
	copiedProfiles := make(map[string]RuntimeKerberosProfile, len(profiles))
	for env, profile := range profiles {
		copiedProfiles[env] = profile
	}

	return &kerberosClientPool{
		timeout:  timeout,
		profiles: copiedProfiles,
		clients:  make(map[string]*synchronizedSPNEGOClient),
	}
}

func (p *kerberosClientPool) clientFor(env string) (runtimeHTTPClient, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if existing, ok := p.clients[env]; ok {
		return existing, nil
	}

	profile, ok := p.profiles[env]
	if !ok {
		return nil, fmt.Errorf("no runtime Kerberos profile configured for env %q", env)
	}

	principal := strings.TrimSpace(profile.Principal)
	if principal == "" {
		return nil, fmt.Errorf("runtime Kerberos principal for env %q is empty", env)
	}

	configPath := strings.TrimSpace(profile.ConfigPath)
	if configPath == "" {
		return nil, fmt.Errorf("runtime Kerberos config path for env %q is empty", env)
	}

	krb5Config, configFileCount, err := loadKerberosConfig(configPath)
	if err != nil {
		return nil, fmt.Errorf("load runtime Kerberos configuration for env %q: %w", env, err)
	}

	keytabPath := strings.TrimSpace(profile.KeytabPath)
	if keytabPath == "" {
		return nil, fmt.Errorf("runtime Kerberos keytab path for env %q is empty", env)
	}

	credential, err := keytab.Load(keytabPath)
	if err != nil {
		return nil, fmt.Errorf("load runtime Kerberos keytab %q for env %q: %w", keytabPath, env, err)
	}

	resolvedPrincipal, keytabRealm, err := resolveKeytabIdentity(principal, credential)
	if err != nil {
		return nil, fmt.Errorf("resolve runtime Kerberos identity for env %q: %w", env, err)
	}

	realm := strings.TrimSpace(profile.Realm)
	if realm == "" {
		realm = keytabRealm
	}
	if realm == "" {
		realm = strings.TrimSpace(krb5Config.LibDefaults.DefaultRealm)
	}
	if realm == "" {
		return nil, fmt.Errorf("runtime Kerberos realm for env %q is not present in the profile, keytab, or krb5 configuration", env)
	}
	if keytabRealm != "" && !strings.EqualFold(realm, keytabRealm) {
		return nil, fmt.Errorf("runtime Kerberos realm %q for env %q does not match keytab realm %q", realm, env, keytabRealm)
	}
	principal = resolvedPrincipal
	krb5Config.LibDefaults.DefaultRealm = realm
	kdcDiscovery := configureKerberosKDCDiscovery(krb5Config, realm)

	krb5Client := client.NewWithKeytab(
		principal,
		realm,
		credential,
		krb5Config,
		client.DisablePAFXFAST(true),
	)
	configured, err := krb5Client.IsConfigured()
	if err != nil {
		return nil, fmt.Errorf("validate runtime Kerberos client for env %q: %w", env, err)
	}
	if !configured {
		return nil, fmt.Errorf("runtime Kerberos client for env %q is not configured", env)
	}

	spnegoClient := &synchronizedSPNEGOClient{
		client: spnego.NewClient(krb5Client, httpclient.NewClient(p.timeout), ""),
	}
	p.clients[env] = spnegoClient

	logger.Info(
		"kerberos runtime client initialized",
		"env", env,
		"principal", principal,
		"realm", realm,
		"keytab", keytabPath,
		"krb5_config", configPath,
		"krb5_config_files", configFileCount,
		"kdc_discovery", kdcDiscovery,
	)

	return spnegoClient, nil
}

func resolveKeytabIdentity(requestedPrincipal string, credential *keytab.Keytab) (string, string, error) {
	requestedName, requestedRealm := types.ParseSPNString(strings.TrimSpace(requestedPrincipal))
	if len(requestedName.NameString) == 0 || strings.TrimSpace(requestedName.NameString[0]) == "" {
		return "", "", fmt.Errorf("requested principal is empty")
	}

	resolvedPrincipal := ""
	resolvedRealm := ""
	for _, entry := range credential.Entries {
		components := entry.Principal.Components
		if !equalPrincipalComponents(components, requestedName.NameString) {
			continue
		}

		entryRealm := strings.TrimSpace(entry.Principal.Realm)
		if entryRealm == "" {
			continue
		}
		if requestedRealm != "" && !strings.EqualFold(requestedRealm, entryRealm) {
			continue
		}

		entryPrincipal := strings.Join(components, "/")
		if resolvedPrincipal == "" {
			resolvedPrincipal = entryPrincipal
			resolvedRealm = entryRealm
			continue
		}
		if !strings.EqualFold(resolvedPrincipal, entryPrincipal) || !strings.EqualFold(resolvedRealm, entryRealm) {
			return "", "", fmt.Errorf("keytab contains principal %q in multiple realms", requestedPrincipal)
		}
	}

	if resolvedPrincipal == "" {
		return "", "", fmt.Errorf("keytab does not contain principal %q", requestedPrincipal)
	}
	return resolvedPrincipal, resolvedRealm, nil
}

func equalPrincipalComponents(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if !strings.EqualFold(left[i], right[i]) {
			return false
		}
	}
	return true
}

func configureKerberosKDCDiscovery(krb5Config *config.Config, realm string) string {
	matchingRealms := make([]int, 0, 1)
	kdcs := make([]string, 0)
	for i := range krb5Config.Realms {
		configuredRealm := &krb5Config.Realms[i]
		if !strings.EqualFold(configuredRealm.Realm, realm) {
			continue
		}
		configuredRealm.Realm = realm
		matchingRealms = append(matchingRealms, i)
		for _, kdc := range configuredRealm.KDC {
			if !containsString(kdcs, kdc) {
				kdcs = append(kdcs, kdc)
			}
		}
	}

	if len(kdcs) > 0 {
		for _, index := range matchingRealms {
			krb5Config.Realms[index].KDC = append([]string(nil), kdcs...)
		}
		return "configuration"
	}
	if krb5Config.LibDefaults.DNSLookupKDC {
		return "dns"
	}
	krb5Config.LibDefaults.DNSLookupKDC = true
	return "dns_fallback"
}

func containsString(values []string, candidate string) bool {
	for _, value := range values {
		if value == candidate {
			return true
		}
	}
	return false
}

func (c *synchronizedSPNEGOClient) Do(req *http.Request) (*http.Response, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := req.Context().Err(); err != nil {
		return nil, err
	}
	return c.client.Do(req)
}
