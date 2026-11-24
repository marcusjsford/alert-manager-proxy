# Grafana dashboard

`alert-path.json` targets a `grafana-clickhouse-datasource` with UID `clickhouse` and the
ClickHouse `otel` schema used by the project Collector. See [alert-path-dashboards.md](../../docs/alert-path-dashboards.md)
for the datasource/import example, panel list and rule/exporter contract.

The dashboard is validated by `make dashboards-check` against the three-proxy fanout stack.
