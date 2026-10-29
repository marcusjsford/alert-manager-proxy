# 030 — Acceptance criteria

- [ ] With an OTel collector in the test compose, a v2 pass-through request produces a server span
      and a child upstream client span in the same trace, and the upstream request carries a
      `traceparent` header (asserted by a test upstream).
- [ ] A test with three configured client rules (CIDR, UA regex, basic-auth user) plus an unmatched
      caller yields the expected `client.name` on spans, metrics and logs for each.
- [ ] `X-Forwarded-For` is ignored unless the immediate peer is in the trusted-proxy list (test both).
- [ ] No metric label carries a raw IP, UA, query string, or matcher — asserted by a test that
      enumerates every exported metric's label keys against the ADR allowlist.
- [ ] Every log line for a request carries `trace_id`/`span_id` matching its span.
- [ ] `Authorization` never appears in any span, log or metric (test sends one and greps all exported
      telemetry).
- [ ] Telemetry visible end-to-end in the real collector → ClickHouse pipeline (query + result
      pasted into `findings.md`).
- [ ] Overhead recorded: p99 added latency of the pass-through path with telemetry on vs. off.
- [ ] With the collector unavailable, client requests still reach the upstream and return their
      normal responses; export failures are bounded/observable and shutdown remains bounded.
