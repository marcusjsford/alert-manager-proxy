-- Standard ClickHouse exporter schema (database name is deployment-configurable).
-- Per-client request/last-seen/error/latency view. Only server spans count requests.
SELECT
    ResourceAttributes['deployment.environment.name'] AS environment,
    ResourceAttributes['service.instance.id'] AS instance,
    SpanAttributes['client.name'] AS client,
    SpanAttributes['am.api.version'] AS api_version,
    SpanAttributes['http.route'] AS endpoint,
    count() AS requests,
    countIf(toUInt16OrZero(SpanAttributes['http.response.status_code']) >= 400
            OR SpanAttributes['am.request.aborted'] = 'true') AS errors,
    quantile(0.99)(Duration) / 1e9 AS p99_seconds,
    max(Timestamp) AS last_seen
FROM otel.otel_traces
WHERE ServiceName = 'alertmanager-proxy'
  AND SpanKind = 'Server'
  AND Timestamp >= now() - INTERVAL 15 MINUTE
GROUP BY environment, instance, client, api_version, endpoint
ORDER BY last_seen DESC;

-- Latest cumulative request counter by stream, not a sum of repeated samples.
SELECT
    ResourceAttributes['deployment.environment.name'] AS environment,
    ResourceAttributes['service.instance.id'] AS instance,
    Attributes['client'] AS client,
    Attributes['api_version'] AS api_version,
    Attributes['endpoint'] AS endpoint,
    Attributes['method'] AS method,
    Attributes['status_code'] AS status_code,
    StartTimeUnix AS stream_start,
    argMax(Value, TimeUnix) AS latest_counter
FROM otel.otel_metrics_sum
WHERE ServiceName = 'alertmanager-proxy'
  AND MetricName = 'am_proxy_requests_total'
  AND TimeUnix >= now() - INTERVAL 15 MINUTE
GROUP BY environment, instance, client, api_version, endpoint, method, status_code, stream_start;

-- Recent correlated access logs, including raw addresses only in logs/spans.
SELECT Timestamp, TraceId, SpanId,
       ResourceAttributes['deployment.environment.name'] AS environment,
       ResourceAttributes['service.instance.id'] AS instance,
       LogAttributes['client.name'] AS client,
       LogAttributes['client.address'] AS address,
       LogAttributes['am.api.version'] AS api_version,
       LogAttributes['http.route'] AS endpoint,
       LogAttributes['status_code'] AS status_code
FROM otel.otel_logs
WHERE ServiceName = 'alertmanager-proxy' AND Body = 'request completed'
ORDER BY Timestamp DESC
LIMIT 100;
