# 060 — Acceptance criteria

- [ ] Contract harness: every silence fixture (list, filter, get, create, update, invalid, delete,
      unknown ID) diffs clean.
- [ ] A silence created via v1 through the proxy on one node is visible via v2 on all four nodes of a
      test cluster (proves gossip unaffected).
- [ ] A silence created via v1 mutes a matching alert end-to-end (test receiver gets nothing).
- [ ] Spans record operation, silence ID and matchers; redaction of `createdBy`/`comment` honours
      config (tested both on and off).
