# 105 — Findings

_Append-only. Log discoveries, wrong assumptions, decisions made under ambiguity (and why), and
deviations from the plan **as they happen**, not reconstructed afterwards._

No findings recorded yet — story not started.

## 2026-11-24 — Implementation and verification

- Production is still **v0.26.0** and the operator-selected target is **v0.34.1**; latest upstream
  was **v0.35.0** when the story ran. The matrix therefore covers all three: production, selected
  target and latest. `PRODUCTION_VERSION` is a separate Makefile pin so it can be changed after
  cutover without losing the 072 target gate.
- Added `test/release/check-version.sh` (downloads the checksum-verified Alertmanager binary, starts
  it with the recorded contract config, builds the proxy, runs strict contract + integration) and
  `test/release/latest-version.sh` (GitHub `releases/latest` redirect, no API token/rate limit).
  `.forgejo/workflows/ci.yml` now runs `compat-production`, `compat-target` and `compat-latest` on
  every push; this replaced the separate integration/contract jobs so each version gets the full
  gate rather than contract-only coverage. `docker-build` depends on all three.
- Added `.forgejo/workflows/upstream-release.yml`, scheduled daily with a manual version input.
  A dispatch for v0.35.0 succeeded without intervention and logged the final report line. The
  schedule is the backstop between pushes; 074 still owns the production-upgrade runbook.
- Bumped `github.com/prometheus/alertmanager` from v0.34.1 to v0.35.0 (with routine transitive
  updates). Upstream's `types` removal, eventrecorder schema change and experimental ConnectRPC API
  do not affect the proxy: it imports `config`, `pkg/labels` and the v2 HTTP API only. No translation
  code changed; the 0.35.0 contract and integration runs proved the v2 surface used by the proxy is
  unchanged.
- Verification: `make compat-matrix` green for 0.26.0, 0.34.1 and 0.35.0 (48/48 fixtures each);
  `make ci` green; Forgejo CI run #54 green across all six jobs. A `workflow_dispatch` of the
  scheduled workflow (run #55, v0.35.0) green. Mutation check: reversing the `GET /api/v1/alerts`
  fingerprint sort produced exit 1, `GET /api/v1/alerts 6/15 FAIL`, and concrete fingerprint diffs;
  reverted immediately.
- Deviation: CI now downloads Alertmanager release binaries instead of using job service containers.
  This makes the same script usable locally, in CI, and in the scheduled job, and avoids the shared
  act-runner service-alias race that previously forced serial `integration`/`integration-v026` jobs.
- Limitation: when latest equals the selected target the matrix runs the same version twice; that is
  deliberate because the two pins mean different things (operator target vs upstream discovery).

## 2026-10-29 — MVP replan

072 owns the strict selected-target/0.26 gate before MVP cutover. This story now extends that gate
with current-production/latest coverage, 075 inspection checks and scheduled release discovery.
