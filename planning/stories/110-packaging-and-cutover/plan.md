# 110 — Permanent operations + v1 retirement

**Depends on:** 074, 100, 105 · **Status:** Not started

## Goal

Extend the MVP deployment's permanent operational procedures and safely retire only the v1 layer
when observed usage permits. Initial packaging and Alertmanager cutover now belong to 074.

## Scope

- Extend 074's versioned deployment artefacts/runbook with 100's dashboards and rules, 080's fanout
  evidence and 105's ongoing upstream compatibility checks. Retain health, limits, restart and
  self-alerting coverage established for MVP; avoid duplicating initial cutover work.
- `v1.enabled` config switch: disables the v1 layer (v1 paths return the same error 0.27+ would)
  without restart or topology change if feasible; tested.
- Retirement runbook: agree the zero-v1 observation period and owners, check the last-seen view,
  disable v1 without removing the proxy, verify v2 operation, and restore the switch on regression.
- Rehearse retirement and rollback on staging; production retirement needs human approval and
  remains conditional on the agreed usage threshold. Complete the operational capability even if
  legacy clients still require v1; do not force production retirement to mark this story Done.
- Refine ongoing release/incident procedures using real MVP operation findings.

## Explicit non-goals

- Initial packaging/deployment/Alertmanager upgrade (074). Rust port (120). Forced v1 retirement
  while clients remain active.

## Key files

`deploy/`, `docs/runbook.md`.
