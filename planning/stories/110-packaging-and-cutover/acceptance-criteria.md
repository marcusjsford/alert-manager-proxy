# 110 — Acceptance criteria

- [ ] Existing 074 health/restart/error/latency procedures are linked to committed 100 dashboards,
      080 fanout rules and 105 release checks; operational queries and alert routes are exercised.
- [ ] With `v1.enabled=false`, v1 calls get the same response a 0.27+ Alertmanager returns, and v2
      traffic and telemetry are unaffected (tested).
- [ ] Staging rehearsal disables and restores v1 while v2 traffic continues; results, timings and
      restart/hot-reload behaviour are recorded.
- [ ] Runbook names the agreed zero-v1 period, evidence query, owners, human approval and rollback
      steps. If production still uses v1, it stays enabled and that decision is recorded.
- [ ] Permanent operations/release procedures incorporate findings from 074; initial cutover
      evidence is referenced rather than repeated as a new prerequisite.
