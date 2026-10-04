# 020 — Acceptance criteria

- [ ] Fixtures exist for every endpoint and variant listed in `plan.md`, recorded from a real
      Alertmanager v0.26.0 (version string captured in the fixture metadata).
- [ ] Re-recording against v0.26.0 twice produces identical fixtures after normalisation (the
      normaliser is complete, not flaky).
- [ ] `make contract` runs the harness against proxy → current Alertmanager and prints a per-endpoint
      pass/fail table; currently-unimplemented endpoints show as expected failures.
- [ ] The harness has no dependency on the proxy's implementation language (it targets a base URL).
- [ ] `findings.md` records which legacy clients and endpoints were observed in real traffic (or why
      real-traffic capture wasn't possible), and every v1/v2 shape difference noticed while recording.
