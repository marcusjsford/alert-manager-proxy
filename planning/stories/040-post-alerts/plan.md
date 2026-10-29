# 040 — `POST /api/v1/alerts`

**Depends on:** 025, 035 · **Status:** Done

## Goal

Translate the highest-volume legacy call — old Prometheus servers pushing alerts — to
`POST /api/v2/alerts`, with v1-identical responses.

## Scope

- Reuse 035's shared v1 routing, headers, envelopes and upstream error helpers; this story owns
  alert-specific decoding/validation and error mapping.
- Decode v1 alert list, validate as v0.26 did, map to v2 `PostableAlerts`, translate v2 errors back
  to v1 status codes and envelopes.
- Telemetry: `am.alerts.count`, alertnames (span only, bounded list), increment
  `am_proxy_alerts_received_total{client, instance, api_version}` — 080 depends on this metric, and
  075 emits the same metric for v2.
- Inputs recorded in [020's findings](../020-v1-contract-capture-and-harness/findings.md): v0.26.0
  validation error strings, v2's different error body shapes, `notjson` vs object-not-array errors.
- Use 025's pinned representative v1 sender for reproducible development tests, and the oldest
  estate sender when identified. Actual estate compatibility is a mandatory 072/074 release gate.
  Remove this endpoint from pending.txt only when all its fixtures pass. This is the sender
  milestone, not the complete v1-client compatibility release.

## Explicit non-goals

- Forwarding to other Alertmanagers (090).

## Key files

`internal/v1/alerts_post.go`, legacy-sender/test-receiver integration.
