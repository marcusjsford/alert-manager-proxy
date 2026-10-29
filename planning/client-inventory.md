# Client inventory and upgrade contract

**Story:** [025](stories/025-client-inventory-and-upgrade-target/plan.md) ·
**Last updated:** 2026-10-29 · **Scope:** agreed Docker development baseline; production evidence tracked below

## Confirmed production facts

| Item | Value | Evidence |
|---|---|---|
| Current Alertmanager version | **0.26.0 on every current Alertmanager** | Operator confirmation in this conversation, 2026-10-29: “all ccurrent alertmanagers are 0.26.0” |
| Selected upgrade target | **0.34.1** | Operator selected “0.34.1”, 2026-10-29 |
| Deployment platform | **Kubernetes** | Operator confirmation, 2026-10-29 |
| Alertmanager instance count | **Three**, correcting the ADR's previous four-instance assumption | Operator confirmation: “there are 3 nodes instead of 4”, 2026-10-29; physical Kubernetes worker placement is not established |
| Migration intent | All clients should reach the three-instance v2 set through the proxy before existing direct v1 APIs are deprecated | Operator confirmation, 2026-10-29; exact cutover mechanics remain to be specified |
| Legacy client versions | **Not known yet** | Operator response, 2026-10-29; this is an acknowledged gap, not an empty client estate |
| Development environment | **This is a development machine; create an isolated Docker test setup** | Operator clarification, 2026-10-29: “we're on a dev machine so we need to spin up our own for testing via docker” |

## Reproducible development baseline

See [`test/lab/README.md`](../test/lab/README.md) for the running lab and `make lab`,
`make lab-check`, `make lab-down` commands.

| Component | Pinned baseline | Purpose |
|---|---|---|
| Target cluster | Three `prom/alertmanager:v0.34.1` instances | Target-version cluster, notification and silence-gossip tests |
| Proxies | Three instances built from this repository, 1:1 | Pass-through now; translation tests as 035/040–070 land |
| Reference | Independent `prom/alertmanager:v0.26.0` | Real working v1 behaviour, without mixed-version cluster assumptions |
| Representative sender | `prom/prometheus:v2.55.1`, explicit `api_version: v1` | Real, reproducible v1-capable binary; not the unknown oldest estate client |
| Receiver | Bounded in-memory Go webhook receiver | Observe firing/resolved notifications for synthetic lab alerts |

Prometheus 2.55.1 was available locally and its real `promtool` accepted the explicit v1 config.
Its discovery API exposes the three proxy `/api/v1/alerts` URLs plus the independent reference;
the lab check requires its `LegacyClientProbe` to arrive at 0.26.0. Evaluation and alert resend
intervals are 5s. The reference is a fourth **test destination**, not a fourth target-cluster member.
Read/silence/status behaviour is initially exercised through the existing recorded HTTP scenario;
production tools and any version-gating remain to be identified before a production release.

The lab uses plain HTTP on an isolated Docker network with loopback-only published ports and no
authentication. This describes the test environment, not a decision about production TLS/auth.

## Upgrade and deployment decisions

| Requirement | Current evidence / decision | Next evidence needed | Evidence owner |
|---|---|---|---|
| Exact upgrade target | **0.34.1, operator-selected**; matches the existing test pin | Strict compatibility verification in 072 | Operator / implementation owner |
| Number of Alertmanagers | **Three, operator-confirmed** | Logical instance IDs, membership, pod/worker placement and Kubernetes Services | Operator / deployment owner to be identified |
| Existing client destinations/fanout | Not observed; ADR requires preserving per-client destinations | Sanitised client Alertmanager target configuration, including discovery/relabelling | Client owners to be identified |
| Deployment platform | **Kubernetes, operator-confirmed** | Helm values/CRDs/manifests, namespace and workload/service definitions | Deployment owner to be identified |
| Client-facing URLs/ports/path prefix | Unknown; repository loopback ports are test-only | Sanitised URLs or stable aliases, scheme, port and any prefix | Deployment owner to be identified |
| Upstream listener and proxy placement | Lab verifies 1:1 proxies; production pod/service arrangement unknown | Existing listener flags and intended proxy binding/upstream URL per instance | Deployment owner to be identified |
| TLS/authentication | Unknown | Where TLS terminates, required auth method, client/upstream certificate requirements | Deployment owner to be identified |
| Ingress and trusted forwarders | Unknown | Existing reverse proxies and their CIDRs, X-Forwarded-For handling | Deployment owner to be identified |
| Cutover mechanism | Move all clients through proxies to the three-instance v2 set before old API deprecation | Confirm parallel replacement set versus rolling upgrade; state transfer/gossip, destination changes and rollback | Operator / deployment owner to be identified |

No production endpoints have been supplied or probed. Authentication values and private payloads
are not needed in this inventory; sanitised configuration can preserve the relevant structure.

## Required client inventory

These rows are **client classes from the ADR**, not discovered applications. Their presence,
versions and owners still need confirmation for the production release gate. The separately named
lab sender above is reproducible now. Add one row per actual client once identified.

