# 105 — Acceptance criteria

- [x] CI runs the full harness against both the production and latest Alertmanager versions on every
      push; both strictly green without pending exemptions, plus pass-through/inspection tests.
      `.forgejo/workflows/ci.yml` runs `compat-production` (0.26.0), `compat-target` (0.34.1) and
      `compat-latest` (discovered) on every push/PR via `test/release/check-version.sh`; each runs
      the strict contract and `-tags integration` tests. Forgejo CI run #54 was green in all six
      jobs, and local `make compat-matrix` passed 0.26.0, 0.34.1 and 0.35.0.
- [x] The scheduled job, pointed at a release newer than the pinned one (or simulated by bumping the
      pin), runs the matrix and reports the result without manual steps.
      `.forgejo/workflows/upstream-release.yml` runs daily and supports a version input. A
      `workflow_dispatch` run pointed at **v0.35.0** (newer than the 0.34.1 target) completed
      successfully with `REPORT alertmanager=v0.35.0 contract=PASS integration=PASS`.
- [x] Deliberately breaking a translation (test branch) makes the matrix fail — proves it's a real gate.
      Reversing the `GET /api/v1/alerts` fingerprint sort made the v0.35.0 check exit 1 with
      `GET /api/v1/alerts 6/15 FAIL` and per-step fingerprint diffs; the mutation was reverted and
      the check returned to green.
