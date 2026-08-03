# Product Version

Product Version combines registered CMDB versions, detected runtime versions,
and release lifecycle data from endoflife.date. A single production deployment
collects both QA and Prod runtime versions.

## Routes

- `GET /`: web UI
- `GET /versions`: redirects to `/` for backward compatibility
- `GET /api/versions`
- `GET /healthz`

## Configuration

The service uses these environment variables:

- `APP_ENV`: controls the service runtime mode; defaults to `prod`.
- `APP_CONFIG_DIR`: directory containing `cmdb.json` and `products.yaml`;
  defaults to `/d/d1/product-version/config`.
- `APP_TLS_DIR`: directory containing `tls.pem`, `tls.key`, and the outbound
  client certificates permitted for the service environment. QA requires
  `itsm_jsm_qa.pem`/`.key`; Prod additionally requires
  `itsm_jsm_prod.pem`/`.key`. It defaults to `/d/d1/product-version/tls`.
- `APP_PORT`: HTTPS listen port; defaults to `8443`.

The service has no PostgreSQL dependency.

Version responses remain fresh in memory for 30 seconds. The cache is warmed
asynchronously during startup. After expiry, a normal page load returns the last
response immediately and starts one shared background refresh, so concurrent
users do not multiply downstream requests or wait for them. The web page's
Refresh button calls `/api/versions?refresh=true`; it waits for fresh CMDB and
runtime results while joining any collection already in progress.

Successful endoflife.date responses have a separate six-hour in-memory cache.
API responses include `collected_at`, which the web page displays as data time.

With `APP_ENV=qa`, CMDB uses the QA URL and client certificate, and Prod runtime
deployments are skipped before any outbound request. With `APP_ENV=prod`, CMDB
uses the Prod URL and client certificate, while both QA and Prod runtime
deployments are collected. Permitted mTLS runtime deployments select the client
certificate from each deployment's `env`.

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

Every product must define at least one runtime deployment. `qa` and `prod` are
both optional, but the same environment cannot appear more than once for a
product. A deployment may be present with `enabled: false` while its endpoint
is being onboarded.

The API response is organized as `key + metadata + sources`. CMDB and EOL are
product-level sources; runtime results are returned as
`sources.runtime.deployments`.

The EOL source exposes its `prefer_lts` policy and the complete release-cycle
catalog. Each cycle includes its initial release date, latest patch and patch
release date, maintenance state, and EOL date when available. The web release
context highlights the policy-selected latest cycle, maintained LTS cycles, and
cycles currently used by QA or Prod.

## Project layout

- `cmd/server`: process entry point.
- `internal/server`: HTTPS startup, TLS paths, middleware, routes, and embedded web assets.
- `internal/cmdb`: low-level CMDB configuration and API client.
- `internal/versions`: product configuration, validation, caching, concurrency, and aggregation.
- `internal/versions/source`: CMDB, runtime/Mimir, and endoflife.date source adapters.
- `internal/versions/model`: API response model.
- `internal/httpclient`: shared plain HTTP and mTLS client construction.
- `config`: runtime configuration, including `products.yaml`, `cmdb.json`, and `proxy`.

## Build

```shell
go build -mod=vendor ./cmd/server
```
