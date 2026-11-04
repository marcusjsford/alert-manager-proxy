# 075 — Findings

_Append-only. Log discoveries, wrong assumptions, decisions made under ambiguity (and why), and
deviations from the plan **as they happen**, not reconstructed afterwards._

No findings recorded yet — story not started.

## 2026-10-29 — MVP replan

Deep v2 request inspection follows 074. Basic request-level telemetry remains in 030 and is
required for initial cutover; equal long-term v1/v2 attribute depth remains the goal here.

## 2026-10-29 — Re-scope and implementation

074 is deferred until feature completion. 075 therefore depends only on 030 and runs before 074,
with 080/090/100/105 and finally 074 in the revised order. 090 is implemented/tested pre-cutover but
enabled only after 080/100 evidence.

- `internal/inspect` parses bounded v2 alert/silence requests and the small silence responses that
  carry an ID or matchers. The original request and response bytes are replayed unchanged; a
  1 MiB cap and malformed JSON mark inspection skipped without affecting the request.
- Response inspection runs under the upstream client span, so the inbound server span is carried
  explicitly through the request context; response attributes land on the server span and obey the
  same redaction rules.
- v1 POST alerts now records `am.alerts.firing`/`am.alerts.resolved` too, keeping application span
  keys aligned with v2. v2 GET alert flags and filters reuse the v1 attribute names; v2 silence list
  keeps only the v1-compatible filter and operation keys. Automatic otelhttp body-size attributes are
  excluded from the parity assertion because v1 and v2 wire bodies intentionally differ.
- `am_proxy_alerts_received_total` is incremented for v2 posts with `api_version="v2"`; malformed and
  oversized bodies increment `am_proxy_inspection_skipped_total` with a span-only reason.
- Added `TestV2InspectionPreservesRequestsAndResponses`, `TestV2InspectionSpanKeyParityWithV1`,
  `TestV2InspectionPreservesBodiesAndCountsSkipped`, `internal/inspect` unit tests, and a focused v2
  POST-alert latency measurement.
- Telemetry measurement: v2 POST /alerts p99 off 5.146 ms, on 9.656 ms, added 4.510 ms (1000
  interleaved samples after 100 warmups on the development host). The existing GET latency sample was
  p99 5.088 ms off / 8.451 ms on (added 3.363 ms).
- Verification: `go test -race ./...`, all-tag vet/lint (0 issues), full `make ci` (both versions,
  strict contracts and both integrations) and `make telemetry-check` pass.
- The telemetry stack was rebuilt from this worktree; recreate it from main after merge before
  removing the worktree.
