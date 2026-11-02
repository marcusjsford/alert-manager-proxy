# 072 — MVP target-version compatibility gate

**Depends on:** 015, 025, 050, 060, 070 · **Status:** Done

## Goal

Prove the entire required v1 surface works through the proxy against both the initial 0.26
deployment and the exact 0.34.1 upgrade target, with a release gate that cannot accept expected
failures. Actual estate client versions and production evidence are owned by 074 after the
2026-10-29 re-scope.

## Scope

- Add a strict release target that runs the full contract harness with an empty pending list for
  both 0.26.0 and 0.34.1. All 48 existing exchanges must pass; regular development XFAILs cannot
  qualify a release.
- Parameterise the contract and pass-through version pins so CI tests proxy → 0.26 and proxy →
  the selected target. Account for native v1 presence on 0.26 in integration assertions.
- Exercise 025's representative Prometheus plus the dependency-free Python v1 client probe covering
  alert send/read/filter, resolved alerts, silence management/suppression, status/versionInfo and
  receivers. Actual estate client versions, the oldest real sender, production auth/path/manifest
  details and fanout evidence move to 074.
- Add targeted assertions for behaviour masked by normalisation (versionInfo policy and timestamp
  precision), keeping only documented/observable compatibility differences.
- Verify translated writes under upstream failure, timeout and recovery, required auth forwarding,
  and v2 pass-through fidelity. No failed write receives a success acknowledgement.
- Document that every production Alertmanager upgrade requires a strict green run for its exact
  target; 105 later automates release discovery and expands the matrix.

## Explicit non-goals

- Scheduled release discovery, full dashboards, automated fanout detection, production deployment,
  or claiming unknown estate clients compatible. Production client/cutover evidence belongs to 074.

## Key files

`Makefile`, `.forgejo/workflows/ci.yml`, `test/contract/`, `test/integration/`,
`test/clients/v1_client.py`, compatibility evidence.
