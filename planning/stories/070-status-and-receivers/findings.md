# 070 — Findings

_Append-only. Log discoveries, wrong assumptions, decisions made under ambiguity (and why), and
deviations from the plan **as they happen**, not reconstructed afterwards._

No findings recorded yet — story not started.

## 2026-10-29 — MVP replan

Real status-client evidence now comes from 025, since 020 captured no production traffic. Added
focused versionInfo verification because the contract normaliser masks that object entirely.

## 2026-10-29 — Development baseline clarified in 025

Estate client versions are unknown and the operator requested isolated Docker reproduction.
Implement/test fixture parsing and the versionInfo policy here; known clients can be exercised
when supplied, while actual estate status-client evidence remains mandatory in 072 before release.

## 2026-10-29 — Implementation and verification

### Ground truth and the config-rendering decision

Probed 0.34.1 and the recorded 0.26 fixture side by side. v2 status supplies `cluster`,
`config.original` (already YAML, with secrets replaced by `<secret>`), `uptime` and `versionInfo`; v1
uses `clusterStatus`, `configYAML`, `configJSON`, `uptime` and `versionInfo`.

The v2 `config.original` is the *target's* rendering, not 0.26's. For the contract config it adds
`smtp_tls_config`, `slack_app_url`, `rocketchat_api_url`, receiver `labels`, notifier `timeout`,
`event_recorder` and `tracing`. Loading that YAML with Alertmanager v0.34.1's own `config` package
and comparing its JSON with the 0.26 fixture showed that, after removing those target-only fields,
the remaining structure is identical (including zero-valued HTTP/TLS fields that are not present in
the YAML). That is the approach taken:

- load `config.original` with the target `config` package;
- render the YAML and JSON through that package, which masks secrets with `<secret>`;
- strip the target-only fields for v1 clients and turn an empty JSON `templates` array into `null`
  (0.26 exposed null there while its YAML had `[]`).

This gives exact contract output for the recorded config. The config package adds substantial
indirect dependencies (AWS, Kafka, Telegram, etc.); the stripped static binary grew from 19M to 22M
(~16%). That is a recorded cost of faithful config JSON rendering; the later Rust evaluation can
revisit it, and a future story could replace the dependency with a copied v0.26 type subset if the
footprint becomes material. Dropping target-only config fields emits the `status.config` lossy event.

### versionInfo policy

**Decision: report the upstream Alertmanager's real `versionInfo` (currently 0.34.1), not a
fabricated 0.26-looking object.** The proxy translates the wire contract; pretending to be 0.26
would mislead dashboards and operators about the actual server and hide upgrade state. Legacy
clients that version-gate on 0.26 remain an explicit unknown-client risk to identify and verify in
072/074. `TestStatusReportsTargetVersionInfo` asserts the real value because the contract
normaliser masks the whole object.

### Layout and receivers

- v2 `cluster` maps directly to v1 `clusterStatus` (`name`, `status`, `peers` with name/address).
- `versionInfo` and `uptime` pass through unchanged.
- v2 `receivers: [{name, labels}]` becomes the v1 `["name", ...]` list in upstream order.
- Status/receiver upstream failures use the shared truthful mapping (4xx → v1 bad_data, 5xx/transport
  → 502); no proxy identity is added to the v1 payload.

### Client evidence

Story 025 did not identify any actual status or receiver consumer. The contract fixture and focused
parsing/version-gating tests are the development evidence; estate versions, version-gating behaviour
and real client responses remain mandatory 072/074 release evidence, not an inferred compatibility
claim.

### Verification evidence

- `make contract`: all ten endpoint groups PASS, 48/48 exchanges, with an empty pending list.
- `TestGetStatusRendersLegacyConfigAndMasksSecrets`: YAML/JSON target fields removed, secrets masked
  in webhook URL, basic-auth password and Slack API URL, empty templates null.
- `TestStatusReportsTargetVersionInfo`, `TestGetReceiversTranslatesNames` and truthful upstream-error
  test pass.
- `make lab` + `make lab-check`: all three proxies expose the real `0.34.1` version and the `lab`
  receiver through the v1 endpoints, alongside the existing alert/silence checks.
- Integration now asserts the real upstream answers v1 with 410 while the proxy returns the
  translated success payload.
- `go test -race ./...`, all-tag `go vet`/lint (0 issues) and full `make ci` pass.
