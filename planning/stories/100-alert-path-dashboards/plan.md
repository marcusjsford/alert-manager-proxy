# 100 — Alert-path dashboards

**Depends on:** 075, 080 · **Status:** Done

## Goal

The permanent operational view of the alert path that justifies keeping the proxy: who sends what to
which Alertmanager, on which protocol, how reliably — plus the view that decides when the v1 layer
can be switched off.

## Scope

- Dashboards over the ClickHouse `otel` database (Grafana or whatever the existing pipeline uses —
  confirm):
  - **Clients:** per-client alert post rate, firing/resolved split, top alertnames, last seen,
    protocol version, error rate.
  - **Silences:** who creates/expires silences, with what matchers, how long they last.
  - **Fanout:** 080's partial-fanout table and per-client instance coverage.
  - **Proxy health:** upstream latency, added latency, error and lossy-translation rates,
    skipped-inspection rate, per instance.
  - **v1 retirement:** last-seen-v1 per client, endpoint/parameter breakdown; the evidence for
    flipping `v1.enabled` off.
- Rules: new unknown client appears; a known client goes silent (stopped sending alerts entirely is
  itself an incident); new v1 client appears.
- Dashboard JSON and rules committed to the repo.

## Explicit non-goals

- New telemetry signals — if a panel needs one, that's a finding and a follow-up story.

## Key files

`deploy/dashboards/`, `deploy/rules/`.
