# 020 — Acceptance criteria

- [x] Fixtures exist for every endpoint and variant listed in `plan.md`, recorded from a real
      Alertmanager v0.26.0 (version string captured in the fixture metadata).
      (`/alerts/groups` doesn't exist in v0.26.0; recorded as an unknown route — see `findings.md`.)
- [x] Re-recording against v0.26.0 twice produces identical fixtures after normalisation (the
      normaliser is complete, not flaky). (`make contract-determinism`, 3/3.)
- [x] `make contract` runs the harness against proxy → current Alertmanager and prints a per-endpoint
      pass/fail table; currently-unimplemented endpoints show as expected failures.
- [x] The harness has no dependency on the proxy's implementation language (it targets a base URL).
      (Verified against a real v0.26.0 as the target: 48/48 PASS.)
- [x] `findings.md` records which legacy clients and endpoints were observed in real traffic (or why
      real-traffic capture wasn't possible), and every v1/v2 shape difference noticed while recording.
