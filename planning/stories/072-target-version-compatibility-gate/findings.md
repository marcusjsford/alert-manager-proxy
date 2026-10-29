# 072 — Findings

_Append-only. Record discoveries, evidence gaps, decisions and deviations as they happen._

## 2026-10-29 — Split from ongoing release compatibility

The development harness currently exits successfully with all 48 exchanges failing as expected.
The MVP needs a strict target-specific gate now; scheduled future-release discovery stays in 105.

## 2026-10-29 — Operator contract and remaining estate evidence

025 confirms 0.26.0 → 0.34.1, three target instances on Kubernetes, and a Docker development lab.
Real estate client versions and deployment/auth/path details remain unavailable; this story must
close those inventory gaps, including testing the oldest actual sender, before production claims.

## 2026-10-29 — Re-scope decision (operator)

Story 025 records that the actual estate client versions, oldest real sender, production auth/path
details and cutover manifests are not available. The operator chose to keep 072 as the development/
CI release gate and move actual estate-client evidence into 074. 072 therefore requires a
representative client corpus, not an unsupported production-compatibility claim.

## 2026-10-29 — Strict gate implementation

- `contract verify -strict` now rejects any non-empty pending list before touching a target, then
  requires every fixture to match. `make strict-contract` (alias `make release-check`) runs the
  strict harness against `v0.26.0` and `v0.34.1` sequentially. `make ci` runs both versions.
- The CI contract job now uses `-strict`; a second `contract-v026` job runs the same gate against
  0.26.0. Both are required by `docker-build`, so a failed/strict contract prevents image release.
- A second `integration-v026` CI job exercises pass-through and translated reads against native v1
  on 0.26. The integration status test now expects native v1 200 on 0.26 and 410 on 0.34.1, while
  the proxy must return the translated 200 payload in both cases.
- Harness tests cover the strict gate mechanics: an empty-pending failing translation is rejected,
  a non-empty pending list is rejected, and the existing `FAIL`/`XPASS` verdict tests remain.
  The manual `ci-mutation-check` branch remains the real integration-mutation evidence.
- `make v1-client-check` runs the Python v1 probe against the three-proxy lab and receiver;
  `make v1-client-syntax` is a dependency-free syntax check.

## 2026-10-29 — Python v1 representative client

`test/clients/v1_client.py` is a dependency-free Python 3 client with URL repetition, path-prefix,
custom-header and TLS options. Against the lab it proved: status/config/versionInfo, receivers,
error/preflight contract, firing and resolved alert delivery, braced/unbraced filters, receiver
regex and timestamps, silence create/list/get/filter, alert suppression, `silenced=false`, in-place
extension, matcher-change replacement, gossip to all three proxies, no receiver notification for a
silenced alert, delete/idempotence/unknown-ID, and alert restoration after deletion. It cleans up
probe silences and exits non-zero on any failed assertion. This is representative evidence; it is
not the unknown estate oldest sender.

## 2026-10-29 — Focused policy assertions

- Integration now compares v1 `versionInfo.version` with the upstream v2 status on both 0.26 and
  0.34.1; the contract normaliser masks that object, so this closes the policy gap.
- Integration posts nanosecond timestamps through v1 and verifies the v2 store and translated v1
  read expose millisecond-truncated instants on both versions. Existing telemetry tests assert the
  `alert.timestamps` lossy event/counter.
- Existing tests cover refused/slow/cancelled upstreams, timeout/recovery, failed-write status,
  auth forwarding and non-v1 pass-through.

## Verification

- `make strict-contract`: 48/48 PASS against proxy → 0.26.0 and proxy → 0.34.1, empty pending.
- Non-empty pending list: strict command fails with `strict release mode requires an empty pending
  list` before running.
- `make integration AM_VERSION=v0.26.0` and the target run pass, including native-v1/versionInfo and
  timestamp-policy assertions.
- `make v1-client-check`: Python client passes all checks against the three-proxy lab and receiver.
- `make ci`: both integration runs, both strict contract runs and the Docker build pass with zero
  pending exemptions. The Python client passes all checks against the three-proxy lab and receiver.
  Dual-version Forgejo job results are recorded on the PR.
