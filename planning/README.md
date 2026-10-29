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

## Delivery milestones

The first release is **v1 compatibility MVP**: client/target inventory, basic telemetry, the full
recorded v1 HTTP surface, strict verification against 0.26 and the selected upgrade target, and a
rehearsed 1:1 deployment/cutover. It ends at story 074. A posting-only demonstration at 040 is an
intermediate milestone, not evidence that dashboards or silence-management clients work.

Deep v2 inspection, automated fanout detection, dashboards, continuous upstream release tracking,
v1 retirement, optional replication and the Rust evaluation follow the MVP. See the master plan's
ordered index; stable story numbers no longer determine priority.
