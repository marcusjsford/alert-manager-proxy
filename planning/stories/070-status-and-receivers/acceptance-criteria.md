# 070 — Acceptance criteria

- [x] Contract harness: status and receivers fixtures diff clean (volatile fields normalised).
      `make contract` reports `GET /api/v1/status 1/1 PASS` and `GET /api/v1/receivers 1/1 PASS`;
      both pending entries are removed.
- [x] Config JSON rendering masks secrets identically to 0.26 (test config with a webhook password and
      a Slack URL).
      `TestGetStatusRendersLegacyConfigAndMasksSecrets` renders webhook basic-auth, webhook URL and
      Slack API secrets and asserts only `<secret>` remains in YAML and JSON.
- [x] The `versionInfo` decision is recorded in `findings.md` with reasoning, and tested.
      Decision: report the target's real version. `TestStatusReportsTargetVersionInfo` asserts
      `0.34.1`, not a fabricated 0.26 value. The contract normaliser still masks the object.
- [x] Status/receiver fixture parsing and focused version-gating policy tests pass; known real
      clients are exercised when available. Unknown estate clients remain explicit mandatory 072
      release evidence and are not claimed compatible from the synthetic baseline alone.
      Receivers/status unit tests cover names, cluster status and version policy. No estate status
      or receiver client is identified yet; that remains a 072 evidence gap.
