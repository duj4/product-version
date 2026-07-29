# Product Versions

Product Versions combines registered CMDB versions, detected runtime versions,
and release lifecycle data from endoflife.date. A single production deployment
collects both QA and Prod runtime versions.

## Routes

- `GET /versions`
- `GET /api/versions`
- `GET /healthz`

## Configuration

The service uses these environment variables:

- `APP_ENV`: controls the service runtime mode; defaults to `prod`.
- `APP_CONFIG_DIR`: directory containing `cmdb.json` and `products.yaml`;
  defaults to `/d/d1/product-versions/config`.
- `APP_TLS_DIR`: directory containing `tls.pem`, `tls.key`,
  `itsm_jsm_qa.pem`/`.key`, and `itsm_jsm_prod.pem`/`.key`; defaults to
  `/d/d1/product-versions/tls`.
- `APP_LISTEN_ADDR`: HTTPS listen address; defaults to `:8443`.

The service has no PostgreSQL dependency.

CMDB is always queried with the Prod client certificate. Runtime HTTP and Mimir
requests select the QA or Prod client certificate from their deployment env.

## Product configuration

Product keys remain globally unique and do not contain an environment suffix.
Environment-specific configuration exists only below the runtime source:

```yaml
products:
  - key: example
    metadata:
      display_name: Example Product
      application_type: generic

    cmdb:
      enabled: true
      name: Example.Product

    runtime:
      deployments:
        - env: qa
          enabled: true
          type: http
          endpoint: https://example-qa.internal/version
          method: GET
          auth:
            type: mtls
          parser:
            version_field: version

        - env: prod
          enabled: true
          type: http
          endpoint: https://example-prod.internal/version
          method: GET
          auth:
            type: mtls
          parser:
            version_field: version

    eol:
      enabled: true
      product: example
      cycle_strategy: major_minor
      prefer_lts: false
```

Every product must define exactly one `qa` and one `prod` runtime deployment.
A deployment may be present with `enabled: false` while its endpoint is being
onboarded.

The API response is organized as `key + metadata + sources`. CMDB and EOL are
product-level sources; runtime results are returned as
`sources.runtime.deployments`.

## Build

```shell
go build -mod=vendor ./cmd/server
```
