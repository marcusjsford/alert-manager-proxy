# 070 — Acceptance criteria

- [ ] Contract harness: status and receivers fixtures diff clean (volatile fields normalised).
- [ ] Config JSON rendering masks secrets identically to 0.26 (test config with a webhook password and
      a Slack URL).
- [ ] The `versionInfo` decision is recorded in `findings.md` with reasoning, and tested.
- [ ] Every legacy client observed in 020 that calls `status` is exercised against the proxy (or the
      gap recorded).
