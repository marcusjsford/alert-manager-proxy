# Upstream release compatibility

`check-version.sh` runs the full release gate against one Alertmanager
release: the strict v1 contract harness (story 020/072) and the pass-through /
inspection tests (stories 035/075). It downloads the checksum-verified
Alertmanager release binary, starts it with `test/contract/alertmanager.yml`,
builds the proxy, and runs both suites against it.

```sh
test/release/check-version.sh v0.35.0
make upstream-check   # latest upstream release
make compat-matrix    # production, selected target and latest upstream
make release-check    # strict-contract + latest upstream
```

`test/release/latest-version.sh` resolves the latest upstream release from the
GitHub `releases/latest` redirect. Set `AM_LATEST_VERSION` to pin or simulate a
version without network discovery.

## Versions in the matrix

| Version | Source | Purpose |
|---|---|---|
| `PRODUCTION_VERSION` (default `v0.26.0`) | `Makefile` | The Alertmanager in production today |
| `TARGET_VERSION` (default `v0.34.1`) | `Makefile` | The selected upgrade target; still required by 072 |
| latest upstream | discovered | Catches breaking upstream changes and new releases |

`make ci` runs `upstream-check` in addition to the pinned production/target
checks, so a local CI run matches the Forgejo compatibility jobs.

## CI and schedule

- `.forgejo/workflows/ci.yml` runs three compatibility jobs on every push and
  PR: `compat-production`, `compat-target`, `compat-latest`. Each runs
  `check-version.sh` and is strict (the pending list must be empty).
- `.forgejo/workflows/upstream-release.yml` runs daily and can be dispatched
  with an explicit version. It discovers the latest release and runs the same
  script, so a release between pushes is still checked.

A failed contract step prints the per-endpoint table and the diffs for every
failing step in the job log. The final `REPORT alertmanager=v… contract=…
integration=…` line identifies the result in either workflow.

## Failure triage

1. Read the failing endpoint and diff in the log. The fixture is the v0.26.0
   contract; the proxy must translate it faithfully to the new upstream API.
2. Reproduce locally with `test/release/check-version.sh <version>`.
3. If the upstream v2 API changed, update the proxy translation/v2 client and
   record the API change in the story findings. Do not remove a fixture or add
   a pending exemption to make the gate green.
4. If a new upstream release is deliberately unsupported, record the decision
   in the ADR/runbook and pin the deployment to the last supported release;
   the gate must still fail rather than silently pass.

The Alertmanager module dependency (`github.com/prometheus/alertmanager`) and
`pkg/labels` usage are bumped with the proxy's supported/latest release so type
and matcher behaviour stays aligned. A bump that changes those APIs is part of
the translation work, not a cosmetic dependency update.
