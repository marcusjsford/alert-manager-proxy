# 074 — MVP packaging, operations + cutover

**Depends on:** 072 · **Status:** Not started

## Goal

Release v1 compatibility in the confirmed three-instance Kubernetes topology and move clients from
0.26.0 to the operator-selected 0.34.1 target reversibly,
without waiting for the post-MVP observability programme.

## Scope

- Kubernetes deployment artefacts matching 025's actual manifests: one proxy per Alertmanager, existing client
  fanout preserved, pinned/versioned image, configuration example, ports/auth/TLS arrangements.
- Separate proxy and upstream health checks, resource limits, restart policy, basic error/latency
  self-alerting and telemetry queries using 030. Demonstrate telemetry-backend failure does not
  interrupt delivery. Full dashboards and automated partial-fanout detection are not prerequisites.
- Rehearse a three-instance destination: verify proxy compatibility with 0.26 and 0.34.1, move the
  inventory's clients through proxies to the target set, soak, and verify fanout and notification/
  silence behaviour. Confirm parallel replacement versus rolling upgrade with the operator first;
  validate state continuity, migration/notification-log behaviour and rollback for the chosen path.
  A rolling mixed-version path requires verified gossip compatibility before proceeding; do not
  assume that 0.26/0.34.1 can safely share a cluster.
- Deprecate old direct v1 endpoints only after all clients have moved successfully and rollback
  requirements are met. Keep proxy v1 translation enabled for migrated legacy clients; disabling
  that layer later still belongs to 110 and requires evidence that v1 usage has ended.
- Record soak duration and release thresholds with the operator before executing the rehearsal.
- Runbook, tagged-image release/versioning procedure and human-approved production cutover.
  Collect client-last-seen/errors/latency evidence with basic queries before full story 100 panels.

## Explicit non-goals

- v1 disable/retirement switch (110), deep v2 inspection (075), dashboards (100), automated fanout
  detection/replication (080/090), scheduled release discovery (105), Rust (120).

## Key files

`deploy/`, `docs/runbook.md`, release notes and rehearsal evidence.
