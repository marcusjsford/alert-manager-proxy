# 025 — Client inventory + Docker baseline

**Depends on:** 015 · **Status:** Done

## Goal

Record the operator-confirmed upgrade contract and provide a reproducible Docker development lab.
Track unavailable estate evidence explicitly for validation before production cutover.

## Scope

- Inventory known legacy client product/version, owner, v1 endpoints, parameters, request/response
  expectations, resend intervals and destinations; unknown production clients remain an evidence
  gap for 072/074, not a reason to invent versions on this development machine.
- Obtain sanitised requests from existing logs/configuration or an isolated reproduction. Record
  missing production access explicitly; do not deploy the current 501 v1 placeholder to capture traffic.
- Pin the existing production version and selected upgrade target; distinguish the test default
  v0.34.1 from a confirmed production upgrade decision.
- Record production Kubernetes requirements and outstanding addresses/ports, TLS/authentication,
  ingress/path prefixes, trusted proxies and exact manifests for 072/074 verification.
- Operator confirmed Kubernetes, three Alertmanager instances, current 0.26.0 and target 0.34.1
  on 2026-10-29. Record the need to obtain actual manifests and determine parallel-set versus
  rolling cutover before 072/074 rather than assume the old plan's four-node rolling upgrade.
  Client versions remain unknown.
- Extend story 020's scenario/fixtures with uncovered real request shapes, recording from 0.26
  before implementing. Pin the representative development client binary; actual estate versions
  remain evidence to collect for the release gate.
- Operator clarified that this is a development machine and requested our own Docker test setup.
  Build an isolated three-instance 0.34.1 cluster with 1:1 proxies, a separate 0.26.0 reference,
  a test notification receiver and pinned representative v1-capable Prometheus. This client is a
  reproducible development baseline, not the estate's unknown oldest Prometheus.
- Expose loopback-only ports, provide start/check/stop commands and verify real cluster membership,
  v2 delivery/resolution, silence gossip and legacy delivery to the reference. Translation is still
  unimplemented: report 501 responses honestly and leave its implementation to 035/040–070.

## Explicit non-goals

- Translation implementation, production access/deployment, selecting unknown estate client
  versions, changing production fanout or selecting a Rust port.

## Key files

`planning/client-inventory.md`, `test/lab/`, `Makefile`, existing contract scenario/fixtures as needed.
