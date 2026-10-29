# Telemetry and client identity

Story 030 adds opt-in OTLP **traces, metrics and logs** without changing API bodies, query strings
or response headers. Telemetry is disabled by default, so a collector is not required to start or
use the proxy. The upstream still receives Authorization and the original payload; those credentials
and payloads are not copied into telemetry.

## Configuration

```yaml
upstream:
  url: http://alertmanager-0:9093
telemetry:
  enabled: true
  service_name: alertmanager-proxy
  instance_id: alertmanager-0
  environment: development
  protocol: grpc
  endpoint: http://collector:4317
  export_timeout: 2s
  batch_interval: 1s
  metric_interval: 15s
clients:
  trusted_proxies: []
  rules:
    - name: legacy-prometheus
      user_agent: '^Prometheus/'
    - name: example-network
      cidrs: [192.0.2.0/24]
    - name: dashboard
      basic_auth_user: grafana
    - name: automation
      header: X-Client-Name
      header_value: automation
redaction:
  headers: [Authorization, X-API-Key]
  silence_comment: true
  silence_created_by: true
```

Start with `alertmanager-proxy -config path/to/config.yml`. The sample identities/CIDRs are examples,
not discovered production clients; see `planning/client-inventory.md` for remaining estate evidence.

Rules are evaluated in order: **all populated conditions in a rule must match**, with any CIDR in
that rule sufficient for its CIDR condition. A rule's configured name, never a header/IP/username
copied from the request, becomes `client.name`. Unmatched callers become `unknown`.

The immediate peer must belong to `trusted_proxies` before X-Forwarded-For is considered. Repeated
headers are combined, and the chain is walked from right to left until the first untrusted hop.
Malformed or oversized chains fall back to the immediate peer. The default trusts no forwarders.

When enabled, `instance_id` is required: use a bounded non-IP name identifying the **fronted
Alertmanager**. Each of the three production proxies must have its own corresponding instance ID.
Resources carry `service.name`, `service.version` (the proxy build version), `service.instance.id`
and `deployment.environment.name`.

| Environment override | Default |
|---|---|
| `AMP_TELEMETRY_ENABLED` | `false` |
| `AMP_SERVICE_NAME` | `alertmanager-proxy` |
| `AMP_SERVICE_INSTANCE_ID` | Required when enabled |
| `AMP_ENVIRONMENT` | `development` |
| `AMP_OTLP_PROTOCOL` | `grpc`; also accepts `http/protobuf` |
| `AMP_OTLP_ENDPOINT` | `http://localhost:4317` |
| `AMP_OTLP_EXPORT_TIMEOUT` | `2s` |
| `AMP_OTLP_BATCH_INTERVAL` | `1s` |
| `AMP_OTLP_METRIC_INTERVAL` | `15s` |

Set both protocol and endpoint when switching to HTTP, e.g. `http/protobuf` and
`http://collector:4318`. HTTP appends `/v1/traces`, `/v1/metrics`, `/v1/logs` to the configured base
URL. gRPC does not accept a path. The URL scheme selects plaintext HTTP or HTTPS/TLS. Endpoints
with embedded credentials, query strings or fragments are rejected; the explicitly configured
endpoint takes precedence over SDK endpoint environment defaults.

## Signals and redaction

- Server and child upstream spans share a trace; outbound `traceparent`/`tracestate` are propagated.
- One JSON/OTLP access log per request records client identity, protocol, route template, method,
  result and elapsed time. Request-scoped proxy warnings use the same context. JSON trace/span IDs
  match the native OTLP log IDs. Startup/exporter diagnostics are background logs, not request logs.
- Raw `client.address` and `user_agent.original` are span/log attributes only. `User-Agent` can be
  explicitly redacted with the header policy.
- Authorization/Proxy-Authorization fields are always omitted, even if the configurable list is
  replaced. Other configured sensitive headers and optional silence comment/createdBy fields are
  filtered recursively from parameter attributes. Request bodies/headers are not automatically
  captured; request parameters are recorded only through the helper API.
- Automatic URL attributes lose credentials, query and fragment. Raw exception messages/stacks
  are replaced by a generic failure description; HTTP result/error type remain available. Startup
  logging uses only the upstream host, not a credential-bearing configured URL.
- Only configured names, finite route templates, known methods (`_OTHER` for custom methods),
  status codes and instance IDs can become metric labels. Unknown paths use wildcard templates;
  silence IDs are replaced by `{id}`. The only allowed label keys are `client`, `api_version`,
  `endpoint`, `method`, `status_code`, `instance`. HTTP library auto-metrics are disabled.

