# 020 — Findings

_Append-only. Log discoveries, wrong assumptions, decisions made under ambiguity (and why), and
deviations from the plan **as they happen**, not reconstructed afterwards._

## 2026-10-05 — Manual probing of v0.26.0 before writing the harness

Ran v0.26.0 with `test/contract/alertmanager.yml` (nested routes, four receivers, one with a basic-
auth password, an inhibition rule) and probed every v1 route by hand.

### Wrong assumption in the plan: `/api/v1/alerts/groups` doesn't exist

`GET /api/v1/alerts/groups` returns **`404 page not found`** on v0.26.0. The v1 route list in 0.26
is only: `GET /status`, `GET /receivers`, `GET|POST /alerts`, `GET|POST /silences`,
`GET|DELETE /silence/:sid`. Unknown v1 paths → 404 plain text, wrong method → 405 plain text. The
scenario records `/alerts/groups` as an unknown route, and story 050's plan, acceptance criteria and
`MASTER_PLAN.md` row were corrected on this story's branch (folder name kept so the branch name
stays stable).

### v1 behaviours a translation must reproduce

- **Alerts**
  - `POST` success: `{"status":"success"}` with no `data`.
  - `{"labels":{}}` and a missing `labels` both → `400 bad_data "at least one label pair required"`;
    an object instead of an array → `json: cannot unmarshal object into Go value of type
    []*types.Alert`; non-JSON → `invalid character 'o' in literal null (expecting 'u')`.
  - Resolved alerts (past `endsAt`) are accepted and not listed by `GET`.
  - `GET` query params: `filter` (with **or without** braces), `receiver` (regex), `silenced`,
    `inhibited`, `unprocessed`, and also **`active`** (not in the plan's list; added to the
    scenario). Invalid bool → `parameter "silenced" can either be 'true' or 'false', not "maybe"`;
    bad filter → `bad matcher format: bad`; bad receiver regex → `failed to parse receiver param: [`.
  - `annotations` is `null` when absent; `generatorURL` is `""` when absent; `receivers` is a list
    of names; `status.silencedBy`/`inhibitedBy` are `null` once processed but `[]` while
    `unprocessed`.
  - Timestamps have nanosecond precision.
  - With `continue: true` on a child route, an alert matching it is listed with only that child's
    receiver (`DiskFull` → `["db-pager"]`, not `db-team` too).
- **v1 `GET /alerts` does not recompute silence state.** It reports alert state as the processing
  pipeline last marked it, so a new silence doesn't show on v1 reads until the next pipeline flush
  (5m `group_interval` in this config). A **v2** `GET /alerts` recomputes state and updates the
  shared marker, after which v1 reads show it too. Confirmed on a separate instance: v1 kept showing
  `unprocessed []` after a silence until one v2 read, then `suppressed [<id>]`.
  **Consequence:** a proxy reading through v2 always shows *fresher* silence state than real 0.26
  did. Harmless — it's what 0.26 converges to. The scenario settles state through v2 `setup` steps
  before each v1 read, so fixtures hold settled state rather than a timing artefact.
- **Silences**
  - Create/update → `{"status":"success","data":{"silenceId":"…"}}` (`silenceId`; v2 uses
    `silenceID`).
  - Matchers in v1 output include `isEqual` (0.26 supports negative matchers in v1).
  - **Update by ID is in place only when `startsAt` matches the stored value.** Alertmanager stores
    `max(startsAt, now)`, so a client resending its own `startsAt` from slightly before creation
    gets a **new** ID and the old silence is expired. The first recording got a new ID for exactly
    this reason while my manual probe had kept the same one. The scenario now captures the stored
    `startsAt` and sends it back, recording both cases deterministically: extend only → same ID,
    change matchers → new ID.
  - Errors (all `400 bad_data`): no matchers → `silence invalid: at least one matcher required`;
    end before start → `silence invalid: end time must not be before start time`; end in the past →
    `end time can't be in the past`; bad regex → `error parsing regexp: missing closing ): …`;
    update with unknown ID → `silence not found`.
  - **`GET /api/v1/silence/<unknown or malformed>` → `404` with plain-text body `Error getting
    silence: <nil>`**, not a JSON envelope.
  - `DELETE` of an already-expired silence → `200 {"status":"success"}`; `DELETE` unknown →
    `400 bad_data "silence not found"`. `GET` of a deleted silence still returns it, `expired`.
- **Status:** `data` = `configYAML`, `configJSON`, `versionInfo`, `uptime`,
  `clusterStatus{name,status,peers[{name,address}]}`. Secrets are `<secret>` in both YAML and JSON.
- **Headers:** every v1 response (including preflight) carries `Access-Control-Allow-Origin: *`,
  `-Methods: GET, POST, DELETE, OPTIONS`, `-Headers: Accept, Authorization, Content-Type, Origin`,
  `-Expose-Headers: Date` and `Cache-Control: no-cache, no-store, must-revalidate`. `OPTIONS` →
  `200`, empty body.
- The cluster takes ~8s to reach `ready` from a cold start; the scenario waits for it.

## Harness design decisions

- **Scenario as data** (`scenario.yaml`) with `${now±d}` and captured `${var}`s, so fixtures, the Go
  proxy and a future Rust port share one definition.
- **Setup via v2.** v2 passes through the proxy, so one v1 endpoint's translation status doesn't
  block verifying another (e.g. 050 can pass before 060 exists).
