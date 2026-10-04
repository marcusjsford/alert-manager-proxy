# 080 — Partial-fanout detection

**Depends on:** 040, 075 · **Status:** Not started

## Goal

Surface clients that post alerts to fewer than all four Alertmanagers. Alertmanager gossips silences
and the notification log, **not alerts**, so these clients lose alerts when the one node they hit is
down. Today nothing notices. See [ADR decision 5](../../ADR.md).

## Scope

- No proxy can see the whole picture alone, so detection lives in the telemetry backend:
  `am_proxy_alerts_received_total{client, instance}` (from 040) across all four proxies; per client,
  count distinct `instance` values with posts in a window (≥ 2× the longest client evaluation /
  resend interval — choose and justify).
- Config: expected peer count (4) per environment.
- Deliverables: a ClickHouse query and a Prometheus-style alert rule that both list
  `client → instances reached / expected`.
- Cover v2 pass-through alert posts too (`POST /api/v2/alerts`, counted by 075) — after v1 retires,
  this is the only traffic, and new clients can be misconfigured too.
- `unknown` clients: detect per `client.address` from spans/logs instead, since the metric
  aggregates them into one label value.

## Explicit non-goals

- Changing any traffic (090).

## Key files

`deploy/queries/partial_fanout.sql`, `deploy/rules/partial_fanout.yml`, docs.
