# Partial-fanout detection

`deploy/queries/partial_fanout.sql` and `deploy/rules/partial_fanout.yml` detect clients that post
alerts to fewer than the expected Alertmanager instances. Alertmanager gossips silences and the
notification log, not alerts, so a partial client loses alerts when its target node is down.

## Query

The query uses the 040/075 `am_proxy_alerts_received_total` metric for named clients and server spans
for `cluster.name="unknown"` clients, grouped by `client.address`. It covers v1 and v2 alert posts.

The default window is **10 minutes**, chosen to be much larger than normal Prometheus evaluation and
resend intervals. If an estate has a longer update interval, raise the window and the rule's `for`
duration to at least twice that interval.

The expected count is configured in the SQL `expected` CTE (3 for `production` and `fanout-test`).
Keep the corresponding `am_proxy_client_instances_expected` gauge in sync. A client is reported only
when `instances_reached < expected`.

## Proxy outage behaviour

The query joins a `coverage` CTE that counts distinct proxy instances reporting any request metric
inside the window. If an expected proxy is down or not reporting telemetry, its last metrics age out
after at most the window and **no partial-fanout rows are returned**. The Prometheus rule additionally
requires every `up{job="alertmanager-proxy"}` series to be 1, so a proxy outage suppresses the
partial-fanout alert immediately rather than waiting for the SQL window to expire. A separate proxy
availability alert should cover the outage itself.

## Exporter and rule

The SQL result is not itself a Prometheus metric. Run the query through a small scheduled exporter
(or the existing telemetry query bridge) and expose:

- `am_proxy_client_instances_reached{environment,client,client_address}`
- `am_proxy_client_instances_expected{environment}`

The rule suppresses partial-fanout alerts while any expected proxy is down. Production validation
of the exporter, the actual client list and the real partial clients happens after the 074 deployment,
as required by story 080.
