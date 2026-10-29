# 035 — Acceptance criteria

- [ ] Contract OPTIONS and unknown-route/method fixtures pass and their pending entries are removed.
- [ ] Shared headers/envelopes match recorded 0.26 responses on success and failure; known
      unimplemented operations still return explicit 501, without intercepting unrelated paths.
- [ ] Upstream refusal, slow response and cancellation are exercised through the shared transport;
      no failed upstream write is acknowledged as success; timeout/error mappings are recorded.
- [ ] Required authorization forwarding and redaction are verified; non-v1 pass-through integration
      and existing contract behaviour have no unexpected regressions.
