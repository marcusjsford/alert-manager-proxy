# 110 — Acceptance criteria

- [ ] Full contract harness green against proxy → current Alertmanager, and against proxy → 0.26
      (phase 1 config).
- [ ] Runbook rehearsed end-to-end on a 4-node staging cluster including one rollback per phase;
      timings recorded.
- [ ] Mixed 0.26/current cluster gossip of silences and notification log verified during the rolling
      upgrade rehearsal.
- [ ] Killing a proxy process triggers its self-alert, and its restart policy brings it back; recorded.
- [ ] With `v1.enabled=false`, v1 calls get the same response a 0.27+ Alertmanager returns, and v2
      traffic and telemetry are unaffected (tested).
- [ ] Production cutover done (human-approved per workflow), with date and any incidents in
      `findings.md`.
