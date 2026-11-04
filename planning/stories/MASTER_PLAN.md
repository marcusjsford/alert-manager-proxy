# alertmanager-proxy — Master Plan

Tracks the story breakdown of the Alertmanager proxy — permanent infrastructure in front of each of
the three Alertmanagers ([ADR decision 6](../ADR.md#decision-6--the-proxy-is-permanent-the-v1-layer-is-what-retires)).
Two goals: unblock upgrading past 0.26 (v1 removed in 0.27.0) by translating v1→v2 for legacy
clients that can't change, and make the whole alert path observable — per client, protocol, endpoint
and parameter, for v1 and v2 — for as long as it runs. Only the v1 layer retires.

Source of design decisions: [`../ADR.md`](../ADR.md). Every story inherits it.

## Workflow

The full working method lives in [`../STORY_WORKFLOW_PROMPT.md`](../STORY_WORKFLOW_PROMPT.md) — read
it before starting a story. Short version: pick the first non-`Done` story in the ordered index whose
dependencies are `Done`, cut a worktree/branch for it, read its `plan.md`/`acceptance-criteria.md`,
verify ground truth (story 020's fixtures) before implementing against an assumption, log findings
as you go, re-verify every acceptance criterion explicitly, then update that story's status and add a
short Findings Summary below.

## Status legend

`Not started` · `In progress` · `Blocked` · `Needs re-scoping` · `Done`

## Story index

Rows are in execution order, not numeric order. IDs and existing folder names remain stable.
Finish an in-progress story before selecting the next eligible row.

| # | Story | Depends on | Status | One-line summary |
|---|---|---|---|---|
| [010](010-skeleton-and-passthrough/) | Skeleton + transparent pass-through | — | **Done** | Go module, config, reverse proxy that passes every path through unchanged, Dockerfile, CI; accepts the ADR |
| [020](020-v1-contract-capture-and-harness/) | v1 contract capture + diff harness | 010 | **Done** | Record fixtures for every v1 endpoint from a real AM v0.26.0; black-box harness replays them against proxy → current AM and diffs |
| [015](015-forgejo-ci-pipeline/) | Forgejo Actions CI working | 010, 020 | **Done** | Five-job Forgejo CI green; real mutation fails integration; local/remote check parity verified |
| [025](025-client-inventory-and-upgrade-target/) | Client inventory + Docker baseline | 015 | **Done** | Confirmed 0.26→0.34.1/three-instance Kubernetes contract; Docker lab and real v1-capable sender verified; production gaps tracked |
| [030](030-otel-foundation-and-client-identity/) | OTel foundation + client identity | 015 | **Done** | Opt-in OTLP traces/metrics/logs; bounded identities and labels, redaction/correlation, real ClickHouse and outage verification |
| [035](035-shared-v1-http-contract/) | Shared v1 HTTP contract | 020, 030 | **Done** | Envelopes, routing, CORS/preflight, cache headers, unknown paths/methods, and shared upstream error handling |
| [040](040-post-alerts/) | `POST /api/v1/alerts` | 025, 035 | **Done** | First delivery milestone: real legacy Prometheus sends firing/resolved alerts to a test receiver |
| [050](050-get-alerts-and-groups/) | `GET /api/v1/alerts` | 040 | **Done** | Query-param and `filter=` translation, response reshaping (`/alerts/groups` turned out not to exist in v0.26.0 v1) |
| [060](060-silences/) | Silences | 040 | **Done** | List/get/create/update/delete, matcher translation |
| [070](070-status-and-receivers/) | `status` + `receivers` | 040 | **Done** | Reshape v2 status into the v1 layout legacy clients parse |
| [072](072-target-version-compatibility-gate/) | MVP target-version compatibility gate | 015, 025, 050, 060, 070 | **Done** | Strict contract and pass-through gate against 0.26 and 0.34.1; representative-client and failure-path verification |
| [075](075-v2-request-instrumentation/) | v2 request-level instrumentation | 030 | **Done** | Same span attributes and alert counters for v2 pass-through calls as for v1, without rewriting v2 traffic |
| [080](080-partial-fanout-detection/) | Partial-fanout detection | 040, 075 | **Done** | Report clients that post alerts to fewer than all three Alertmanagers |
| [100](100-alert-path-dashboards/) | Alert-path dashboards | 075, 080 | **Done** | Permanent per-client view of the alert path (v1 and v2) + partial fanout; includes the v1-retirement view |
| [105](105-upstream-release-compatibility/) | Continuous upstream release compatibility | 072, 075 | Not started | Extend the MVP gate to production/latest and automatically test newly discovered releases |
| [090](090-opt-in-alert-replication/) | Opt-in alert replication | 080, 100 | Not started | Pre-cutover opt-in behaviour: forward alerts to peers for partial clients, with production enablement justified by observed need |
| [074](074-mvp-packaging-and-cutover/) | MVP packaging, operations + cutover | 072, 075, 080, 090, 100, 105 | Not started | Deploy 1:1 on Kubernetes, health/limits/restart/self-alerting, three-instance rehearsal, rollback, and human-approved cutover after feature completion |
| [110](110-packaging-and-cutover/) | Permanent operations + v1 retirement | 074, 100, 105 | Not started | Extend MVP operations with dashboard-backed procedures and the tested `v1.enabled` switch-off/rollback |
| [120](120-rust-port-evaluation/) | Rust port evaluation | 110 | Not started | Port to Rust; keep only if it measurably wins on footprint/latency and passes the identical harness |

## Compatibility MVP — release boundary at 074

- **040 is the development alert-sender milestone:** the pinned representative Prometheus delivers
  firing/resolved notifications through v1 → proxy → v2. Actual estate client compatibility remains
  a mandatory 074 production gate, as does read/silence/UI compatibility; 072 is the development/CI
  gate against both versions.
- **074 is the full compatibility MVP:** 025, 030, 035, 040–070, 072 and 074 are complete, all 48
  current contract exchanges plus estate-specific additions pass with no pending exemptions, and
  the chosen upgrade is rehearsed and deployed with human approval. Unknown clients are an explicit
  evidence gap, not assumed compatible. Preserve client delivery across the three-instance topology.
- Feature-complete work (v2 inspection, fanout detection, dashboards, scheduled release checks and
  opt-in replication) precedes 074. 074 is the cutover/release boundary after those features and the
  production evidence; 110 owns later permanent-operation refinements and v1 retirement. Rust is
  evaluated after 110.
- 072 gates the selected upgrade version now; 105 extends that gate to future releases. 074 owns
  initial packaging, estate client evidence and cutover after 075/080/090/100/105.
- Operator confirmed current 0.26.0, target 0.34.1 and three instances on Kubernetes in 025.
  [Client inventory](../client-inventory.md) records remaining client/deployment evidence and the
  distinction between deprecating old direct endpoints and disabling the proxy's v1 translation.

Ending v1 translation is an operational step (flip `v1.enabled` once story 100's v1 view shows no
traffic for the agreed period, procedure in 110), not a story — the proxy itself is not retired.

## 2026-10-29 — Review-driven replan

The user requested an MVP-first execution order. Added 025 (estate evidence), 035 (previously
unowned HTTP semantics), 072 (strict target gate), and 074 (initial operations/cutover). Removed the
dashboard/fanout prerequisite from initial cutover and retained those goals after the MVP. Resumed
the pre-existing clean 015 branch at `215882b`; its five-job push run is green, but its local-only
mutation branch and absent findings mean the story is not yet Done. Planning amendments travel
with 015 as the requested roadmap update; no future story implementation is included.

## Completed Story Summaries

### 100 — Alert-path dashboards

Added the committed Grafana ClickHouse dashboard covering client alert volume/firing-resolved split,
alertnames, last seen, error rate, silence operations/lifetime, 080 fanout, proxy latency/errors/
lossy/skipped inspection and the v1-retirement last-seen view. All 12 panel queries return real rows;
the v1 request total matches a one-day raw-span count. Added exporter-contract rules for a silent
client (2× per-client expected interval), new unknown clients and new v1 clients, with passing
`promtool` tests and a Grafana import/query check.

### 080 — Partial-fanout detection

Added a ClickHouse query and Prometheus-style rule that report named clients and unknown
address-based clients reaching fewer than the expected Alertmanager instances. A dedicated
three-AM/three-proxy stack proves 3/2/1 fanout (including one v2 post) and unknown-address
detection; the rule suppresses during proxy outages using instance coverage plus `up` health.
Production telemetry validation is explicitly deferred to post-074 evidence.

### 075 — v2 request-level instrumentation

Added bounded, read-only inspection of v2 alert and silence traffic on the pass-through path. Bodies
are capped at 1 MiB and forwarded byte-identically; response inspection adds the silence ID/matchers
needed for v1 parity. v1 and v2 calls now expose the same application span attributes, v2 alert posts
increment the received-alert metric, and oversized/malformed bodies increment the new inspection
skipped counter. Latency and byte-preservation tests are recorded; 074 was deferred so feature work
continues before cutover.

### 072 — MVP target-version compatibility gate

Added a strict release command and CI jobs that run all 48 fixtures against proxy → 0.26.0 and
proxy → 0.34.1 with an empty pending list; non-empty pending and any diff now fail before release.
Pass-through integration and the Docker gate run for both versions. A dependency-free Python v1
client exercises firing/resolved alerts, filters, silence suppression/gossip, status/versionInfo and
receivers; focused integration checks cover the real versionInfo policy and millisecond timestamp
truncation. Actual estate client versions, the oldest sender and production auth/path/manifests were
re-scoped by the operator into 074.

### 070 — `status` + `receivers`

Reshaped v2 status into the v1 layout, including legacy YAML/JSON config rendering with target-only
fields removed and secrets masked as `<secret>`. Chose to report the real upstream version rather
than impersonating 0.26 and recorded that decision/risk for 072. Receivers become the v1 name list.
All 48 contract exchanges now pass with no pending exemptions; config-package footprint and unknown
estate status-client evidence are recorded.

### 060 — Silences

Implemented v1 silence list/filter/get/create/update/delete through the shared v2 client, preserving
0.26's matcher decode, validation order, error strings, in-place-versus-new-ID update behaviour and
list ordering. v2's differing unknown-ID, malformed-ID and create errors are mapped back to the v1
envelopes/plain-text responses. Filtering is applied client-side with v1's label-matching semantics.
Telemetry records the operation, silence ID, matchers and redacted creator/comment; lab checks prove
v1-created silences gossip to all three instances and mute a real alert so the receiver gets nothing.
All silence contract fixtures pass and their pending entries are removed.

### 050 — `GET /api/v1/alerts`

Translated v1 alert reads through the shared v2 client: Alertmanager's own matcher parser handles
braced/unbraced filters and sends repeated v2 matchers, while receiver and boolean validation
reproduce 0.26's errors and defaults. Responses are reshaped to v1 (receiver names, status null/[]
rules, null annotations, empty generatorURL), sorted by fingerprint, and timestamp precision is
reported as lossy. The recorded silence setup moved to v2 so all 15 GET fixtures pass independently
of story 060; set-valued UUID normalisation was made deterministic and all 48 fixtures re-recorded
and re-verified. Telemetry records the parsed query and lossy counter.

### 040 — `POST /api/v1/alerts`

Implemented v1 alert ingestion with 0.26's exact decode/validation errors, v2 field translation and
the shared auth/timeout/telemetry path. The five recorded POST cases pass; a real 0.34.1 shows the
posted fields via v2, and the pinned Prometheus 2.55.1 now delivers firing and resolved v1 alerts
through every proxy to the lab receiver. Spans carry v1 route/count/identity and bounded alertnames,
the received-alert metric increments, and failed or timed-out writes return v1 errors rather than
success. Other v1 endpoints remain their expected XFAILs.

### 035 — Shared v1 HTTP contract

Added shared v0.26 envelopes, CORS/cache/Content-Type headers, plain-text exceptions and
method-aware v1 routing, including the recorded 404/405/preflight behaviour and trailing-slash
redirects. All known endpoints stay explicit 501; unknown routes no longer masquerade as
unimplemented. A context-aware v2 client preserves auth and repeated queries, enforces the
existing timeouts, refuses redirects, maps refused/slow/cancelled/non-2xx calls truthfully and
wires the 030 transport, so failed writes can never look successful. Contract OPTIONS and unknown
route/method fixtures pass; the remaining 0/48 endpoint XFAILs are unchanged.

### 030 — Telemetry foundation + bounded client identity

Added opt-in gRPC/HTTP OTLP traces, metrics and correlated JSON/OTLP logs with per-Alertmanager
identity, ordered client rules and trusted forwarding-chain resolution. Wire-format tests verify
all signals, redaction, label bounds, propagation and unchanged payloads; the owned Collector/
ClickHouse pipeline proves persisted parent/child traces, matching log IDs and request metrics for
both protocols. Real collector outage preserved 100 upstream responses, with clean proxy shutdown
in 0.851s; the recorded development p99 comparison was 1.693ms off / 3.447ms on (1.753ms difference).
Full local CI passes; v1 translation and deep v2 inspection remain in their later endpoint stories.

### 025 — Confirmed upgrade contract + Docker development baseline

Operator confirmed current 0.26.0, target 0.34.1 and three instances on Kubernetes, correcting the
four-instance assumption; development should use Docker and actual client versions are not yet
known. Added a persistent isolated lab with three target members/proxies, an independent 0.26
reference, real Prometheus 2.55.1 configured for v1 and a bounded webhook receiver. Fresh-start
race-enabled checks prove membership, reference v1 delivery, v2 firing/resolved notifications and
silence gossip; full local CI passes. Production client/auth/manifests/cutover evidence remains a
mandatory 072/074 gate; v1 translation remains unimplemented and is reported as 501/XFAIL.

### 015 — Forgejo Actions CI + MVP-first roadmap

Resumed the existing contract-job commit `215882b`; retained Forgejo runs showed it had already
passed while planning stayed stale. Fresh push run 11 passed build-test, lint, integration, contract
and docker-build; isolated mutation run 10 failed integration on dropped query strings and skipped
docker-build. Local make ci and manual check parity passed; the contract's current 0/48 XFAILs
remain development scaffolding, with strict release gating assigned to new 072. Added 025/035/072/074
and moved initial cutover ahead of deep observability; shared runner configuration was not changed.

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

### 020 — v1 contract capture + diff harness

Recorded 48 v1 exchanges from a real Alertmanager v0.26.0 driven by `test/contract/scenario.yaml`,
plus a black-box harness (`make contract`) that replays them against any URL and reports
`PASS`/`FAIL`/`XFAIL`/`XPASS` per endpoint; deterministic across fresh instances (3/3) and 48/48 against
v0.26.0 itself. Checking the real system corrected the plan: `/api/v1/alerts/groups` doesn't exist in
0.26 (removed from story 050), v1 alert reads show stale silence state where v2 recomputes it, and
silence update-by-ID is in place only when the stored `startsAt` is sent back. A v1/v2 comparison
against 0.34.1 found the differences 040–070 must bridge — notably v2 rejects braced `filter`s,
returns `405` to CORS preflight, uses `[]`/`{}` where v1 had `null`, and has different error shapes.
Real legacy-client traffic couldn't be captured from here; story 030's telemetry will answer which
clients still use v1.
