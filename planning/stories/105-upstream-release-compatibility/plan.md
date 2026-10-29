# 105 — Upstream release compatibility

**Depends on:** 072, 075 · **Status:** Not started

## Goal

Keep a permanent proxy correct as Alertmanager releases continue
([ADR decision 6](../../ADR.md#decision-6--the-proxy-is-permanent-the-v1-layer-is-what-retires)).

## Scope

- Extend 072's strict exact-target gate: run the contract harness and pass-through/inspection tests
  against the Alertmanager version in production and the latest upstream release, with no pending
  exemptions. Keep the selected-upgrade gate available independently of scheduled discovery.
- A scheduled job that detects a new upstream Alertmanager release and runs the matrix against it,
  reporting any diff.
- Extend the upgrade policy/runbook introduced by 072/074 with automated release reporting and
  operator triage. A production upgrade still requires a strict green run for its exact target.
- Keep the vendored v2 client/models and `pkg/labels` dependency current; record any upstream API
  change that affected translation.

## Explicit non-goals

- Supporting Alertmanager versions older than the one in production after cutover.

## Key files

`.forgejo/workflows/ci.yml`, `.forgejo/workflows/upstream-release.yml`.
