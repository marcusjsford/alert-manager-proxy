# Docker compatibility development lab

The operator-selected destination is **three Alertmanager 0.34.1 instances**, with one proxy per
instance. Production is Kubernetes; this isolated Docker lab reproduces the topology locally.

```sh
make lab        # build and start the lab; wait for readiness
make lab-check  # real HTTP/cluster/notification smoke checks, with Go's race detector
make lab-down   # remove only the amp-lab project and its test data
```

The lab stays running after checks. State is ephemeral (tmpfs/in-memory) and resets when containers
are recreated. `make ci`, `make integration` and `make contract` use separate disposable projects
and ports. The existing contract fixtures require their own fixed configuration, not this lab's
notification config.

| Component | Host URL |
|---|---|
| Proxy 1 / 2 / 3 | http://127.0.0.1:19391 / :19392 / :19393 |
| Corresponding Alertmanagers | http://127.0.0.1:19291 / :19292 / :19293 |
| Proxy admin ports | http://127.0.0.1:19491 / :19492 / :19493 |
| Independent Alertmanager 0.26.0 reference | http://127.0.0.1:19226 |
| Representative Prometheus 2.55.1 | http://127.0.0.1:19290 |
| Notification receiver | http://127.0.0.1:19280/notifications |

All published ports bind to loopback. The 0.26 reference does not join the three-member cluster.
The receiver retains the latest 100 synthetic webhook notifications; it is a test utility.

## Representative legacy traffic

Prometheus **2.55.1** is deliberately pinned and configured with `api_version: v1`. It is a
reproducible v1-capable client, **not the estate's unknown oldest Prometheus**. `LegacyClientProbe`
fires every evaluation cycle; the evaluation and resend intervals are both 5s. It sends to the
three proxy endpoints plus the independent 0.26 reference, which provides the working v1 baseline.
That fourth reference destination is test-only; target-cluster fanout is three.

At story 025, proxy v1 operations still return **501**. Prometheus notification errors to the three
proxies are therefore expected. Story 040 implements alert delivery; 050–070 implement the other
v1 operations. The smoke tests prove reference delivery and exercise v2 firing/resolved webhooks
and silence gossip across the real three-member target cluster. They do not claim working v1
translation, production client compatibility or mixed-version migration compatibility.

Useful inspection commands:

```sh
docker compose -f test/lab/docker-compose.yml -p amp-lab ps
docker compose -f test/lab/docker-compose.yml -p amp-lab logs prometheus
docker compose -f test/lab/docker-compose.yml -p amp-lab logs am-1 proxy-1
docker compose -f test/lab/docker-compose.yml -p amp-lab exec prometheus \
  promtool check config /etc/prometheus/prometheus.yml
```

See [the inventory](../../planning/client-inventory.md) for confirmed operator requirements and
the production evidence still required before release/cutover.