| Client class | Product/version or image | Owner | Required v1 calls | Targets/fanout | Evaluation/resend interval | Reproduction evidence |
|---|---|---|---|---|---|---|
| Legacy alert sender | Unknown; oldest real Prometheus version not identified | Unassigned | Not observed; expected POST /api/v1/alerts | Unknown | Unknown | Sanitised alerting configuration, version output and representative request needed for 040/072 |
| Alert-reading dashboard/script | Unknown; presence not confirmed | Unassigned | GET /api/v1/alerts and query flags/filters, if used | Unknown | Unknown | Product/plugin version or script revision plus example request/response needed for 050/072 |
| Silence-management client | Unknown; presence not confirmed | Unassigned | List/get/create/update/delete silences, if used | Unknown | Unknown | Product/script version, matcher/body shapes and update expectations needed for 060/072 |
| Status/receiver client | Unknown; presence not confirmed | Unassigned | GET /api/v1/status and/or /receivers, if used | Unknown | Unknown | Client version and any versionInfo/configJSON parsing or version-gating needed for 070/072 |
| Browser-based legacy UI | Unknown; presence not confirmed | Unassigned | CORS/preflight and applicable API calls | Unknown | Unknown | Origin, API base path and sanitised preflight needed for 035/072 |

## Existing protocol evidence — synthetic, not captured client traffic

| Evidence | What it establishes | What it does not establish |
|---|---|---|
| `test/contract/fixtures/v0.26.0/meta.json` | Fixtures were recorded from real Alertmanager 0.26.0 | Which clients run in production |
| `test/contract/scenario.yaml` and 48 exchange fixtures | Baseline v1 route/body/filter/error/header behaviour | Estate-specific request shapes, auth, fanout or resend behaviour |
| `test/contract/docker-compose.yml`, `test/integration/docker-compose.yml`, CI | v0.34.1 test pin; disposable local test topology | Production platform/topology; target selection is established separately by the operator above |
| `test/contract/alertmanager.yml` | Synthetic routes/receivers, inhibition and secret-masking cases | Real receiver URLs, users or credentials |

For concrete baseline examples, see `001-alerts-post-valid.json`,
`007-alerts-get-filter-braces.json`, `021-silences-post-create.json`,
`035-silences-post-update-extend.json`, `042-status-get.json` and `044-cors-preflight-alerts.json`
under `test/contract/fixtures/v0.26.0/`. Do not label these as production captures.

## Collection and follow-through

1. Operator supplies the Kubernetes configuration/inventory source and identifies deployment/client
   owners. Target release, platform and three-instance count are confirmed; named owners, concrete
   manifests and cutover mechanics are still outstanding.
2. Obtain versions from existing binary/image metadata or a read-only status/version response.
   For Prometheus, collect its `alerting.alertmanagers` configuration (including api_version,
   path_prefix, scheme, timeout, auth/TLS, discovery and relabelling) and evaluation/resend settings.
3. Obtain sanitised HTTP examples or a reproducible client script for each required client. Preserve
   method, path, query multiplicity, body fields, relevant headers and response expectations.
4. Compare those requests with the scenario. Record uncovered shapes from an isolated 0.26
   instance, then re-run fixture determinism and the development harness. No new estate-specific
   shapes have been identified yet, so fixtures/scenario have not been changed.
5. Use the verified representative sender for 040 development tests; pin actual estate client
   versions before 072/074 production validation. Arbitrary pins cannot stand in for the estate's
   oldest sender or unidentified read/silence/status tools.

## Migration and retirement boundary

The agreed destination is the three-instance Alertmanager 0.34.1 set behind 1:1 proxies. Direct
legacy API endpoints can be deprecated after clients have moved successfully and rollback is
accounted for. The proxy's v1 translation stays enabled while any migrated client still speaks v1;
switching that layer off requires the zero-v1 evidence/procedure in 100/110. Neither the existence
of a parallel target set nor rolling mixed-version cluster compatibility has been verified yet.

## Evidence boundary

Local Docker lists no running Alertmanager/Prometheus containers. The connected telemetry service
searches for `alertmanager` and `prometheus` in `production` each returned zero matches. Neither
observation rules out remote hosts, non-container services, uninstrumented services or different
service names; neither substitutes for operator confirmation.

Read-only Kubernetes checks found no matching Alertmanager pods in the reachable local
`kubernetes-admin@kubernetes` context. Its `observability/prometheus` deployment uses an unpinned
latest image and its source ConfigMap has no `alerting` block, so it was not identified as a legacy
sender. The `microk8s` and `docker-desktop` API connections were refused. The operator subsequently
clarified that development should use Docker; no Kubernetes resources were changed.

025 now delivers the operator-confirmed contract, representative reproduction and Docker lab.
Its scope was amended in response to that clarification. Actual estate client identification,
named owners, manifests/auth/path requirements and migration mechanics remain mandatory 072/074
validation, rather than blocking development or being marked verified without evidence.
