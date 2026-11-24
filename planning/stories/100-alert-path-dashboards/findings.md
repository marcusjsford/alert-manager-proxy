# 100 — Findings

_Append-only. Log discoveries, wrong assumptions, decisions made under ambiguity (and why), and
deviations from the plan **as they happen**, not reconstructed afterwards._

No findings recorded yet — story not started.

## 2026-10-29 — Implementation and verification

- Confirmed the repository pipeline uses ClickHouse `otel` and does not provision Grafana. Chose
  Grafana 11.3 + `grafana-clickhouse-datasource` with a provisioned datasource UID `clickhouse`;
  imported `deploy/dashboards/alert-path.json` into a fresh Grafana instance (HTTP 200) and executed
  a real dashboard panel query through Grafana (HTTP 200).
- The dashboard has 12 panels: per-client alert rate, firing/resolved, top alertnames, last seen,
  client error rate, silence operations, observed create-to-delete silence lifetime, embedded 080
  partial fanout, proxy server/upstream p99, proxy errors/lossy, inspection skipped and v1 last-seen
  by client/endpoint/filter. It takes an `environment` textbox and uses ClickHouse `$__timeFilter`.
- `test/dashboards` seeds v1/v2 alerts, errors, silences and oversized inspection data in the
  three-proxy fanout stack, then executes every panel's `rawSql`. Actual row counts:
  alert posts 10, firing/resolved 8, top alertnames 12, last seen 5, client errors 4, silence
  operations 8, silence lifetime 6, fanout 3, latency 12, proxy health 6, skipped 1, v1 last-seen 7.
- The v1 last-seen panel summed 36 requests for the prior day and an independent raw server-span
  count over the same day was also 36.
- `deploy/rules/alert_path.yml` defines silent-client (last-seen age > 2× per-client expected
  interval, `for: 5m`, suppressed when a proxy `up` is missing/down), new-unknown-client and
  new-v1-client rules. The exporter age-gauge contract is documented in
  `docs/alert-path-dashboards.md`. `deploy/rules/alert_path_test.yml` proves the silent client fires
  while a client with a 10-minute interval does not; `promtool test rules` passes.
- No new telemetry signal was needed. Silence lifetime is observed create-to-delete time; scheduled
  expiry remains a limitation/follow-up if needed. Production traffic validation remains post-074.
- Verification: `make dashboards-check`, `make rules-check`, `go test -race ./...`, all-tag vet/lint,
  full `make ci`, Grafana import/query and Prometheus rule tests all pass.
