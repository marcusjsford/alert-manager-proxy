# 040 — Acceptance criteria

- [ ] Contract harness: every `POST /api/v1/alerts` fixture (valid, missing labels, past `endsAt`,
      malformed JSON) diffs clean.
- [ ] Alerts posted via v1 through the proxy are visible via `GET /api/v2/alerts` on the upstream
      with identical labels, annotations, `startsAt`, `endsAt`, `generatorURL`.
- [ ] A real legacy Prometheus (the oldest version in the estate, version recorded) configured with
      `api_version: v1` (or its implicit default) delivers firing and resolved alerts through the
      proxy to a current Alertmanager, which notifies a test receiver.
- [ ] Span carries `am.api.version=v1`, `http.route=/api/v1/alerts`, `am.alerts.count`, client
      identity; `am_proxy_alerts_received_total` increments by the alert count.
