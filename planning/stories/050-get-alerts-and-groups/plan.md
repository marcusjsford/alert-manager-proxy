# 050 — `GET /api/v1/alerts`

**Depends on:** 040 · **Status:** Not started

## Goal

Serve v1 alert queries from v2, reshaped exactly as 0.26 returned them.

## Scope

- Query params: v1 `filter` (a single `{a="b",c=~"d"}` matcher-set string) → v2 repeated `filter`
  params, parsed with Alertmanager's own `pkg/labels` parser (ADR decision 1); `receiver`,
  `silenced`, `inhibited`, `unprocessed`, `active` mapped with v1 defaults.
- Response reshaping: v2 `receivers: [{name}]` → v1 strings; status block; fields v2 adds
  (`updatedAt`) dropped; fields v1 had that v2 lacks — decided per field, recorded, and counted as
  lossy per ADR decision 4.
- ~~`/alerts/groups`~~: **v0.26.0 has no `/api/v1/alerts/groups`** (404, found in story 020), so
  there is nothing to translate; it stays an unknown route that must 404 like 0.26 did. The folder
  name keeps "groups" so the story branch name stays stable.
- Inputs recorded in [020's findings](../020-v1-contract-capture-and-harness/findings.md): v2 rejects
  braced filters, `active` is also a v1 param, v1 `null` vs v2 `[]`, `mutedBy`, stale-vs-live state.
- Telemetry: parsed filter and flags as span attributes.
- Reuse 035's HTTP contract and remove the GET alerts pending entry when its fixtures pass.

## Explicit non-goals

- Silence-related endpoints (060).

## Key files

`internal/v1/alerts_get.go`, `internal/v1/filter/`.
