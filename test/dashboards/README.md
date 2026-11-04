# Dashboard query check

Starts from the same dedicated three-proxy stack as 080:

```sh
make fanout
make dashboards-check
```

The check seeds alerts (v1/v2), errors, a silenced alert, create/delete silence spans and oversized
inspection data, then executes every `rawSql` panel in `deploy/dashboards/alert-path.json` against
ClickHouse. It fails if any panel returns zero rows. It does not run in the normal `make ci` because
it needs the persistent development/fanout stack.
