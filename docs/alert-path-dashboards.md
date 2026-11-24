# Alert-path dashboards and rules

## Backend choice

The repository's development pipeline stores traces, metrics and logs in ClickHouse `otel` through the
Collector. No Grafana instance is provisioned by this repo. The committed dashboard targets
**Grafana 11 + `grafana-clickhouse-datasource`** with a datasource UID of `clickhouse`; that combination
was used to import the folder into a fresh Grafana instance during verification. The ClickHouse
database/schema is the same one used by 030/075/080.

## Dashboard

`deploy/dashboards/alert-path.json` contains one dashboard (`uid: amp-alert-path`) with an
`environment` textbox and these panels:

- alert posts by client/protocol; firing versus resolved counts
- top alertnames, last seen by client, client alert error rate
- silence operations and observed create-to-delete lifetime
- 080 partial-fanout table (embedded, environment-scoped)
- proxy server/upstream p99 latency, errors and lossy translations
- inspection-skipped counts per instance
- last-seen v1 per client/endpoint/filter (the v1-retirement view)

Every panel query was executed against real ClickHouse data from the three-proxy fanout stack.
`make dashboards-check` reports rows for every panel; the v1 panel's summed request count matched a
manual one-day raw-span count (36 = 36 at verification time).

Import with a provisioned ClickHouse datasource:

```sh
docker run -d --name amp-grafana --network amp-fanout_default -p 13000:3000 \
  -e GF_INSTALL_PLUGINS=grafana-clickhouse-datasource grafana/grafana:11.3.0
# create datasource uid=clickhouse with host=clickhouse, port=8123, database=otel,
# user=amp/password=amp-lab-only, then:
curl -u admin:admin -H 'Content-Type: application/json' \
  -X POST http://127.0.0.1:13000/api/dashboards/db \
  -d "$(jq -n --slurpfile d deploy/dashboards/alert-path.json '{dashboard:$d[0],overwrite:true}')"
```

The dashboard uses the ClickHouse datasource's `$__timeFilter` macro and the `$environment` textbox.
Create the `clickhouse` datasource with the same UID before importing.

## Rules

`deploy/rules/alert_path.yml` defines:

- **Client silent:** last-seen age exceeds **2× that client's configured expected interval** for
  5 minutes. The per-client interval prevents healthy slow senders from being reported. The rule is
  suppressed when any proxy `up{job="alertmanager-proxy"}` is down.
- **New unknown client:** unknown-client first-seen age below 5 minutes.
- **New v1 client:** v1-client first-seen age below 5 minutes.

The ClickHouse exporter is expected to expose these age gauges (computed from the dashboard's raw
queries):

- `am_proxy_client_last_seen_age_seconds{environment,client}`
- `am_proxy_client_expected_interval_seconds{environment,client}`
- `am_proxy_unknown_client_first_seen_age_seconds{environment,client_address}`
- `am_proxy_v1_client_first_seen_age_seconds{environment,client}`

`deploy/rules/alert_path_test.yml` uses `promtool test rules` to prove that a fast client becomes
silent, a client whose interval is longer than the window does **not** fire, and the unknown/new-v1
rules behave as intended.

## Limits

- Silence lifetime is the observed create-to-delete time in telemetry, not the configured `endsAt`;
  adding scheduled-expiry telemetry would be a new signal/follow-up story.
- `createdBy` may be absent when 030 redaction is enabled; client identity remains available.
- Real production traffic and the final partial-client list remain post-074 evidence; synthetic-lab
  success is not presented as production compatibility.
