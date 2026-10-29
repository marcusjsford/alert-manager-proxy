# 070 — Acceptance criteria

- [ ] Contract harness: status and receivers fixtures diff clean (volatile fields normalised).
- [ ] Config JSON rendering masks secrets identically to 0.26 (test config with a webhook password and
      a Slack URL).
- [ ] The `versionInfo` decision is recorded in `findings.md` with reasoning, and tested.
- [ ] Status/receiver fixture parsing and focused version-gating policy tests pass; known real
      clients are exercised when available. Unknown estate clients remain explicit mandatory 072
      release evidence and are not claimed compatible from the synthetic baseline alone.
