# 030 — Findings

_Append-only. Log discoveries, wrong assumptions, decisions made under ambiguity (and why), and
deviations from the plan **as they happen**, not reconstructed afterwards._

No findings recorded yet — story not started.

## 2026-10-29 — MVP replan

015 is now the prerequisite. Basic request/client/error/latency visibility supports 074 without
waiting for full dashboards. Collector-unavailable forwarding is an explicit acceptance criterion.

## 2026-10-29 — Started from merged 025

Created this story's clean branch/worktree from main at `89c76c4`. The three-instance Docker lab
is running from main; no production access is needed for development. Read the plan, criteria,
ADR telemetry model and existing configuration/server/transport before implementing.

The telemetry foundation must not emit the HTTP library's automatic metric dimensions, since the
ADR permits only six bounded labels. Own request/upstream instruments will use that allowlist;
otelhttp tracing will use a no-op meter provider. Request headers/bodies and raw query strings
must not be implicitly captured, and SDK-exported URL/error attributes need redaction tests.

## 2026-10-29 — Version/API ground truth and development pipeline

Queried module metadata rather than assuming SDK versions: current Go OTel APIs/SDKs are 1.47.0,
otelhttp is 0.72.0, otelslog is 0.21.0, and OTLP log exporters remain 0.23.0 despite the stable
1.47.0 log SDK. The initial attempt to use 1.47.0 log exporters failed because those modules do
not exist at that version; pinned their actual published compatible versions. Go stays at 1.26.2.

The existing shared development collector's supervisor repeatedly reports missing app:4320 OpAMP
DNS/server configuration. Its Docker network contains only that collector. Did not modify it or
claim existing/nuc01 pipeline connectivity. Following the operator's development-machine/Docker
direction from 025, added an owned amp-telemetry stack: Collector Contrib 0.145.0, ClickHouse
26.5-alpine, AM 0.34.1 and gRPC/HTTP/disabled proxy variants. The real development pipeline is the
end-to-end acceptance evidence; production endpoint configuration is still 074 work. The first
collector image pull returned registry HTTP 500; a retry succeeded with the pinned image.

## 2026-10-29 — Implementation choices

- Providers belong to each Runtime, never global SDK state. Export is opt-in; enabled proxies
  require a bounded non-IP instance ID identifying their Alertmanager. Client names are likewise
  bounded non-IP configured identifiers; matching never turns arbitrary request values into labels.
- Identity rules are ordered, AND their configured predicates, and support CIDR/UA/basic-auth user/
  exact optional header. Forwarding trust is checked at the immediate peer; all repeated XFF lines
  form one chain, walked right-to-left. Malformed/oversized chains fall back to the immediate peer.
- Custom instruments have only the six ADR keys. HTTP instrumentation gets a no-op meter provider;
  an SDK view also filters label keys. Fixed route/method/code sets plus configured names bound
  values. Disable the SDK cardinality-overflow label so it cannot add an unapproved dimension.
- HTTP middleware lives with its Runtime in internal/telemetry/http.go rather than introducing
  cyclic/public-state plumbing through a separate middleware package. Future endpoint helpers add
  redacted parameters to spans/access logs, received counts, and lossy events/counters.
- Authorization/Proxy-Authorization are always omitted from telemetry. Parameter maps are filtered
  recursively, silence text can be suppressed/configured, URL credentials/query/fragment are
  removed at export, and raw exception text/stacks are replaced by a generic failure message.
  Startup uses only the upstream host; invalid URL errors do not echo credential-bearing input.
  The original Authorization, query and bytes still reach the upstream, verified by a test spy.
- JSON and OTLP request logs share contextual trace/span IDs. Export failures use a separate JSON
  diagnostic path to avoid recursive export of exporter errors. Background diagnostics are not
  misrepresented as request logs.
- Trace/log SDK queues are non-blocking and bounded at 2048 records; export attempts time out,
  retries are disabled, and metric export is periodic. All signal shutdowns run concurrently under
  the same deadline used by HTTP draining. An initial cancellation-closure form triggered go vet's
  lostcancel check; changed to explicit normal-path cancellation plus early-return cleanup.

## 2026-10-29 — Transparency and regression details

Code review against otelhttp's actual response-body wrapper found that a plain io.ReadCloser
wrapper would erase io.ReadWriteCloser after a 101 upgrade. Preserve that interface and keep
duration measurement alive until close for duplex responses. A real TCP upgrade/echo test now
verifies bidirectional traffic. Hijacked headers bypass the server ResponseWriter, so preserve the
actual upstream 101 in request metrics/logs and normalise the exported server status accordingly.

Also capture metrics/access logs when a handler aborts, then re-panic the same sentinel. The abort
regression intentionally compares exact http.ErrAbortHandler identity (a scoped errorlint exception
explains that wrapping it changes net/http behaviour). Other lint findings were ordinary selector/
switch simplifications and were fixed. Aborts without an observed response use status_code=0.

## 2026-10-29 — Signal, privacy and cardinality evidence

Race-enabled unit tests exercise both real OTLP gRPC and protobuf/HTTP wire formats with captured
server/client spans, metrics and logs. CIDR, UA, basic-auth, exact-header and unknown clients all
have the expected name on all three signals. Each child shares its server trace and references
the server span; the upstream spy sees that child in traceparent. JSON log IDs match the native
OTLP IDs and server span, including request-scoped diagnostics.

