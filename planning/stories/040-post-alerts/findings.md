# 040 — Findings

_Append-only. Log discoveries, wrong assumptions, decisions made under ambiguity (and why), and
deviations from the plan **as they happen**, not reconstructed afterwards._

No findings recorded yet — story not started.

## 2026-10-29 — MVP replan

025 identifies the real legacy sender; 035 now owns shared envelopes/routing/headers/transport.
040 is the sender-delivery milestone, while 074 remains the full compatibility release boundary.
Added explicit failed-write acknowledgement and pending-entry removal criteria.

## 2026-10-29 — Docker development baseline confirmed in 025

The operator requested a local Docker lab; actual client versions are unknown. 040's development
sender is the real, pinned Prometheus 2.55.1 configured for v1 in that lab. Oldest-estate-client
validation remains a separate mandatory 072/074 release gate, not an inferred compatibility claim.

## 2026-10-29 — Implementation and verification

### Ground truth: v0.26 POST behaviour and 0.34.1 v2 differences

Probed the running lab reference (0.26.0) and target (0.34.1) directly before implementing:

- 0.26 accepts `[]` and an alert with only one non-`alertname` label, and accepts `annotations:null`.
  Missing/empty/null `labels` → `400 bad_data "at least one label pair required"`.
- A `null` array element crashes 0.26 with an empty reply (the process panics). The proxy rejects it
  explicitly with `400 bad_data "alert must not be null"` instead of reproducing a crash.
- Invalid times use Go's time parser message, e.g. `parsing time "bad" as
  "2006-01-02T15:04:05Z07:00": cannot parse "bad" as "2006"`.
- v2 rejects a missing `labels` field with `422 {"code":602,"message":"0.labels in body is
  required"}` but an empty labels object with `400 "at least one label pair required"`. It also
  returns `{"code":400,"message":"parsing alerts body ..."}` for malformed JSON. Because 040
  decodes and validates first, those v2 bodies are only fallbacks; a valid v1 list reaches v2 as a
  valid `PostableAlerts` array.
- v2 accepts past `endsAt` (already resolved), missing times (defaults to now/now+resolve_timeout,
  matching v1), empty `generatorURL`, and empty annotations.
- v2 stores timestamps at millisecond precision; 0.26 used nanoseconds. A request containing
  sub-millisecond values records `am.translation.lossy` with field `alert.timestamps`, reason
  `millisecond precision`.

To match 0.26's exact decode errors without hand-writing strings, the v1 wire type lives in
packages deliberately named `model` and `types`:

- `internal/v1/model` defines `LabelSet`/`LabelName`/`LabelValue`.
- `internal/v1/types` defines `Alert`.

That makes `encoding/json` produce the recorded
`[]*types.Alert` and `model.LabelValue` type names. The fixture and unit tests assert the exact
object-not-array, non-JSON and non-string-label strings.

### Translation and error decisions

`internal/v1/alerts_post.go` decodes `[]*types.Alert`, rejects empty labels and null elements, maps
to `[]postableAlert`, forwards safe headers (including `Authorization`) and sends
`POST /api/v2/alerts` through 035's shared client. Missing times are omitted so v2 applies the same
defaults; labels, annotations, present timestamps and a non-empty `generatorURL` are preserved.
An empty array is forwarded and returns v1 success, matching 0.26.

- v2 4xx responses become `400 bad_data`, using a bare JSON string or an object's `message` when
  present; otherwise the message is `upstream Alertmanager returned <status>`.
- v2 5xx, transport refusal and timeouts use 035's mappings (502/504) and never return success.
- The proxy does **not** retry writes itself. The acceptance test performs the client retry:
  first upstream 500 → v1 502, then the same request → v1 200. Retrying inside the proxy would
  duplicate alert deliveries and is explicitly out of scope.

### Telemetry additions

`Runtime.SpanParameters` was added so request content can be span-only. POST alerts uses it for
`am.alerts.names` (deduplicated, at most 32 names, each truncated to 128 runes) while
`Runtime.Alerts` continues to add `am.alerts.count` to span and access log and increments
`am_proxy_alerts_received_total` with the existing bounded labels. The metric counts parsed alerts
before delivery, so it remains a received-volume signal even when the upstream write fails. The
timestamp lossy event is emitted once per request when any supplied time has sub-millisecond
precision.

### Real sender and receiver test

The lab Prometheus remains the real **2.55.1** binary configured with `api_version: v1`, explicitly
not an estate version claim. Its existing `LegacyClientProbe` now arrives through all three proxies;
the lab test verifies that via v2 reads. A second, test-only gauge served by the receiver
(`amp_lab_probe`, toggled through `POST /probe`) drives `LegacyResolveProbe`. The test forces a
resolved state, clears retained notifications, sets the probe to 1, waits for firing in Prometheus/
Alertmanager/the receiver, then sets it to 0 and waits for the resolved webhook. This exercises the
real sender → v1 POST → proxy → v2 POST → 0.34.1 cluster → receiver path for both transitions.

### Verification evidence

- `make contract`: `POST /api/v1/alerts 5/5 PASS`; other endpoint groups remain XFAIL and OPTIONS/
  unknown routes remain PASS.
- `make integration`: pass-through plus `TestV1PostAlertsVisibleViaV2` against a real 0.34.1.
- `make lab` + `make lab-check`: cluster/version checks, real Prometheus v1 delivery to all proxies,
  v1 firing/resolved webhooks, v2 notifications and silence gossip all pass.
- `go test -race ./...`, all-tag `go vet`/lint (0 issues) and full `make ci` pass.
- Telemetry test verifies span identity/route/count/names, lossy event, metric count/labels, and
  that alertnames do not appear in the access log.
