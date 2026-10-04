# 080 — Acceptance criteria

- [ ] Test environment with four AM + four proxies and three synthetic clients posting to 4, 2 and 1
      proxies (one of them via v2): the query reports exactly the latter two with correct counts.
- [ ] An `unknown` client posting to 1 proxy is identified by address.
- [ ] A client posting to all four but with one proxy down is **not** misreported as misconfigured
      for longer than one window (or the false positive is documented and the rule accounts for
      instance health).
- [ ] Run against real production telemetry once proxies are deployed (may be after 110 — if so,
      mark this box with the date run); findings list the real partial clients found.
