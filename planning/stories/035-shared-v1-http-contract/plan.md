# 035 — Shared v1 HTTP contract

**Depends on:** 020, 030 · **Status:** Not started

## Goal

Give every translation endpoint the shared HTTP behaviour recorded from Alertmanager 0.26.0.

## Scope

- Shared success/error envelope helpers, Content-Type, cache and CORS headers on successful and
  failing responses, including 0.26's recorded plain-text exceptions.
- Method-aware dispatch for the known v1 routes; unknown routes return the recorded 404 and wrong
  methods the recorded 405. `/api/v1/alerts/groups` stays unknown.
- OPTIONS/preflight handling reproduces 0.26 status, empty body and headers, independent of endpoint
  implementation. Keep 501 placeholders only for known, not-yet-translated operations.
- Shared context-aware v2 transport/error helpers preserve required auth, enforce existing timeouts,
  report upstream failures truthfully in the chosen v1 error contract, and emit 030 telemetry.
- Move pending entries for OPTIONS and unknown routes out when they pass. Endpoint stories own
  their specific validation/error mappings and remove their own pending entries.
- Check non-root path requirements against 025 when available; do not change non-v1 pass-through.

## Explicit non-goals

- Alert/silence/status payload translation, replication, or introducing new authentication policy.

## Key files

`internal/v1/router.go`, `internal/v1/envelope/`, shared upstream helpers, `test/contract/pending.txt`.
