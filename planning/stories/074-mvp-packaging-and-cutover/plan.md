# 074 — MVP packaging, operations + cutover

**Depends on:** 072 · **Status:** Not started

## Goal

Release v1 compatibility in the confirmed four-node topology and upgrade Alertmanager reversibly,
without waiting for the post-MVP observability programme.

## Scope

- Deployment artefacts for 025's confirmed platform: one proxy per Alertmanager, existing client
  fanout preserved, pinned/versioned image, configuration example, ports/auth/TLS arrangements.
- Separate proxy and upstream health checks, resource limits, restart policy, basic error/latency
  self-alerting and telemetry queries using 030. Demonstrate telemetry-backend failure does not
  interrupt delivery. Full dashboards and automated partial-fanout detection are not prerequisites.
- Rehearse on four nodes: deploy in front of 0.26, repoint clients or swap ports, soak with the
  inventory's clients, verify fanout and notification/silence behaviour, roll forward to the target,
  and roll back each phase. Validate mixed-version silence/notification-log gossip first; if the
  selected versions cannot coexist, record and rehearse a supported upgrade path before proceeding.
- Record soak duration and release thresholds with the operator before executing the rehearsal.
- Runbook, tagged-image release/versioning procedure and human-approved production cutover.
  Collect client-last-seen/errors/latency evidence with basic queries before full story 100 panels.

## Explicit non-goals

- v1 disable/retirement switch (110), deep v2 inspection (075), dashboards (100), automated fanout
  detection/replication (080/090), scheduled release discovery (105), Rust (120).

## Key files

`deploy/`, `docs/runbook.md`, release notes and rehearsal evidence.
