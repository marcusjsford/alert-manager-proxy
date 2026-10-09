# 050 — Acceptance criteria

- [ ] Contract harness: every `GET /api/v1/alerts` fixture diffs clean, including
      each query-param variant.
- [ ] A table-driven test covers `filter` strings with `=`, `!=`, `=~`, `!~`, quoted commas, escaped
      quotes, and an invalid string (which returns the same v1 error status/body as 0.26).
- [ ] Every v1↔v2 field difference is listed in `findings.md` with its handling; each lossy one
      increments `am_proxy_translation_lossy_total` (asserted in a test).
