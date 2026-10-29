# 074 — Findings

_Append-only. Record discoveries, evidence gaps, decisions and deviations as they happen._

## 2026-10-29 — MVP boundary split from 110

Initial deployment and upgrade no longer wait for 075/080/100. Basic telemetry, failure visibility,
four-node rehearsal and rollback remain mandatory. The proxy stays permanent infrastructure;
dashboard-backed retirement and the v1 disable switch stay in 110.

## 2026-10-29 — Topology and migration intent confirmed in 025

Operator corrected the target to three instances on Kubernetes, upgrading from 0.26.0 to 0.34.1.
Clients move through proxies before old direct v1 endpoints are deprecated. Parallel replacement
versus rolling migration remains to be confirmed and rehearsed; proxy translation must continue
for migrated clients still speaking v1. The earlier four-node description is superseded.
