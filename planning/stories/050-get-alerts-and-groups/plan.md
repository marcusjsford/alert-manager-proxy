# 050 — `GET /api/v1/alerts` + `GET /api/v1/alerts/groups`

**Depends on:** 040 · **Status:** Not started

## Goal

Serve v1 alert queries from v2, reshaped exactly as 0.26 returned them.

## Scope

- Query params: v1 `filter` (a single `{a="b",c=~"d"}` matcher-set string) → v2 repeated `filter`
  params, parsed with Alertmanager's own `pkg/labels` parser (ADR decision 1); `receiver`,
  `silenced`, `inhibited`, `unprocessed` mapped with v1 defaults.
- Response reshaping: v2 `receivers: [{name}]` → v1 strings; status block; fields v2 adds
  (`updatedAt`) dropped; fields v1 had that v2 lacks — decided per field, recorded, and counted as
  lossy per ADR decision 4.
- `/alerts/groups`: v1's group structure differs materially from v2's — derive from 020's fixtures,
  not memory.
- Telemetry: parsed filter and flags as span attributes.

## Explicit non-goals

- Silence-related endpoints (060).

## Key files

`internal/v1/alerts_get.go`, `internal/v1/groups.go`, `internal/v1/filter/`.
