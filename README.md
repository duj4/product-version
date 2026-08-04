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
- `APP_KERBEROS_DIR`: directory containing the optional runtime keytabs
  `itsm_version_qa.keytab` and `itsm_version_prod.keytab`; defaults to
  `/d/d1/product-version/kerberos`.
- `APP_KRB5_CONFIG`: Kerberos configuration file; defaults to
  `/etc/krb5.conf`.
- `APP_PORT`: HTTPS listen port; defaults to `8443`.

The service has no PostgreSQL dependency.

Kerberos files are resolved but not opened during startup. They are loaded only
when a runtime deployment with `auth.type: kerberos` is collected. A missing
keytab or Kerberos configuration therefore makes only that runtime source
unavailable and does not prevent the service from starting. The deployment
`env` selects the identity automatically: QA uses `itsm_version_qa`, while
Prod uses `itsm_version_prod`. A QA service never loads the Prod keytab.
Keytab files should be readable only by the service account (for example, mode
`0600`).

The Kerberos loader recursively expands read-only `include` and `includedir`
directives, including system drop-ins under `/etc/krb5.conf.d`. If
`default_realm` is absent, the client derives its Realm from the selected
keytab. When no KDC is listed, DNS SRV discovery is used as a fallback.

Version responses remain fresh in memory for 30 seconds. The cache is warmed
asynchronously during startup. After expiry, the first request starts a refresh
and waits for it; concurrent requests join that same collection rather than
multiplying downstream traffic. If the collection times out, the last cached
response is returned as a fallback. The web page's Refresh button calls
`/api/versions?refresh=true`, which also refreshes a still-valid aggregate cache
while joining any collection already in progress.

Successful endoflife.date responses have a separate six-hour in-memory cache.
API responses include `collected_at`, which the web page displays as data time.
A fallback keeps the timestamp of the last successfully completed collection.

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

For a Jira endpoint protected by Kerberos/SPNEGO, configure a normal HTTP
deployment with `auth.type: kerberos`. The response field in Jira's
`/rest/api/latest/serverInfo` payload is `version`:

```yaml
runtime:
  deployments:
    - env: qa
      enabled: true
      type: http
      endpoint: https://jira-qa.example.com/rest/api/latest/serverInfo
      method: GET
      auth:
        type: kerberos
      accepted_statuses:
        - 200
      parser:
        version_field: version
```

Do not add an `Authorization` header or credentials to `products.yaml`. The
runtime client performs the SPNEGO challenge automatically and caches Kerberos
tickets in memory. A production Jira deployment uses the same structure with
`env: prod`; its environment selects the Prod principal and keytab.

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
- `internal/versions/source`: CMDB, runtime HTTP/Mimir/Kerberos, and endoflife.date source adapters.
- `internal/versions/model`: API response model.
- `internal/httpclient`: shared plain HTTP and mTLS client construction.
- `config`: runtime configuration, including `products.yaml`, `cmdb.json`, and `proxy`.

## Build

```shell
go build -mod=vendor ./cmd/server
```
