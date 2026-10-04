# 075 — Acceptance criteria

- [ ] For each v2 endpoint in scope, a test asserts the upstream receives byte-identical request bodies
      and the client receives byte-identical response bodies, with and without inspection enabled.
- [ ] For each v1 endpoint that has a v2 equivalent, a v1 call and the matching v2 call produce the
      same span attribute keys (test enumerates both), differing only in `am.api.version`.
- [ ] `am_proxy_alerts_received_total` counts v2 posts with `api_version="v2"`.
- [ ] An oversized body and a malformed body are both forwarded unchanged and increment the
      skipped-inspection counter.
- [ ] Added p99 latency of v2 `POST /api/v2/alerts` with inspection on vs. off recorded in `findings.md`.