Tests scan every exported signal and JSON output for plain/encoded Authorization secrets, query
secrets and redacted helper/log/silence values. Optional silence redaction can be disabled; a
configured UA/API-key policy is enforced. Twenty-five unique source IPs/UAs/IDs/filters/custom
methods produce one unknown-client/_OTHER/template metric series, with no unapproved label keys
or automatic HTTP instruments. Helpers' received counts and lossy events/counters are asserted.
Trusted/untrusted/malformed/repeated XFF chains and IPv4/IPv6 cases are covered. An enabled v1
placeholder is observed as a legacy-Prometheus v1 request/501 without forwarding upstream.

## 2026-10-29 — Actual Collector → ClickHouse query/results

Inspected the real exporter-created schema before writing queries; otel_traces has Server/Client
SpanKind values, otel_logs carries native TraceId/SpanId, and metric tables expose Attributes and
StartTimeUnix/TimeUnix. All three committed operational SQL queries execute successfully. Views
retain environment and counter stream start time, avoiding summed cumulative samples or mixed
development/production usage evidence.

Final make telemetry-check probes (fresh trace IDs; metric points must postdate the request):

| Protocol / instance | Trace ID | Parent-child pairs | Correlated access logs | New request metric points |
|---|---|---:|---:|---:|
| gRPC / am-grpc | ef434e5ec0618558096e04da4ef93852 | 1 | 1 | 1 |
| HTTP / am-http | ad3217984ac1eb1bedd8a334f5e65f6c | 1 | 1 | 1 |

Representative query (substitute either ID above):

```sql
SELECT count()
FROM otel.otel_traces AS c INNER JOIN otel.otel_traces AS p
  ON c.TraceId = p.TraceId AND c.ParentSpanId = p.SpanId
WHERE c.TraceId = 'ef434e5ec0618558096e04da4ef93852'
  AND c.ResourceAttributes['service.instance.id'] = 'am-grpc'
  AND c.SpanAttributes['client.name'] = 'lab-smoke'
  AND p.SpanAttributes['client.name'] = 'lab-smoke';
-- Result: 1

SELECT count()
FROM otel.otel_logs AS l INNER JOIN otel.otel_traces AS s
  ON l.TraceId = s.TraceId AND l.SpanId = s.SpanId
WHERE l.TraceId = 'ef434e5ec0618558096e04da4ef93852'
  AND l.Body = 'request completed'
  AND l.LogAttributes['trace_id'] = l.TraceId
  AND l.LogAttributes['span_id'] = l.SpanId;
-- Result: 1
```

The corresponding request sum query filters service.instance.id, client=lab-smoke,
endpoint=/api/v2/status, MetricName=am_proxy_requests_total, positive Value and TimeUnix >= request
start; result 1 for each protocol. Trace/log scans for PIPELINE_ secrets return 0; both sum and
histogram table scans for non-allowlisted dimensions/automatic metric names return 0.

## 2026-10-29 — Collector failure and shutdown evidence

An absent collector at binary/runtime startup does not gate upstream delivery. A race-enabled
500-request test with failing exporters delivers every request to a real HTTP upstream and proves
bounded flush/shutdown plus observable export warnings.

Stopped only amp-telemetry's real collector process and ran the explicit outage-tagged check:
100 real upstream 0.34.1 responses through both gRPC/HTTP proxy variants succeeded in 148.687ms.
While the collector was still stopped, stopped proxy-grpc with a 5s Docker stop allowance: it exited
normally (code 0, not OOM-killed) in 0.851s under its configured shared 3s shutdown budget. A shell
trap restarted the collector/proxy, and the real pipeline checks passed again after recovery.

## 2026-10-29 — Added latency measurement

Method: warm HTTP keep-alive connections, GET /api/v2/receivers through otherwise identical disabled
and gRPC-enabled proxy variants to the same real AM; 100 warmups then 1000 samples per mode,
interleaved, concurrency 1. Full responses are consumed. Enabled includes JSON access logs, three
asynchronous OTLP signals and the running ClickHouse pipeline. Report p99(on) minus p99(off),
computed from unrounded durations; this is not a production SLO or an isolated hardware benchmark.

Hardware/runtime: 13th Gen Intel Core i9-13900H, WSL2/Linux amd64 exposing 20 logical CPUs;
Go 1.26.2; shared development host running other containers.

| Sample | Disabled p99 | Enabled p99 | Difference |
|---|---:|---:|---:|
| Initial, with concurrent lint activity | 5.317ms | 8.211ms | 2.894ms |
| Final build, without concurrent agent lint/build during sampling | 1.693ms | 3.447ms | 1.753ms |

The second sample was justified by the concurrent load in the first and the completed streaming/
status fixes. The final pipeline suite passed in 5.292s. Numbers vary with host load; method and
both observed results are retained rather than claiming a universal overhead figure.

## 2026-10-29 — Required checks

Full local rtk proxy make ci passes: build, go vet, race tests, lint (0 issues), existing real-AM
integration, unchanged contract replay and Docker build. Additional telemetry/outage/lab-tagged
lint passes. Contract scenario, fixtures and pending list remain unchanged, with the expected
0/48 XFAIL baseline: this story adds observation, not v1 translation. New focused v1 observation,
offline-startup, protocol-upgrade and abort tests also pass with the race detector. Configuration,
local pipeline and operational query documentation accompany the implementation.

## 2026-10-29 — Acceptance re-verification and completion

Re-ran the full local CI after the last helper/test changes: all required checks pass, including
race-enabled telemetry tests and lint with 0 issues. The updated environment-aware operational
SQL also executes without error. Re-checked all nine criteria against the concrete evidence above
and marked the story Done on its branch; the next eligible story after merge is 035.

The owned telemetry stack remains available for review. Its configuration mounts originate in
this worktree; recreate it from main before removing the worktree after merge, as with the 025
lab. The separate three-instance amp-lab and shared collector were not reconfigured.
