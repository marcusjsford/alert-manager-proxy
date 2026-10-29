# 060 — Findings

_Append-only. Log discoveries, wrong assumptions, decisions made under ambiguity (and why), and
deviations from the plan **as they happen**, not reconstructed afterwards._

No findings recorded yet — story not started.

## 2026-10-29 — MVP replan

Removed stale assumptions that isEqual/casing were unknown and every update replaces the ID;
020 already recorded those behaviours. Reuse 035's HTTP helpers and 025's real client evidence.

## 2026-10-29 — Implementation and verification

### Ground truth from v0.26 and the chosen 0.34.1 target

Read the recorded v1 implementation rather than inferring silence semantics:

- v1 decodes matchers with `labels.Matchers`, whose `UnmarshalJSON` defaults missing `isEqual` to
  true and compiles regex matchers anchored. That is why an invalid regex fails during decode with
  `error parsing regexp: missing closing ): `^(?:()$``, before the silence store is touched.
- v1 checks `startsAt == endsAt` first, then `endsAt` in the past, then the store's validation
  (at least one matcher, not all matching the empty string, nonzero timestamps, end not before
  start). The error strings are reproduced exactly and in that order.
- v1 create/update uses the store's `Set`: an in-place update preserves the ID only when matchers are
  identical and the allowed timestamp changes hold; otherwise the old silence expires and a new UUID
  is returned. The v2 store implements the same rule. The fixtures confirm both paths: the extend
  keeps `362e...`, changing matchers returns `db67...`.
- v2 uses `silenceID`; the proxy emits v1's `data.silenceId`. v2 returns 404 for an unknown
  get/delete, 422 for a malformed path UUID, and 404 `"silence not found"` for an unknown update.
  These map to v1's plain-text 404 get, 400 `bad_data "silence not found"` delete/update.
- v1 lists active/pending/expired groups, active by `endsAt` ascending and expired by `endsAt`
  descending. The proxy sorts into that order itself. v0.26's pending comparator was inconsistent;
  the proxy uses `startsAt` ascending (the intended/v2 order). There are no pending fixtures, and the
  plan records this as the chosen sane behaviour.
- v1 list filtering is *label-value matching* against the silence's matcher map, not v2's exact
  matcher-set membership check. To preserve v1 semantics the proxy fetches all silences, parses the
  first `filter` form value with the shared matcher parser, applies v1's `matchFilterLabels`
  behaviour client-side, then sorts. Unknown v1 query parameters are not forwarded.

The two expired silences created by 050's v2 setup remain in the contract store and therefore appear
in the recorded v1 GET /silences fixtures. This is expected; the fixtures include them.

### Telemetry

Spans/access logs carry `am.silence.operation` (`create`/`update`/`get`/`list`/`delete`),
`am.silence.id` when known, and the canonical `am.silence.matchers` for create/update/get. Create
and update also attach `am.silence.createdBy` and `am.silence.comment`; 030's redactor removes both
from spans and logs when the corresponding config options are enabled. `TestV1SilenceTelemetryAndRedaction`
checks redaction on and off and confirms the operation, ID and matcher attributes. List/get responses
use v2 millisecond timestamps, so they record the `silence.timestamps` lossy event when silences are
returned.

### Real-client evidence gap

Story 025 did not identify any actual silence-management client in the estate. The lab's v1 create/
get/gossip/muting test uses the proxy and real Alertmanager cluster, not an unknown production
client product. Estate-specific silence clients remain part of the 072/074 release gate, as already
recorded in the inventory.

### Verification evidence

- `make contract`: POST silences 10/10 PASS, GET silences 4/4, GET by ID 4/4, DELETE 3/3; only
  status and receivers remain XFAIL. All four pending entries are removed.
- `make lab` + `make lab-check`: three-member cluster, Prometheus v1 alerts, v1 firing/resolved,
  v2 notifications, existing v2 silence gossip and new v1 silence gossip/muting all pass.
  The v1 silence/alert test waits 6 seconds after a silenced alert is posted and confirms the
  receiver has no notification for that run.
- `go test -race ./...`, all-tag `go vet`/lint (0 issues) and full `make ci` pass.
