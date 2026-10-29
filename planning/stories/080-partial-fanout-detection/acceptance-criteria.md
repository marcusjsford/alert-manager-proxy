# 080 — Acceptance criteria

- [ ] Test environment with three AM + three proxies and three synthetic clients posting to 3, 2 and 1
      proxies (one of them via v2): the query reports exactly the latter two with correct counts.
- [ ] An `unknown` client posting to 1 proxy is identified by address.
- [ ] A client posting to all three but with one proxy down is **not** misreported as misconfigured
      for longer than one window (or the false positive is documented and the rule accounts for
      instance health).
- [ ] Run against real production telemetry after the 074 deployment (or later, with the date
      recorded); findings list the real partial clients found.
