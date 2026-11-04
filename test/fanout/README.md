# Partial-fanout test stack

An isolated three-Alertmanager/three-proxy stack with the project OTel Collector and ClickHouse. It
exercises `deploy/queries/partial_fanout.sql` with real metrics and spans.

```sh
make fanout        # build and start loopback-only test stack
make fanout-check  # client fanout 3/2/1, one v2 post, one unknown client, then query ClickHouse
make fanout-down   # remove the stack and its data
```

Host ports: proxies `19691`–`19693`, ClickHouse HTTP `19623`. The proxies identify clients by the
`FanoutFull/`, `FanoutTwo/` and `FanoutOne/` user agents configured in `test/telemetry/proxy.yml`;
an unmatched `FanoutUnknown/` client exercises address-based unknown detection.

The query intentionally returns rows only when all three proxy instances are reporting telemetry.
While a proxy is down, partial-fanout findings are suppressed rather than blaming correctly
configured clients; the Prometheus rule also requires all `up{job="alertmanager-proxy"}` series.
