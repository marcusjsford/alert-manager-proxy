# 080 — Acceptance criteria

- [x] Test environment with three AM + three proxies and three synthetic clients posting to 3, 2 and
      1 proxies (one of them via v2): the query reports exactly the latter two with correct counts.
      `make fanout` + `make fanout-check` reports `fanout-two` 2/3, `fanout-one` 1/3 and omits
      `fanout-full` 3/3.
- [x] An `unknown` client posting to 1 proxy is identified by address.
      The same test reports the unmatched client as `unknown` at its span `client.address` with
      1/3 reached.
- [x] A client posting to all three but with one proxy down is **not** misreported as misconfigured
      for longer than one window (or the false positive is documented and the rule accounts for
      instance health).
      The SQL coverage CTE suppresses partial rows once a proxy's metrics age out, and the Prometheus
      rule requires all `up{job="alertmanager-proxy"}` targets to be healthy so suppression is
      immediate. This is documented in `docs/partial-fanout.md`.
- [x] Real production telemetry validation is explicitly deferred to post-074 evidence because 074
      has not run (operator feature-first ordering). 074/110 own recording real partial clients and
      their dates; the query/rule are verified against the synthetic three-instance stack now.
