# 040 — Acceptance criteria

- [x] Contract harness: every `POST /api/v1/alerts` fixture (valid, missing labels, past `endsAt`,
      malformed JSON) diffs clean.
      `make contract` reports `POST /api/v1/alerts 5/5 PASS`; its pending entry is removed. All
      five fixtures (valid, empty labels, missing labels, object-not-array, non-JSON) match.
- [x] Alerts posted via v1 through the proxy are visible via `GET /api/v2/alerts` on the upstream
      with identical labels, annotations, `startsAt`, `endsAt`, `generatorURL`.
      `TestV1PostAlertsVisibleViaV2` posts a v1 alert through the proxy to a real 0.34.1 and
      compares every field after reading it directly from v2. Timestamps compare as the same
      instant; v2's recorded millisecond truncation is reported as lossy.
- [x] The real, pinned representative Prometheus from 025, configured with `api_version: v1`,
      delivers firing and resolved alerts through the proxy to 0.34.1 and a test receiver. Record
      its version; the unknown oldest estate sender remains a separate 072/074 release requirement.
      Lab uses Prometheus **2.55.1**. `TestRepresentativeV1Client` proves v1 delivery through all
      three proxies; `TestV1FiringAndResolvedThroughProxy` toggles a real Prometheus rule and
      receives both firing and resolved webhooks from the 0.34.1 cluster.
- [x] Span carries `am.api.version=v1`, `http.route=/api/v1/alerts`, `am.alerts.count`, client
      identity; `am_proxy_alerts_received_total` increments by the alert count.
      `TestV1PostAlertsTelemetry` exports and asserts those span attributes, the bounded
      `am.alerts.names` span-only list, and a metric point of 2 with the bounded labels.
- [x] Failed/timed-out upstream alert writes do not return v1 success; retry/recovery behaviour is
      exercised and this endpoint's pending entry is removed once all its contract cases pass.
      Tests prove upstream 500 → v1 502, timeout → 504, a client retry can then return 200, and no
      failed v2 decode/validation path reaches the upstream. Pending entry is removed.
