# Legacy v1 client probes

`v1_client.py` is a dependency-free Python 3 client for the Alertmanager v1 API. It exercises the
client classes recorded in the story 025 inventory:

- alert sender (firing and already-resolved alerts)
- alert reader (`filter` with and without braces, `receiver` regex, `silenced=false`, timestamps)
- silence manager (create, list/filter, get, in-place extension, matcher-change replacement,
  suppression, gossip, delete and unknown ID)
- status/config/versionInfo and receivers readers
- routing/error/preflight contract

It is a **representative** client, not the estate's unknown oldest sender. Actual estate client
versions and production end-to-end evidence are owned by story 074.

Run against the three-proxy lab and its notification receiver:

```sh
make v1-client-check
```

Or target one endpoint, a prefixed/auth deployment, or TLS:

```sh
python3 test/clients/v1_client.py --url http://127.0.0.1:19391
python3 test/clients/v1_client.py --url https://am.example/ \
  --prefix /prefix --header 'Authorization: Bearer ...' --insecure
```

`--url` may be repeated; writes go to the first URL and every other URL is polled for silence gossip.
`--receiver` enables the receiver-muting check when the receiver supports `DELETE /notifications`
(the lab receiver does). `--json-out` records the summary for release evidence. The script exits
non-zero on the first failed assertion and best-effort deletes its probe silences.

`make v1-client-syntax` only compiles the script, so it can run where Docker/lab access is absent.
