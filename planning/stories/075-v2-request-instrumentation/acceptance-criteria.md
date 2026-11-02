# 075 — Acceptance criteria

- [x] For each v2 endpoint in scope, a test asserts the upstream receives byte-identical request bodies
      and the client receives byte-identical response bodies, with and without inspection enabled.
      `TestV2InspectionPreservesRequestsAndResponses` covers alert/silence POST, list/get and delete.
- [x] For each v1 endpoint that has a v2 equivalent, a v1 call and the matching v2 call produce the
      same span attribute keys (test enumerates both), differing only in `am.api.version`.
      `TestV2InspectionSpanKeyParityWithV1` compares the application `am.*` keys; automatic otelhttp
      body-size attributes are excluded because v1/v2 wire bodies legitimately differ.
- [x] `am_proxy_alerts_received_total` counts v2 posts with `api_version="v2"`.
      The preservation/counter test asserts the v2 count; the parity test confirms the same alert
      attributes as v1.
- [x] An oversized body and a malformed body are both forwarded unchanged and increment the
      skipped-inspection counter.
      Unit and telemetry tests cover both and assert `am_proxy_inspection_skipped_total`.
- [x] Added p99 latency of v2 `POST /api/v2/alerts` with inspection on vs. off recorded in
      `findings.md`.
      Recorded: off 5.146 ms, on 9.656 ms, added 4.510 ms (1000 interleaved samples).
