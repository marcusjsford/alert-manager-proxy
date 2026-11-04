# 100 — Acceptance criteria

- [x] Every panel's query runs against real data and returns results (screenshot or result table in
      `findings.md`).
      `make dashboards-check` executed all 12 panel queries against the fanout stack; row counts are
      recorded in findings.
- [x] The "last seen v1 per client" table matches a manual count from raw spans for one day.
      Panel sum of `requests` = 36; independent one-day raw v1 server-span count = 36.
- [x] The "client went silent" rule fires in a test where a synthetic client stops posting, and does
      not fire for a client whose interval is longer than the window (window choice recorded).
      `make rules-check` passes: the fast/silent client fires; the client with a 10-minute expected
      interval does not when the threshold is 2× that interval. The rule uses a per-client interval
      and `for: 5m`.
- [x] Dashboard JSON and rules are in the repo and import cleanly into a fresh instance.
      `deploy/dashboards/alert-path.json` imported with Grafana 11.3 + grafana-clickhouse-datasource;
      a real panel query executed through Grafana returned 200. `deploy/rules/alert_path.yml` and its
      rule tests pass with `promtool`.
