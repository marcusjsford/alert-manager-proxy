# 110 — Packaging, operations + cutover

**Depends on:** 100, 105 · **Status:** Not started

## Goal

Put the proxy in production 1:1 with each Alertmanager **as permanent infrastructure**, then upgrade
Alertmanager past 0.26 — reversibly at every step.

## Scope

- Deployment artefacts matching how the four Alertmanagers run today (confirm: systemd, compose,
  k8s?) — same host, proxy on the client-facing port or a new one.
- Operating it permanently (ADR decision 6): liveness/readiness that reflect the proxy's own health
  separately from upstream's, resource limits, restart policy, self-alerting on proxy errors and
  added latency, and a check that the proxy is never down while its Alertmanager is up.
- `v1.enabled` config switch: disables the v1 layer (v1 paths return the same error 0.27+ would)
  without restart or topology change if feasible; tested.
- Phased runbook:
  1. Deploy proxies in front of the **existing 0.26** cluster; legacy clients repointed (or
     port-swapped) — validates translation against the version they already use.
  2. Soak, watching the dashboards (100) and partial-fanout (080).
  3. Rolling upgrade Alertmanager node by node, gated on a green 105 run against the target
     version; mixed-version gossip confirmed beforehand.
  4. Rollback for each step.
  5. Later, separately: switching `v1.enabled` off once 100's v1 view shows zero traffic for the
     agreed period.
- Release process: tagged images, versioning, changelog.

## Explicit non-goals

- Rust port (120).

## Key files

`deploy/`, `docs/runbook.md`.
