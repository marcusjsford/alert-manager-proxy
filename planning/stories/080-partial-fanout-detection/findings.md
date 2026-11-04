# 080 — Findings

_Append-only. Log discoveries, wrong assumptions, decisions made under ambiguity (and why), and
deviations from the plan **as they happen**, not reconstructed afterwards._

No findings recorded yet — story not started.

## 2026-10-29 — Three-instance topology confirmed in 025

Operator corrected the four-instance assumption to three. Updated configurable expected coverage
and synthetic 3/2/1 fanout cases; real telemetry follows 074 deployment, without requiring 110.

## 2026-10-29 — Implementation and verification

- `deploy/queries/partial_fanout.sql` uses the 040/075 alert counter for named clients and server
  spans for `unknown` clients grouped by `client.address`. It covers v1 and v2 routes, uses a
  10-minute window (much larger than normal evaluation/resend intervals), and joins a coverage CTE
  requiring all expected proxy instances to be reporting request telemetry.
- Expected counts are configured per environment in the SQL CTE (3 for `production` and
  `fanout-test`). `deploy/rules/partial_fanout.yml` exposes the intended exporter metrics
  `am_proxy_client_instances_reached`/`_expected` and suppresses while any
  `up{job="alertmanager-proxy"}` is down. `promtool check rules` passes.
- `test/fanout` adds a dedicated three-AM/three-proxy Collector/ClickHouse stack and a Go check. It
  posts as clients `fanout-full` (3 proxies), `fanout-two` (2, one via v2), `fanout-one` (1) and an
  unmatched user agent that becomes `unknown` and is detected by span address. Actual query output:
  `fanout-one 1/3`, `fanout-two 2/3`, `unknown 172.26.0.1 1/3`; `fanout-full` is absent.
- The query intentionally returns no partial rows when proxy coverage is below the expected count.
  A proxy outage can leave stale metrics in the SQL window, so the Prometheus rule's `up` guard
  suppresses the alert immediately; a separate proxy-availability alert covers the outage. This is
  documented in `docs/partial-fanout.md`.
- Real production telemetry is unavailable because 074 has not run (operator feature-first ordering).
  The production run/real partial-client evidence is explicitly deferred to post-074 validation and
  added to 074's acceptance criteria; synthetic success is not presented as production evidence.
- Verification: `make fanout` + `make fanout-check` passes; `promtool check rules` passes; SQL query
  executed against ClickHouse and returned exactly the three expected rows; `go test -race ./...`,
  all-tag vet/lint and full `make ci` pass.
