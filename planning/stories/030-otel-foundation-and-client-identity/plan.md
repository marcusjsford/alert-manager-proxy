# 030 — OTel foundation + client identity

**Depends on:** 010 · **Status:** Not started

## Goal

Know exactly which client calls which endpoint on which protocol version with which parameters —
across traces, metrics and logs — per [ADR decision 4](../../ADR.md#decision-4--telemetry-model).

## Scope

- OTel SDK setup: OTLP exporter (gRPC/HTTP configurable), resource with `service.name`,
  `service.version`, `service.instance.id` (= the fronted Alertmanager's identity).
- `otelhttp` server middleware on all routes (v1, pass-through, v2); `otelhttp` client transport on
  the upstream, propagating `traceparent`.
- Client identification: config rules matching source CIDR (honouring `X-Forwarded-For` only from
  configured trusted proxies), `User-Agent` regex, basic-auth username, optional header →
  `client.name`; fallback `unknown`. Span/log attributes: `client.name`, `client.address`,
  `user_agent.original`, `am.api.version` (`v1`/`v2`/`other`), `http.route` (template).
- Helper API for translation stories to attach parameters (`am.v1.filter`, `am.alerts.count`,
  `am.silence.id`, …) and record lossy-translation events (`am.translation.lossy` event + counter).
- Metrics: `am_proxy_requests_total`, `am_proxy_request_duration_seconds`,
  `am_proxy_upstream_duration_seconds`, `am_proxy_translation_lossy_total`,
  `am_proxy_alerts_received_total`; labels limited per ADR.
- Logs: `log/slog` JSON + `otelslog` bridge; one access log per request with trace/span IDs.
- Redaction config for headers (`Authorization`) and optionally silence `comment`/`createdBy`.

## Explicit non-goals

- Dashboards (100). Partial-fanout logic (080).

## Key files

`internal/telemetry/`, `internal/clientid/`, `internal/middleware/`.
