package source

// CMDBProduct contains the product fields required by the CMDB version source.
type CMDBProduct struct {
	Key             string
	Name            string
	ApplicationType string
}

// RuntimeProduct contains the product fields required by the runtime source.
type RuntimeProduct struct {
	Key  string
	Env  string
	Type string

	// Direct HTTP runtime fields.
	Endpoint         string
	Method           string
	Auth             RuntimeAuth
	AcceptedStatuses []int
	VersionField     string

	// Mimir runtime fields.
	Mimir RuntimeMimir
}

// RuntimeMimir contains the product fields required by the Mimir runtime source.
type RuntimeMimir struct {
	Endpoint     string
	Auth         RuntimeAuth
	Query        string
	VersionLabel string
}

// RuntimeAuth defines how the runtime endpoint should be called.
type RuntimeAuth struct {
	Type string
}

// RuntimeTLSProfile contains the certificate paths for one environment.
type RuntimeTLSProfile struct {
	CACertPath     string
	ClientCertPath string
	ClientKeyPath  string
}

// EOLProduct contains the product fields required by the endoflife.date source.
type EOLProduct struct {
	Key       string
	Product   string
	PreferLTS bool
}

// RuntimeKerberosProfile contains the environment-specific Kerberos identity.
// Files are deliberately loaded only when a Kerberos-authenticated source runs.
type RuntimeKerberosProfile struct {
	Principal  string
	Realm      string
	KeytabPath string
	ConfigPath string
}
