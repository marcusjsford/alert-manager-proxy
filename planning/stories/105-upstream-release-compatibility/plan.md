# 105 — Upstream release compatibility

**Depends on:** 050, 060, 070 · **Status:** Not started

## Goal

Keep a permanent proxy correct as Alertmanager releases continue
([ADR decision 6](../../ADR.md#decision-6--the-proxy-is-permanent-the-v1-layer-is-what-retires)).

## Scope

- CI matrix: the contract harness (020) and pass-through tests (010, 075) against the Alertmanager
  version in production and the latest upstream release.
- A scheduled job that detects a new upstream Alertmanager release and runs the matrix against it,
  reporting any diff.
- Policy, written into the runbook (110): an Alertmanager upgrade in production requires a green run
  against the target version first.
- Keep the vendored v2 client/models and `pkg/labels` dependency current; record any upstream API
  change that affected translation.

## Explicit non-goals

- Supporting Alertmanager versions older than the one in production after cutover.

## Key files

`.forgejo/workflows/ci.yml`, `.forgejo/workflows/upstream-release.yml`.
