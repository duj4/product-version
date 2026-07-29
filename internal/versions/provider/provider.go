package provider

// CMDBProduct contains the product fields required by the CMDB version provider.
type CMDBProduct struct {
	Key             string
	Name            string
	ApplicationType string
}

// RuntimeProduct contains the product fields required by the runtime provider.
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

// RuntimeMimir contains the product fields required by the Mimir runtime provider.
type RuntimeMimir struct {
	Endpoint     string
	Auth         RuntimeAuth
	Headers      map[string]string
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

// EOLProduct contains the product fields required by the endoflife.date provider.
type EOLProduct struct {
	Key       string
	Product   string
	PreferLTS bool
}
