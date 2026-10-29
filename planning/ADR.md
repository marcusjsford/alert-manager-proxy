# alertmanager-proxy — Architecture Decision Record

**Status:** accepted 2026-10-04 (story [010](stories/010-skeleton-and-passthrough/)) ·
**Scope:** the whole initiative

Each decision records what was chosen, what it optimises for, and what was rejected — the rejected
options are the useful half, because a decision whose alternatives aren't written down gets
re-litigated by the next person with the same idea.

## Context

Alertmanager removed API v1 in **0.27.0** (deprecated since the v2 API landed). A set of legacy
clients — old Prometheus servers, scripts, dashboards — speak only v1 and cannot be changed, which
pins the three-instance Alertmanager cluster to 0.26.0. The goal is to unblock upgrading Alertmanager by
putting a translation layer in front of it.

**Operator confirmation (2026-10-29, story 025):** all current Alertmanagers run 0.26.0; the chosen
target is 0.34.1 on Kubernetes with three instances, correcting the original four-node assumption.
Clients move through the proxy to the three-instance v2 set before existing direct v1 endpoints
are deprecated. Exact Kubernetes resources, client versions, auth/path requirements and parallel-
set versus rolling cutover mechanics remain unverified; see [client inventory](client-inventory.md).

The same position in the request path gives something Alertmanager doesn't offer itself: per-client,
per-call visibility of everything that writes alerts and silences. That is valuable independently of
v1, so the proxy is permanent (decision 6) and only its v1 layer is temporary.

---

## Decision 1 — Go first; a Rust port is a measured, later experiment

**Chosen:** Go, as a single static binary.

**Optimises for:** translation fidelity. Alertmanager is Go: the proxy imports the upstream v2
client/models and — most importantly — Alertmanager's own matcher parser (`pkg/labels`), so v1
`filter=` strings and silence matchers parse *exactly* as Alertmanager parses them. A
reimplemented parser is where drift would most likely live. v1 response types are copied from the
`v0.26.0` tag (Apache-2.0) into an internal package, since one module can't import two versions of
the same dependency. The OTel Go SDK (`otelhttp` server+client, `otelslog` bridge) covers all three
signals.

Because the proxy is permanent (decision 6), its footprint and added latency are a standing cost
on every alert, which is what makes the Rust evaluation worth doing at all.

**Rust, later (story 120):** once the Go proxy is in production and the contract harness (story 020)
is complete, port to Rust and keep it only if it measurably wins on RSS, p99 added latency, CPU per
request, and image size against the Go baseline, while passing the identical harness. This is why
story 020's harness must be a **black-box HTTP harness, language-agnostic**, not Go unit tests — it
is the port's acceptance test.

**Rejected — Rust from the start:** every Alertmanager type and the matcher grammar would need
hand-porting before there's a correct reference to diff against. Doing Go first produces exactly that
reference.
**Rejected — Python:** neither upstream-type reuse nor footprint advantages; adds a runtime for a
small shim.

## Decision 2 — Topology: one proxy per Alertmanager, stateless

**Chosen:** a proxy instance in front of each of the three Alertmanagers (Kubernetes, same pod or
equivalent per-instance placement, 1:1). Clients that fan out to all three Alertmanagers target all
three proxies instead; actual client destinations and service discovery must be inventoried. Each proxy translates
v1 and transparently passes through everything else (`/api/v2/*`, UI, `/-/healthy`, `/metrics` of
its own on a separate port) to *its* Alertmanager.

**Optimises for:** preserving today's HA semantics exactly. Prometheus-style clients already fan out
to every Alertmanager; a 1:1 proxy changes nothing about that, adds no shared state, and losing one
proxy is the same failure as losing its Alertmanager.

**Rejected — a single shared proxy fanning out to all three:** introduces a new single point of
failure and changes write semantics for every client, including the well-configured majority, to fix
a minority problem that decision 5 handles more narrowly.

