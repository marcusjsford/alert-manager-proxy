# 070 — `GET /api/v1/status` + `GET /api/v1/receivers`

**Depends on:** 040 · **Status:** Not started

## Goal

Return the v1 status and receivers payloads that legacy dashboards parse.

## Scope

- Receivers: v2 `[{name}]` → v1 list of strings.
- Status: v2 `config.original` (YAML), `cluster`, `versionInfo`, `uptime` → v1 layout. v1 also
  exposed a JSON rendering of the config; producing it means loading the YAML with Alertmanager's own
  `config` package and marshalling — check whether secrets are masked the same way as 0.26 did.
- `versionInfo`: decide whether to report the upstream's real version or a v0.26-looking one; legacy
  clients that version-gate may care. Record the decision.
- Inputs recorded in [020's findings](../020-v1-contract-capture-and-harness/findings.md): v2 receivers
  are `{name, labels}` objects; v2 status has no `configJSON`; v1 masks secrets as `<secret>`.

## Explicit non-goals

- Exposing anything about the proxy itself in the v1 status payload.

## Key files

`internal/v1/status.go`, `internal/v1/receivers.go`.
