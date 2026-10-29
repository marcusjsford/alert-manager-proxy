# 015 — Findings

_Append-only. Log discoveries, wrong assumptions, decisions made under ambiguity (and why), and
deviations from the plan **as they happen**, not reconstructed afterwards._

No findings recorded yet — story not started.

## 2026-10-29 — Resumed existing work; verified Forgejo ground truth

The review was initially based on main, where 015 was Not started and ci.yml lacked the contract
job. A clean existing worktree/branch `story/015-forgejo-ci-pipeline` contains pushed commit
`215882b` (2026-10-09), which adds a fifth contract job and makes docker-build depend on it.
Resume that work without rewriting its history. A separate clean local `ci-mutation-check` branch
at `49dcdc9` deliberately drops RawQuery; it has not been pushed and must never be merged.

Read-only Forgejo API verification:

- [Push run 9](http://localhost:3000/gitadmin/alertmanager-proxy/actions/runs/9), internal run ID
  6273, commit `215882b`: all five jobs succeeded. Job IDs: build-test 13232, lint 13233,
  integration 13234, contract 13235, docker-build 13236. The integration log contains the actual
  TestPassThroughMatchesDirect subtests, not a skip, and successful clone/checkout from forgejo.
- The first two runs (1/2, internal IDs 5546/5551) were cancelled before starting; run 1's jobs
  all have task_id=0. Subsequent runs 3–9 succeeded. Run 9 queued from 20:28:55 to 20:54:22 UTC.
  This is evidence of scheduling delay, not a failed clone/service/build. The retained run metadata
  does not prove why the runner was delayed in October 4; do not invent a historical root cause.
- Today's discrepancy is verified: successful unmerged branch work and CI evidence were never
  reflected in planning. Main also still omits the contract check. No shared-runner change is
  indicated by the evidence, and none is made.

The initial root-cause acceptance wording was amended to require investigation and honest limits
on historical evidence, rather than claiming an unrecoverable historical cause was established.
The user authorised committing/pushing this branch and the isolated mutation branch for real CI
verification. Merge/deployment are not part of this execution.

## 2026-10-29 — MVP-first planning changes

Added 025 (real client/target/deployment evidence), 035 (shared HTTP contract), 072 (strict exact-
target gate) and 074 (initial operations/cutover). Moved envelope ownership out of 040 into 035;
075/080/100/105/110/090/120 follow the 074 MVP boundary. Stable IDs remain, so workflow selection
now follows the ordered index rather than numeric IDs. 110 retains permanent-operation refinements
and v1 retirement; it no longer gates the first upgrade. Production still requires explicit human
approval, strict compatibility, basic telemetry, HA rehearsal and rollback.

## 2026-10-29 — Local verification and check parity

`rtk proxy make ci` passed in the resumed worktree: static build, go vet, go test -race,
golangci-lint (0 issues), integration against real v0.34.1, contract replay, and Docker build.
The contract table is still 0/48, all ten endpoint categories XFAIL, which is correct for the
development gate and not a claim of working v1 translation. Strict release verification is 072.

Manual local/remote check comparison:

| Local make ci prerequisite | Forgejo job/check |
|---|---|
| build: static proxy build | build-test: go build ./...; static binary also built by docker-build |
| test: go vet + go test -race | build-test: same vet and race-test commands |
| lint: pinned v2.14.0 container | lint: same pinned image, golangci-lint run ./... |
| integration: real AM v0.34.1 | integration: same pinned AM service, same tagged uncached tests and upstream env |
| contract: proxy + configured AM v0.34.1 | contract: same config/scenario/fixtures/pending list, checksum-verified AM release binary |
| docker: versioned image build | docker-build: same Dockerfile, commit SHA as version |

The execution environments differ (Compose locally, service/release binary in CI), but each uses
real Alertmanager and the same checks. docker-build needs build-test, lint, integration and
contract; the existing push run confirms all five execute. No runner configuration was modified.

## 2026-10-29 — Real mutation push proves the integration gate

Pushed the pre-existing isolated `ci-mutation-check` commit `49dcdc9` with user authorisation.
[Run 10](http://localhost:3000/gitadmin/alertmanager-proxy/actions/runs/10), internal run ID 8529,
failed as intended: build-test and lint fail too, and integration job 19919 specifically fails
TestPassThroughMatchesDirect/POST_/api/v2/alerts. The filtered direct response contains only the
two source=amp-integration alerts; the proxied response additionally contains source=other because
the deliberate mutation clears RawQuery. This is a real behavioural failure against the AM job
service, not a compile failure or an upstream-unavailable false positive. docker-build is skipped;
contract succeeds with its current expected v1 failures. The mutation branch stays isolated and
must never be merged.

One-off roadmap consistency verification passed: all 19 folders have the required three files,
master/plan dependency and status copies agree, every dependency precedes its consumer, and 074's
ancestors contain the compatibility work but none of the post-MVP observability/retirement stories.
