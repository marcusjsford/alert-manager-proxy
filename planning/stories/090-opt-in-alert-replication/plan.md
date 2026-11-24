# 090 — Opt-in alert replication

**Depends on:** 074, 080, 100 · **Status:** Not started

## Goal

Let a proxy forward alert posts it receives to the *other* Alertmanagers, so partial-fanout clients
still get HA, without touching the clients.

## Trigger

This story is **evidence-gated and post-074**. 074 must first record the production client/fanout
data that 080 detects and 100 displays:

- If **no partial clients** are found, record the evidence, close this story as not needed, and
  change no traffic path. Detection and dashboards remain the permanent answer.
- If **partial clients** are found, implement the feature below. Production enablement still needs
  operator approval and the measured need; the default stays `off`.

Do not implement speculative replication before that evidence exists. It is not a prerequisite for
the initial cutover.

## Scope

- Config: peer Alertmanager URLs (direct, **never peer proxies** — no loops); mode
  `off` (default) | `clients: [names]` | `all`.
- Decide how an **unknown** client (080 reports these by address, not name) can be opted in without
  turning on `all` for everyone; record the decision. Address-based selection or an explicit
  "unknown requires `all`" limitation are both acceptable if documented and tested.
- Replicate after the local upstream responds; local result determines the client's response.
  Peer writes are async with bounded queue, timeout, and drop-on-full (counted), so a slow peer can't
  back-pressure clients.
- Measure: for a client already sending to all three, `all` mode means each alert is written 3× per
  instance (9 writes vs 3). Quantify Alertmanager CPU/latency impact and decide whether `all` is
  ever recommended; `clients:` is the expected production mode.
- Telemetry: child spans per peer write, `am_proxy_replication_total{peer, result}`, queue depth
  gauge. Record partial-client evidence from 074 in `findings.md` before enabling anything.

## Explicit non-goals

- Replicating silences (already gossiped) or anything other than alert posts.
- Enabling replication in production without recorded partial-client evidence and operator approval.

## Key files

`internal/replicate/`.