- **Fixtures are raw; normalisation happens at compare time,** so improving the normaliser never
  needs a re-record. `verify` refuses to run if `scenario.yaml` changed since recording (SHA-256 in
  `meta.json`).
- **Timestamps compare as minute-rounded offsets from the run's start**, not masked: a default
  `endsAt` of `now+resolve_timeout` shows as `<now+5m>` and is still checked. **Sub-second precision
  is deliberately not compared**: v2 (so any translation) has millisecond precision, v1 had
  nanoseconds. Accepted loss for 040/050; the lossy counter from ADR decision 4 can still record it.
- **Fingerprints are kept**, not normalised (the plan listed them as volatile): they hash the label
  set alone and were identical across runs, so a difference would be a real bug.
- **Verdicts:** `PASS`, `FAIL`, `XFAIL` (in `pending.txt` and failing), `XPASS` (in `pending.txt`
  but passing). `XPASS` fails the run so the pending list can't go stale.
- **Verify-mode waits capped at 5s** unless a step sets `timeout`: while v1 POST is unimplemented,
  the alert-settling waits can never succeed, and 15s each cost a minute per run.

## Determinism

- The first `make contract-determinism` failed consistently in one place: with two silences on one
  alert, `silencedBy` order differed between instances. Most likely ordered by the random silence
  UUID, so meaningless. `silencedBy`/`inhibitedBy` now compare as sets. Caveat: UUID placeholders are
  numbered by first appearance; here every silence ID first appears in its create response, so
  numbering is stable. A scenario change that makes a set the first appearance could reintroduce
  flakiness.
- After that fix: **3/3 determinism runs identical** (each = two fresh v0.26.0 recordings compared).

## v1 vs v2 shape differences (v0.26.0 v1 fixtures vs v0.34.1 v2, same alerts and silences)

| Area | v1 (0.26.0) | v2 (0.34.1) |
|---|---|---|
| Envelope | `{"status":"success","data":…}` | bare body |
| Errors | `{"status":"error","errorType":"bad_data","error":…}` | bare JSON string (`"at least one label pair required"`), or `{"code":400,"message":…}` for a malformed body |
| Alert fields | `generatorURL: ""` when absent; no `updatedAt` | `generatorURL` omitted when empty; `updatedAt` added |
| `annotations` absent | `null` | `{}` |
| `silencedBy`/`inhibitedBy` | `null` once processed | `[]` |
| Alert status | `state`, `silencedBy`, `inhibitedBy` | adds **`mutedBy`** |
| `receivers` on alerts | `["db-pager"]` | `[{"name":"db-pager"}]` |
| Timestamps | nanoseconds | milliseconds |
| `filter` | `{a="b",c=~"d"}`, braces optional | **braces rejected** (`400 "unexpected open or close brace: …"`); one `filter` param per matcher |
| Create-silence response | `data.silenceId` | `silenceID` |
| Unknown silence GET | `404`, plain text `Error getting silence: <nil>` | `404`, empty body |
| Unknown silence DELETE | `400 bad_data "silence not found"` | `404`, empty body |
| `GET /receivers` | `["default", …]` | `[{"name":"default","labels":{"name":"default"}}, …]` |
| Status | `configYAML`, `configJSON`, `clusterStatus` | `config.original` only (no JSON), `cluster` |
| `Cache-Control` | `no-cache, no-store, must-revalidate` | `no-store` |
| CORS preflight `OPTIONS` | `200`, full CORS headers | **`405`**, only `Access-Control-Allow-Origin` |

Pointers to this table were added to the plans of 040, 050, 060 and 070.

## Real legacy-client traffic: not captured

The plan asked for sanitised real requests from the legacy clients against the production 0.26
cluster. **Not possible from this environment** — no access to the production Alertmanagers or their
clients. The scenario covers the full v0.26.0 v1 surface instead (every route, query parameter and
error path found). **Open question for the user:** which clients still speak v1 (Prometheus
versions, Grafana, scripts, dashboards) and which endpoints do they call? Once proxies run in front
of 0.26 (story 110 phase 1), story 030's telemetry answers this directly; any request shape the
scenario doesn't cover becomes a new step and a re-record.

## Verification

- `make contract-record`: 48 compared exchanges from v0.26.0 (`meta.json`:
  `alertmanagerVersion: 0.26.0`).
- `make contract-determinism`: 3/3 identical.
- `verify` against a fresh **v0.26.0** with an empty pending list: all 10 endpoints `PASS` (48/48) —
  also demonstrates the harness on a target that isn't the Go proxy.
- `make contract` against proxy → v0.34.1: all 10 endpoints `XFAIL`, exit 0, ~35s.
- Unit tests: expansion; normalisation (equivalent runs compare equal; `null` vs `[]`, offsets,
  fingerprints, extra fields, array order and UUID references all detected); status/header diffs;
  all four verdicts.

## 2026-10-09 — Missing contract table in the `make ci` log

A `make ci > log 2>&1` run exited 0 but its log had no contract table, though `make contract` alone
printed it. Not a harness bug: make's whole stdout was missing (even its echoed commands), because
the shell hook in this environment (RTK) summarises `make` output. Re-run unfiltered
(`rtk proxy make ci`, test cache cleared): build, vet, `go test -race` (config, server, harness),
lint 0 issues, integration PASS, contract table all 10 endpoints `XFAIL`, docker build — exit 0.
Every acceptance criterion re-checked against that run and the recordings above.
