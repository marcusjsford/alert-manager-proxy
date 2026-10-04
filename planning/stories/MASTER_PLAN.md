# alertmanager-proxy — Master Plan

Tracks the story breakdown of the Alertmanager proxy — permanent infrastructure in front of each of
the four Alertmanagers ([ADR decision 6](../ADR.md#decision-6--the-proxy-is-permanent-the-v1-layer-is-what-retires)).
Two goals: unblock upgrading past 0.26 (v1 removed in 0.27.0) by translating v1→v2 for legacy
clients that can't change, and make the whole alert path observable — per client, protocol, endpoint
and parameter, for v1 and v2 — for as long as it runs. Only the v1 layer retires.

Source of design decisions: [`../ADR.md`](../ADR.md). Every story inherits it.

## Workflow

The full working method lives in [`../STORY_WORKFLOW_PROMPT.md`](../STORY_WORKFLOW_PROMPT.md) — read
it before starting a story. Short version: pick the lowest-numbered non-`Done` story whose
dependencies are `Done`, cut a worktree/branch for it, read its `plan.md`/`acceptance-criteria.md`,
verify ground truth (story 020's fixtures) before implementing against an assumption, log findings
as you go, re-verify every acceptance criterion explicitly, then update that story's status and add a
short Findings Summary below.

## Status legend

`Not started` · `In progress` · `Blocked` · `Needs re-scoping` · `Done`

## Story index

| # | Story | Depends on | Status | One-line summary |
|---|---|---|---|---|
| [010](010-skeleton-and-passthrough/) | Skeleton + transparent pass-through | — | **Done** | Go module, config, reverse proxy that passes every path through unchanged, Dockerfile, CI; accepts the ADR |
| [015](015-forgejo-ci-pipeline/) | Forgejo Actions CI working | 010 | Not started | Make the pushed `ci.yml` actually run green on the local Forgejo; until then `make ci` is the gate |
| [020](020-v1-contract-capture-and-harness/) | v1 contract capture + diff harness | 010 | Not started | Record fixtures for every v1 endpoint from a real AM v0.26.0; black-box harness replays them against proxy → current AM and diffs |
| [030](030-otel-foundation-and-client-identity/) | OTel foundation + client identity | 010 | Not started | Traces, metrics, logs over OTLP; config-driven client identification; per-instance identity |
| [040](040-post-alerts/) | `POST /api/v1/alerts` | 020, 030 | Not started | The legacy-Prometheus hot path |
| [050](050-get-alerts-and-groups/) | `GET /api/v1/alerts` + `/alerts/groups` | 040 | Not started | Query-param and `filter=` translation, response reshaping |
| [060](060-silences/) | Silences | 040 | Not started | List/get/create/update/delete, matcher translation |
| [070](070-status-and-receivers/) | `status` + `receivers` | 040 | Not started | Reshape v2 status into the v1 layout legacy clients parse |
| [075](075-v2-request-instrumentation/) | v2 request-level instrumentation | 030 | Not started | Same span attributes and alert counters for v2 pass-through calls as for v1, without rewriting v2 traffic |
| [080](080-partial-fanout-detection/) | Partial-fanout detection | 040, 075 | Not started | Report clients that post alerts to fewer than all four Alertmanagers |
| [090](090-opt-in-alert-replication/) | Opt-in alert replication | 080 | Not started | Proxy forwards received alerts to peer Alertmanagers for partial clients |
| [100](100-alert-path-dashboards/) | Alert-path dashboards | 075, 080 | Not started | Permanent per-client view of the alert path (v1 and v2) + partial fanout; includes the v1-retirement view |
| [105](105-upstream-release-compatibility/) | Upstream release compatibility | 050, 060, 070 | Not started | Harness runs against each new Alertmanager release in CI; upstream upgrades are gated on it |
| [110](110-packaging-and-cutover/) | Packaging, operations + cutover | 100, 105 | Not started | Deploy 1:1 with each AM as permanent infra (health, limits, self-alerting), repoint clients, upgrade AM, rollback; `v1.enabled` switch-off procedure |
| [120](120-rust-port-evaluation/) | Rust port evaluation | 110 | Not started | Port to Rust; keep only if it measurably wins on footprint/latency and passes the identical harness |

030 can run before or after 020 (both depend only on 010); it is ordered before the translation
stories so each of them adds its own span attributes rather than retrofitting telemetry at the end.
075 has the same dependency as 030 alone, but is numbered after the translation stories so it can
reuse the attribute helpers they settle on.

Ending v1 translation is an operational step (flip `v1.enabled` once story 100's v1 view shows no
traffic for the agreed period, procedure in 110), not a story — the proxy itself is not retired.

## Completed Story Summaries

### 010 — Skeleton + transparent pass-through

Go proxy that passes every non-v1 path through to one Alertmanager unchanged (byte-identical
status, alerts, silences and UI against a real v0.34.1), answers `/api/v1/*` with `501` in v1's
error envelope, and returns `502`/`504` for unreachable/slow upstreams. Checking the real systems
first corrected the plan: a current Alertmanager answers v1 with `410 Gone` and a "deprecated" body,
not `404`; and 0.26's v1 responses carry CORS headers that later stories must reproduce. Found and
fixed a 502/504 misclassification of dial timeouts, and an integration test that a mutation showed
couldn't detect dropped query strings. Go baseline: 6.9 MiB binary, 16.5 MB image, 2.5 MiB idle RSS.
CI isn't working on the local Forgejo yet; the CI-green criterion moved to story 015 and `make ci`
is the gate meanwhile.
