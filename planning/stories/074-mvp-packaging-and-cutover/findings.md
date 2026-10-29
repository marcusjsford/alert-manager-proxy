# 074 — Findings

_Append-only. Record discoveries, evidence gaps, decisions and deviations as they happen._

## 2026-10-29 — MVP boundary split from 110

Initial deployment and upgrade no longer wait for 075/080/100. Basic telemetry, failure visibility,
four-node rehearsal and rollback remain mandatory. The proxy stays permanent infrastructure;
dashboard-backed retirement and the v1 disable switch stay in 110.
