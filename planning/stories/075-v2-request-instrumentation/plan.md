# 075 — v2 request-level instrumentation

**Depends on:** 030, 074 · **Status:** Not started

## Goal

Give v2 pass-through calls the same telemetry depth as translated v1 calls
([ADR decision 4](../../ADR.md#decision-4--telemetry-model), [decision 6](../../ADR.md#decision-6--the-proxy-is-permanent-the-v1-layer-is-what-retires)).
After v1 retires, v2 is all the proxy sees — this story is what makes the proxy worth keeping.
This deep inspection is post-MVP; 030's request-level visibility already supports initial cutover.

## Scope

- Read-only inspection of v2 requests on the pass-through path: buffer the body (with a size cap),
  extract attributes, and forward the **original bytes unchanged**.
  - `POST /api/v2/alerts`: alert count, alertnames (bounded), resolved vs firing count;
    `am_proxy_alerts_received_total{client, instance, api_version="v2"}`.
  - `GET /api/v2/alerts`, `/alerts/groups`, `/silences`: `filter` params and flags.
  - `POST /api/v2/silences`, `DELETE /api/v2/silence/{id}`: operation, ID, matchers, `createdBy`
    (030's redaction rules apply).
- Reuse the attribute names and helpers settled by 040–070 so a v1 call and its v2 equivalent are
  queryable with the same attributes, differing only in `am.api.version`.
- Bodies that fail to parse or exceed the cap are still forwarded untouched; a counter records that
  inspection was skipped.

## Explicit non-goals

- Validating or rewriting v2 traffic — the proxy observes v2, never changes it.

## Key files

`internal/inspect/`, `internal/passthrough/`.
