-- Partial-fanout detection over the development/production OTel ClickHouse
-- pipeline. A window of 10m is deliberately much larger than normal
-- Prometheus evaluation/resend intervals; adjust it if estate intervals are
-- longer (the rule's `for` duration must cover at least two client intervals).
--
-- The expected peer count is configured per environment here. Keep this in
-- sync with the value exported as am_proxy_client_instances_expected for the
-- Prometheus rule (see deploy/rules/partial_fanout.yml).
--
-- Known clients come from the alert counter emitted by 040/075. Unknown clients
-- are split by client.address from server spans because their metric label is
-- the single bounded value "unknown". Coverage requires every expected proxy
-- instance to be reporting request telemetry; while a proxy is down the query
-- returns no partial-fanout rows, so the false positive is bounded by the
-- window and suppressed rather than blaming correctly-fanned-out clients.
WITH
    now() - INTERVAL 10 MINUTE AS window_start,
    expected AS (
        SELECT 'fanout-test' AS environment, 3 AS expected
        UNION ALL
        SELECT 'production' AS environment, 3 AS expected
    ),
    coverage AS (
        SELECT
            ResourceAttributes['deployment.environment.name'] AS environment,
            uniqExact(ResourceAttributes['service.instance.id']) AS observed_instances
        FROM otel.otel_metrics_sum
        WHERE MetricName = 'am_proxy_requests_total'
          AND TimeUnix >= window_start
        GROUP BY environment
    ),
    known AS (
        SELECT
            ResourceAttributes['deployment.environment.name'] AS environment,
            Attributes['client'] AS client,
            '' AS client_address,
            uniqExact(ResourceAttributes['service.instance.id']) AS instances_reached
        FROM otel.otel_metrics_sum
        WHERE MetricName = 'am_proxy_alerts_received_total'
          AND TimeUnix >= window_start
          AND Attributes['endpoint'] IN ('/api/v1/alerts', '/api/v2/alerts')
          AND Attributes['method'] = 'POST'
          AND Attributes['client'] != 'unknown'
        GROUP BY environment, client
    ),
    unknown AS (
        SELECT
            ResourceAttributes['deployment.environment.name'] AS environment,
            'unknown' AS client,
            SpanAttributes['client.address'] AS client_address,
            uniqExact(ResourceAttributes['service.instance.id']) AS instances_reached
        FROM otel.otel_traces
        WHERE ServiceName = 'alertmanager-proxy'
          AND SpanKind = 'Server'
          AND Timestamp >= window_start
          AND SpanAttributes['http.route'] IN ('/api/v1/alerts', '/api/v2/alerts')
          AND SpanAttributes['http.request.method'] = 'POST'
          AND SpanAttributes['client.name'] = 'unknown'
          AND SpanAttributes['client.address'] != ''
        GROUP BY environment, client_address
    )
SELECT
    partial.environment,
    partial.client,
    partial.client_address,
    partial.instances_reached,
    expected.expected,
    coverage.observed_instances
FROM (
    SELECT * FROM known
    UNION ALL
    SELECT * FROM unknown
) AS partial
INNER JOIN expected USING environment
INNER JOIN coverage USING environment
WHERE partial.instances_reached < expected.expected
  AND coverage.observed_instances = expected.expected
ORDER BY partial.client, partial.client_address
FORMAT JSON;
