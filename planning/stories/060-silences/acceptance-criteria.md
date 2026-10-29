# 060 — Acceptance criteria

- [x] Contract harness: every silence fixture (list, filter, get, create, update, invalid, delete,
      unknown ID) diffs clean.
      `make contract` reports 10/10 create, 4/4 list, 4/4 get-by-ID and 3/3 delete; all four
      pending entries are removed.
- [x] A silence created via v1 through the proxy on one node is visible via v2 on all three
      instances of a test cluster (proves gossip unaffected).
      `TestV1SilenceGossipAndMuting` creates on proxy-1 via v1 and waits for `GET /api/v2/silence/{id}`
      success on all three proxies.
- [x] A silence created via v1 mutes a matching alert end-to-end (test receiver gets nothing).
      The same test posts a matching alert through all three proxies, waits for `suppressed` state,
      clears prior notifications and verifies the receiver receives no notification for that run.
- [x] Spans record operation, silence ID and matchers; redaction of `createdBy`/`comment` honours
      config (tested both on and off).
      `TestV1SilenceTelemetryAndRedaction` asserts operation/id/matchers and both redaction modes;
      create/update attach request matchers, get adds the returned matchers, delete records ID.
