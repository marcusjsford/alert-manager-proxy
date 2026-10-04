# Planning

Story-driven execution tracking for `alertmanager-proxy`: a stateless proxy in front of each
Alertmanager that is **permanent infrastructure**, not a migration shim. It does two jobs:

- **Observability of the alert path** (permanent): records with OpenTelemetry exactly which client
  called what, on which protocol, with which parameters — for v1 and v2 alike — and detects clients
  that don't reach every Alertmanager.
- **v1→v2 translation** (temporary): accepts the removed Alertmanager **API v1** from legacy clients
  that can't be changed and translates each call to **API v2**, so Alertmanager can be upgraded. This
  layer is switched off once v1 usage reaches zero; the proxy stays.

- [`STORY_WORKFLOW_PROMPT.md`](STORY_WORKFLOW_PROMPT.md) — **read this first.** The working method
  (folder contract, workflow steps, git discipline, autonomous-execution policy), written as a drop-in
  prompt for an agent with no memory of how any of this got here.
- [`ADR.md`](ADR.md) — the decisions every story inherits (language, topology, contract source of
  truth, telemetry model, partial-fanout handling), with the rejected alternatives.

| Initiative | Index | What it covers |
|---|---|---|
| alertmanager-proxy | [`stories/MASTER_PLAN.md`](stories/MASTER_PLAN.md) | The Go proxy, v1 contract capture, endpoint translation, OTel for v1 and v2, partial-fanout detection/replication, upstream-release compatibility, cutover, and a measured Rust-port evaluation |

The method is carried over from the `code-indexer` project's `planning/` tree, where it was
developed; lessons quoted in the workflow prompt that originate there are labelled as such.
