# 074 — Acceptance criteria

- [ ] Versioned Kubernetes artefacts match 025's manifests and retain one proxy per Alertmanager and
      the required client fanout; health checks distinguish proxy/upstream failures.
- [ ] Restart/resource settings are exercised; killing a proxy produces its self-alert and restart;
      100 dashboards and 080 fanout views work, and collector failure does not block delivery.
- [ ] Strict 072 gate is green for 0.26.0 and 0.34.1; three-instance target rehearsal includes real client
      traffic, firing/resolved notifications, silence gossip/suppression and verified alert fanout.
- [ ] Actual estate client versions (including the oldest sender), production auth/TLS/path and
      manifest requirements are recorded, and every required client class is run end-to-end with
      reproducible results. Representative-lab evidence alone does not satisfy this criterion.
- [ ] The operator-selected parallel-set or rolling path has verified silence/notification-state
      continuity, rollback per phase and recorded timings; rolling mixed-version gossip is proven
      before using that path.
- [ ] Runbook records agreed soak/acceptance thresholds, release/versioning steps and rollback;
      soak results meet those thresholds.
- [ ] Production cutover is explicitly human-approved and completed; date/results/incidents are
      recorded. Staging success alone does not mark production cutover Done.
- [ ] Old direct v1 endpoints are deprecated only after all clients have moved successfully and
      rollback requirements are satisfied; proxy translation continues for clients still using v1.
