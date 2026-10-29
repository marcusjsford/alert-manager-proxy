# 050 — Findings

_Append-only. Log discoveries, wrong assumptions, decisions made under ambiguity (and why), and
deviations from the plan **as they happen**, not reconstructed afterwards._

No findings recorded yet — story not started.

## 2026-10-29 — MVP replan

Made active explicit in the query-param scope, removed the stale groups.go key-file reference,
and assigned shared HTTP behaviour to 035. GET alerts must leave pending.txt when implemented.

## 2026-10-29 — Implementation and verification

### Ground truth from the v0.26 source and fixtures

Checked the recorded 0.26 implementation and fixtures rather than guessing:

- v1 uses `r.FormValue("filter")`, so only the first repeated filter is used; braces are optional.
  It parses with `labels.ParseMatchers` and matches all matchers against the alert labels.
  The parser is now imported from Alertmanager v0.34.1 (the target) via
  `internal/v1/filter`; its error text and parsing match 0.26 for the contract cases.
- v1 bool parsing accepts only `""`/`"true"`/`"false"` (empty defaults true) and returns
  `parameter "<name>" can either be 'true' or 'false', not "<value>"`.
- v1 compiles `receiver` as `^(?:<value>)$` and returns `failed to parse receiver param: <value>`
  on failure, then matches it against any receiver of the alert.
- v1 sorts response alerts by fingerprint, skips already-resolved alerts, and returns
  `data: []` rather than null when nothing matches.
- Processed alerts expose `silencedBy`/`inhibitedBy` as `null` when empty; unprocessed alerts
  expose `[]`. v2 normalises both to `[]`, so the proxy reconstructs the v1 form using
  `status.state`.
- v2 already sorts by fingerprint, but the proxy sorts again because the v1 contract requires it
  and the upstream order is not part of the shared transport contract.
- v2 accepts `unprocessed` but its alert filter currently ignores it; the proxy still validates the
  parameter and defensively drops `state:"unprocessed"` when `unprocessed=false`.

### Contract-scenario dependency discovered and fixed

The first contract run failed only on four `GET /alerts` steps that need silences. Those silences
were created by v1 `POST /silences`, which is story 060's endpoint and still returns 501. Removing
the GET pending entry therefore exposed a hidden ordering dependency: the endpoint could not pass
its own fixtures until 060 landed.

The scenario now creates and expires the silences needed by `GET /alerts` through v2 setup, and
moves the v1 silence operations after every `GET /alerts` comparison. The v1-created silences can
therefore not affect the GET fixtures, and 060 still gets its complete recorded v1 silence corpus.
Alertmanager retains expired silences, so those two setup silences remain visible in 060's
`GET /silences` fixtures; the re-recorded fixtures include them and the proxy must reproduce that
list.
This follows story 020's stated design that setup goes through v2 so endpoints are independently
verifiable. All 48 exchanges were re-recorded from fresh v0.26.0 and determinism passes.

A second determinism issue appeared because the two v2-created silence UUIDs first appeared together
in a `silencedBy` set, whose order is random. The runner now records UUID seeds in scenario execution
order, including uncompared setup responses, so placeholders follow creation order. Set-valued arrays
are also sorted before assigning any remaining placeholders; their order carries no meaning. The
output is still sorted after replacement, so set comparison semantics are unchanged. This keeps the
recorded semantic mapping stable across different random UUIDs on every proxy run.

### v1↔v2 field differences and handling

| Difference | Handling | Lossy? |
|---|---|---|
| v1 `filter` braces/one matcher-set vs v2 repeated single matchers | Parse the first v1 filter with Alertmanager's parser; send each matcher's canonical string as a repeated v2 `filter` | No |
| v1 `receiver` regex vs v2 regex | Validate with the v1 anchored compile and error string; forward the original expression | No |
| v1 `active`/`silenced`/`inhibited`/`unprocessed` vs v2 booleans | Validate v1 defaults/error; forward explicit values; apply `unprocessed=false` client-side because v2 ignores it | No field loss |
| v2 `receivers: [{name}]` vs v1 `receivers: ["name"]` | Flatten in upstream order | No |
| v2 `annotations: {}` vs v1 `null` | Empty map becomes nil | No |
| v2 omits empty `generatorURL` vs v1 `""` | Always emit the string field | No |
| v2 `silencedBy`/`inhibitedBy: []` vs v1 `null`/`[]` | Empty becomes nil for processed states, remains `[]` for unprocessed | No |
| v2 `status.mutedBy` | Dropped; v1 had no field | No (v2-only) |
| v2 `updatedAt` | Dropped; v1 had no field | No (v2-only) |
| v2 millisecond timestamps vs v1 nanosecond output | Emit v2's millisecond values; record `alert.timestamps` lossy event | **Yes** |
| v2 recomputes live silence state; v1 reported the pipeline's last state | Use the fresher v2 state; 020 already recorded this accepted divergence | No field loss |
| v1 may expose `state:"unprocessed"`; v2 computes active/suppressed | Cannot reconstruct v1's pipeline timing; use v2's state and filter it only when `unprocessed=false` | Accepted state-model difference, not a dropped field |

No other lossy difference was found: resolved alerts are excluded by both, receiver order comes from
the same route matching, and fingerprints are the same label-set hash.

### Telemetry

The parsed query is attached to the span/access log as `am.v1.filter`, `am.v1.receiver` and the four
`am.v1.<flag>` booleans. When at least one alert is returned, the proxy records
`am.translation.lossy` with field `alert.timestamps`, reason `millisecond precision`, and increments
`am_proxy_translation_lossy_total` once for the request. The telemetry test asserts those values and
the counter. Unknown v1 query parameters are not forwarded to v2.

### Verification evidence

- `make contract`: `GET /api/v1/alerts 15/15 PASS`, `POST /api/v1/alerts 5/5 PASS`; all remaining
  endpoint groups are their expected XFAILs.
- `make contract-record`: 48 exchanges from v0.26.0.
- `make contract-determinism`: all ten endpoint groups PASS across two fresh recordings.
- `go test -race ./...`, `go vet`, all-tag vet/lint (0 issues) and `make ci` pass.
- `TestV1GetAlertsTelemetry` exports a real v1 server span and lossy counter; the filter table covers
  all four operators, quoted commas, escaped quotes and invalid input.
- Forgejo push CI run 27 exposed a pre-existing flaky 2-second budget in
  `TestCollectorOutageDoesNotBlockRequests` under the shared runner. The test still requires all 500
  requests and asserts bounded shutdown; its delivery budget is now 10 seconds so CI load cannot
  turn the outage check into a false failure.