| Instrument | Meaning |
|---|---|
| `am_proxy_requests_total` | Client-facing requests, including v1 placeholder/error responses |
| `am_proxy_request_duration_seconds` | Full handler time |
| `am_proxy_upstream_duration_seconds` | Upstream operation through response-body completion/close |
| `am_proxy_translation_lossy_total` | Explicit translation decisions reported by the helper |
| `am_proxy_alerts_received_total` | Parsed received alert counts reported by endpoint/inspection code |

`status_code=0` means no HTTP response was observed (e.g. a transport failure or pre-response
abort). Aborted requests still produce correlated logs and error spans without changing the
original abort behaviour. Protocol upgrades retain bidirectional streaming, and actual 101 status
is reflected even when the reverse proxy writes upgrade headers directly to a hijacked connection.

Traces and logs use bounded, non-blocking 2048-entry SDK queues. Export attempts have a timeout,
retries are disabled, and collector failures never become request errors. Queue pressure can drop
telemetry; a failed batch emits a JSON `telemetry export failed` diagnostic. Service shutdown shares
one deadline between HTTP draining and all three concurrently shutting-down providers.

### Endpoint-story helpers

`Runtime.Parameters(ctx, attrs...)` attaches redacted details to the request span and access log,
never labels. `Runtime.SpanParameters(ctx, attrs...)` attaches redacted detail to the span only, for
bounded request content that would make logs noisy. `Runtime.Lossy(ctx, field, reason)` adds
`am.translation.lossy` and increments the lossy counter; field/reason are span-only.
`Runtime.Alerts(ctx, count)` adds `am.alerts.count` and increments the received-alert counter.
`POST /api/v1/alerts` also attaches the deduplicated, bounded `am.alerts.names` list to the span
only and records a `alert.timestamps` lossy event when sub-millisecond precision is truncated.
Silence operations attach `am.silence.operation`, `am.silence.id` and canonical matchers; create and
update also attach `am.silence.createdBy`/`am.silence.comment`, which the redaction configuration
removes when enabled. List/get record the `silence.timestamps` lossy event for v2's millisecond
precision. Field/reason names should describe the implementation's
translation decision, not embed arbitrary request content.

030 does not inspect v2 JSON payloads or implement v1 translation. 040–070 and 075 call these helpers
as their endpoint work lands. The alert/lossy counters have no observations until called.

## Real development pipeline

```sh
make telemetry        # owned collector, ClickHouse, AM, gRPC/HTTP/off proxy variants
make telemetry-check  # persistence/correlation/redaction checks and a p99 on/off sample
make telemetry-down   # remove only amp-telemetry, including its test database
```

The test stack uses Collector Contrib 0.145.0 and ClickHouse 26.5. Its proxy variants share a test
upstream to compare protocols/overhead; it is not the production topology. They use loopback ports:
gRPC proxy 19595, HTTP proxy 19596, disabled baseline 19597; collector 19517/19518, collector health
19533 and ClickHouse HTTP 19523. ClickHouse's fixture credentials are `amp` / `amp-lab-only`.

This project-owned pipeline avoids modifying the shared development collector, whose OpAMP
configuration was unavailable during this story. Real traces/logs/metrics are inserted into the
`otel` database and queried by the checks. The persistent `amp-lab` three-instance environment
remains separate. Production collector connectivity/configuration is validated during cutover.

`make ci` continues to run unit/race/redaction/identity/outage tests plus existing integration and
contract checks. The heavier real pipeline and latency sample are explicit development checks,
not automatically started by CI. The latency sample interleaves 1000 warm-connection requests per
mode after 100 warmups per mode, at concurrency 1; results are host/workload-specific.
The reported added p99 is the difference between the enabled and disabled end-to-end p99 values.

Explicit process-outage check (in the development stack only): stop `collector`, run
`go test -race -tags telemetry,outage -run '^TestCollectorProcessOutage$' -count=1 -v ./test/telemetry/`,
then restart the collector. Story findings also record a real proxy shutdown while export was down.

Operational SQL is in `deploy/queries/request_visibility.sql`. Cumulative metric points must not
be summed across export intervals as if they were per-interval counts; the queries retain stream
start time and use latest values. Trace-based views show request/error/latency and last-seen usage.
Views retain the deployment environment; filter to the intended environment for operational or
retirement decisions. Errors include 4xx/5xx responses and explicitly observed request aborts.
