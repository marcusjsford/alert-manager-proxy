# 040 — `POST /api/v1/alerts`

**Depends on:** 020, 030 · **Status:** Not started

## Goal

Translate the highest-volume legacy call — old Prometheus servers pushing alerts — to
`POST /api/v2/alerts`, with v1-identical responses.

## Scope

- Shared v1 envelope package (`{"status":"success","data":…}` / error shape and `errorType` values as
  recorded in 020's fixtures) — reused by 050–070.
- Decode v1 alert list, validate as v0.26 did, map to v2 `PostableAlerts`, translate v2 errors back
  to v1 status codes and envelopes.
- Telemetry: `am.alerts.count`, alertnames (span only, bounded list), increment
  `am_proxy_alerts_received_total{client, instance, api_version}` — 080 depends on this metric, and
  075 emits the same metric for v2.

## Explicit non-goals

- Forwarding to other Alertmanagers (090).

## Key files

`internal/v1/envelope/`, `internal/v1/alerts_post.go`.
