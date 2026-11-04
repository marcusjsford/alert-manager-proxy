# 090 — Opt-in alert replication

**Depends on:** 080, 100 · **Status:** Not started

## Goal

Let a proxy forward alert posts it receives to the *other* Alertmanagers, so partial-fanout clients
still get HA, without touching the clients.

## Scope

- Config: peer Alertmanager URLs (direct, **never peer proxies** — no loops); mode
  `off` (default) | `clients: [names]` | `all`.
- Pre-cutover opt-in behaviour change; implement and test it before 074, but enable it in
  production only after 080/100 evidence records the actual client need.
- Replicate after the local upstream responds; local result determines the client's response.
  Peer writes are async with bounded queue, timeout, and drop-on-full (counted), so a slow peer can't
  back-pressure clients.
- Measure: for a client already sending to all three, `all` mode means each alert is written 3× per
  instance (9 writes vs 3). Quantify Alertmanager CPU/latency impact and decide whether `all` is
  ever recommended.
- Telemetry: child spans per peer write, `am_proxy_replication_total{peer, result}`,
  queue depth gauge.

## Explicit non-goals

- Replicating silences (already gossiped) or anything other than alert posts.

## Key files

`internal/replicate/`.
