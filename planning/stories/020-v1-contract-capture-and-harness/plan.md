# 020 — v1 contract capture + diff harness

**Depends on:** 010 · **Status:** Done

## Goal

Make the v1 contract a recorded fact (ADR decision 3), and build the harness every translation story
and the Rust port (120) are accepted against.

## Scope

- Spin up Alertmanager **v0.26.0** (last release with v1) with a fixed config (several receivers,
  nested routes, an inhibition rule) in a container.
- A scripted scenario that drives it via v1 and records request/response pairs for every v1 endpoint
  and the variants legacy clients actually use:
  - `POST /api/v1/alerts` (valid, missing labels, `endsAt` in past, malformed JSON)
  - `GET /api/v1/alerts` with each of `filter`, `receiver`, `silenced`, `inhibited`, `unprocessed`
  - ~~`GET /api/v1/alerts/groups`~~ — does not exist in v0.26.0 (404); recorded as an unknown route
    instead (see `findings.md`)
  - `GET /api/v1/silences` (with/without `filter`), `GET /api/v1/silence/:id`,
    `POST /api/v1/silences` (create, update-by-id, invalid), `DELETE /api/v1/silence/:id`
  - `GET /api/v1/status`, `GET /api/v1/receivers`
  - error cases: unknown silence ID, bad matcher syntax
- Capture real traffic shapes too: if possible, record (sanitised) requests from the actual legacy
  clients against today's 0.26 cluster so fixtures reflect what's really sent, not just what's
  possible. List which clients/endpoints were observed in `findings.md`.
- Black-box harness (language-agnostic: it only speaks HTTP, so story 120 reuses it as-is): replay
  the same scenario against **proxy → current Alertmanager**, normalise volatile fields (timestamps,
  IDs, fingerprints, uptime, version), and diff. Unimplemented endpoints are reported as expected
  failures, not hidden.

## Explicit non-goals

- Making any diff pass — that's 040–070.

## Key files

`test/contract/` (scenario, fixtures, normaliser, runner), `test/contract/docker-compose.yml`,
`Makefile` target `contract`.
