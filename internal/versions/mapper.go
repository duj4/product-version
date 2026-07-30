package versions

import "product-version/internal/versions/source"

func toRuntimeProduct(productKey string, deployment RuntimeDeploymentConfig) source.RuntimeProduct {
	return source.RuntimeProduct{
		Key:              productKey,
		Env:              deployment.Env,
		Type:             deployment.Type,
		Endpoint:         deployment.Endpoint,
		Method:           deployment.Method,
		Auth:             toRuntimeAuth(deployment.Auth),
		AcceptedStatuses: deployment.AcceptedStatuses,
		VersionField:     deployment.Parser.VersionField,
		Mimir: source.RuntimeMimir{
			Endpoint:     deployment.Mimir.Endpoint,
			Auth:         toRuntimeAuth(deployment.Mimir.Auth),
			Query:        deployment.Mimir.Query,
			VersionLabel: deployment.Mimir.VersionLabel,
		},
	}
}

func toRuntimeAuth(auth AuthConfig) source.RuntimeAuth {
	return source.RuntimeAuth{
		Type: auth.Type,
	}
}
