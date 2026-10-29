# 072 — MVP target-version compatibility gate

**Depends on:** 015, 025, 050, 060, 070 · **Status:** Not started

## Goal

Prove the entire required v1 surface works against both the initial 0.26 deployment and the exact
upgrade target before cutover, with a release gate that cannot accept expected failures.

## Scope

- Add a strict release target to run the full contract harness with an empty pending list and
  require the implementation pending list to be empty. All 48 existing exchanges and additions
  from 025 must pass; regular development XFAILs must not qualify a release.
- Parameterise the contract and pass-through version pins so CI tests proxy → 0.26 and proxy →
  the selected target. Account for native v1 presence on 0.26 in integration assertions.
- Exercise real legacy clients from 025, including firing/resolved delivery, silence mutation and
  suppression, filters, status parsing and clients that inspect versionInfo.
- Add targeted assertions for behaviour masked by normalisation (versionInfo policy and timestamp
  precision), keeping only documented/observable compatibility differences.
- Verify translated writes under upstream failure, timeout and recovery, required auth/path
  arrangements, and v2 pass-through fidelity. No failed write receives a success acknowledgement.
- Document that every production Alertmanager upgrade requires a strict green run for its exact
  target; 105 later automates release discovery and expands the matrix.

## Explicit non-goals

- Scheduled release discovery, full dashboards, automated fanout detection or production deployment.

## Key files

`Makefile`, `.forgejo/workflows/ci.yml`, `test/contract/`, `test/integration/`, compatibility evidence.
