# 074 — Acceptance criteria

- [ ] Versioned deployment artefacts match 025's platform and retain one proxy per Alertmanager and
      the required client fanout; health checks distinguish proxy/upstream failures.
- [ ] Restart/resource settings are exercised; killing a proxy produces its self-alert and restart;
      basic request/error/latency/client queries work and collector failure does not block delivery.
- [ ] Strict 072 gate is green for both deployment phases; four-node rehearsal includes real client
      traffic, firing/resolved notifications, silence gossip/suppression and verified alert fanout.
- [ ] Mixed-version gossip (silences and notification log) is verified for the rehearsed upgrade
      path, with rollback per phase and recorded timings.
- [ ] Runbook records agreed soak/acceptance thresholds, release/versioning steps and rollback;
      soak results meet those thresholds.
- [ ] Production cutover is explicitly human-approved and completed; date/results/incidents are
      recorded. Staging success alone does not mark production cutover Done.
