# 035 — Acceptance criteria

- [x] Contract OPTIONS and unknown-route/method fixtures pass and their pending entries are removed.
      `make contract` reports `OPTIONS /api/v1/* 2/2 PASS` and `unknown v1 routes 3/3 PASS`; only
      those two lines were removed from `test/contract/pending.txt`.
- [x] Shared headers/envelopes match recorded 0.26 responses on success and failure; known
      unimplemented operations still return explicit 501, without intercepting unrelated paths.
      Envelope/router tests assert exact JSON bytes, contract headers, empty preflight and the
      plain-text 404/405 bodies. All known endpoints remain 501; `/api/v1x` still passes through.
- [x] Upstream refusal, slow response and cancellation are exercised through the shared transport;
      no failed upstream write is acknowledged as success; timeout/error mappings are recorded.
      Upstream tests cover refused dial, response-header timeout, cancellation, non-2xx, redirects
      and failed writes; mappings are recorded in `findings.md`.
- [x] Required authorization forwarding and redaction are verified; non-v1 pass-through integration
      and existing contract behaviour have no unexpected regressions.
      Tests prove Authorization/custom headers reach v2 and logs omit auth, query, URL credentials
      and raw transport errors; `make integration` and the full `make ci` pass, with only the
      endpoint-owned pending entries still XFAIL.