## Decision 3 — The v1 contract is what Alertmanager 0.26 actually returns

**Chosen:** the source of truth for v1 behaviour is a corpus of request/response fixtures recorded
from a real Alertmanager **v0.26.0** (story 020), replayed against proxy → current Alertmanager with
a normalising diff (timestamps, fingerprints, generated IDs). Not the v1 docs, not memory.

**Rejected — implement from the source code/spec alone:** response envelopes
(`{"status":"success","data":…}`), error shapes, and field ordering/nullability are where legacy
clients break, and they're easy to get subtly wrong by reading.

## Decision 4 — Telemetry model

**Chosen:** OTLP export of traces, metrics and logs (to the existing collector → ClickHouse pipeline
used by `code-indexer`, `nuc01`), with:

- **Client identity** resolved by a config map (source CIDR, `User-Agent` pattern, basic-auth user,
  optional header) to a **bounded** `client.name`; unmatched → `unknown`. Raw `client.address` and
  `user_agent.original` always recorded on spans/logs.
- **Every proxy instance identifies its own Alertmanager** via `service.instance.id` — needed by
  decision 5.
- **Parameters on spans/logs, never on metric labels.** Metric labels are limited to `client`,
  `api_version`, `endpoint` (route template), `method`, `status_code` and `instance`.
- **v1 and v2 get the same depth.** Request-level attributes (alert count and alertnames, silence
  operation/ID/matchers, query filters) are extracted for v2 pass-through calls as well as v1 ones —
  after v1 retires, v2 is all the proxy sees (story 075).
- **Lossy translation is observable**: a span event plus a counter whenever a field is dropped,
  defaulted, or approximated.

## Decision 5 — Clients that don't send to all three Alertmanagers: detect first, replicate opt-in

Alertmanager clustering gossips **silences and the notification log, not alerts** — so a client
that posts to only one Alertmanager loses its alerts if that node is down. Today nothing notices.

**Chosen:** two separate steps.
1. **Detect (story 080):** per client, count distinct proxy instances that received alert posts
   within a window; anything below the configured peer count is reported (metric + query + alert
   rule). No behaviour change.
2. **Replicate, opt-in (story 090):** a proxy can be configured to forward received alert posts to
   the *other* Alertmanagers directly (never to other proxies — no loops). Re-posting an alert
   Alertmanager already holds is merged by fingerprint, so a client that already sends to all three
   gets redundant but harmless writes; the story must measure that cost and decide whether to
   replicate only for clients detected as partial.

**Rejected — fix it silently by always fanning out:** changes write volume ×3 for everyone and hides
the misconfiguration instead of surfacing it.

## Decision 6 — The proxy is permanent; the v1 layer is what retires

**Chosen:** the proxy stays in the alert path after the Alertmanager upgrade, for its observability
(decisions 4 and 5). The v1 translation layer is a separately switchable component (`v1.enabled`)
that is turned off once the v1 usage view (story 100) shows no traffic for an agreed period —
without redeploying topology or repointing clients.

**Consequences to hold to:**
- **Pass-through must be a first-class path**, not a fallback: same telemetry depth (decision 4),
  same test rigour, and it must not rewrite v2 requests or responses.
- **It must track Alertmanager releases.** A permanent proxy has to keep working as Alertmanager's
  v2 API evolves; 072 establishes the strict selected-target upgrade gate, and 105 extends it to
  new upstream releases. Every upstream upgrade requires a green strict run for its exact target.
- **It is part of the alert path's availability.** A proxy failure is an alert-delivery failure for
  its node. The 1:1 topology (decision 2) bounds the blast radius to one node — the same as losing
  that Alertmanager — but the proxy needs its own health checks, resource limits and alerting
  (story 074 initially, extended by 110), and must never be the reason a node is down when its
  Alertmanager is up.

**Rejected — remove the proxy after cutover:** the telemetry it provides (who sends what, partial
fanout) has no other source; Alertmanager's own metrics don't attribute requests to clients.
