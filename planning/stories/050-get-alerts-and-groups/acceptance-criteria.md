# 050 — Acceptance criteria

- [x] Contract harness: every `GET /api/v1/alerts` fixture diffs clean, including
      each query-param variant.
      `make contract` reports `GET /api/v1/alerts 15/15 PASS`; the pending entry is removed. The
      silence state those fixtures need is now created/deleted via v2 setup, so 050 does not depend
      on 060.
- [x] A table-driven test covers `filter` strings with `=`, `!=`, `=~`, `!~`, quoted commas, escaped
      quotes, and an invalid string (which returns the same v1 error status/body as 0.26).
      `internal/v1/filter/filter_test.go` covers all of those plus escaped newlines and exact
      `bad matcher format: bad`; the contract's invalid-filter fixture also passes.
- [x] Every v1↔v2 field difference is listed in `findings.md` with its handling; each lossy one
      increments `am_proxy_translation_lossy_total` (asserted in a test).
      The difference table is in `findings.md`. Timestamp precision is the lossy difference;
      `TestV1GetAlertsTelemetry` asserts the span event and exactly one counter increment.
