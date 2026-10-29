# v1 contract harness

The v1 API contract is what Alertmanager **v0.26.0** actually returned (ADR decision 3), recorded
in `fixtures/v0.26.0/` by driving `scenario.yaml` against a fresh instance configured with
`alertmanager.yml`. The harness replays the same scenario against any target and diffs normalised
responses. It only speaks HTTP to a base URL, so it verifies any implementation of the proxy,
including the Rust port (story 120).

| Command | What it does |
|---|---|
| `make contract` | Build the proxy, run it in front of current Alertmanager, verify against the fixtures. Per-endpoint `PASS`/`FAIL`/`XFAIL`/`XPASS` table; fails on `FAIL` or `XPASS` |
| `make contract-record` | Re-record the fixtures from a fresh v0.26.0 (needed after editing the scenario or config; `verify` refuses stale fixtures) |
| `make contract-determinism` | Record twice from two fresh v0.26.0 instances and require identical normalised results |
| `go run ./test/contract/cmd/contract verify -target URL -pending ""` | Verify any running target, e.g. a different proxy build |

**Pending list.** `pending.txt` lists endpoints not translated yet. They report `XFAIL` instead of
failing. A story that implements an endpoint removes its line; a pending endpoint that passes
reports `XPASS` and fails the run, so the list can't go stale. `-v` prints diffs for pending
endpoints too.

**Normalisation** (`harness/normalise.go`): timestamps become minute-rounded offsets from the run's
start (`<now+60m>`), silence UUIDs become `<uuid:N>` in scenario execution order (including
uncompared v2 setup responses; set-valued `silencedBy`/`inhibitedBy` arrays are sorted before any
remaining placeholders are assigned, because their order is meaningless), version info and cluster
peer identities are masked. Fingerprints, `null` vs `[]`,
field presence, array order (other than the sets above), status codes, error strings and the
contract headers (`Content-Type`, `Cache-Control`, CORS) are all compared.

**Settling state.** v0.26.0's v1 `GET /alerts` reports alert state as the processing pipeline last
left it, not live. The scenario's `setup` steps poll v2 (which recomputes state) before each v1
read, so fixtures show settled state. See story 020's findings. The silence state used by the
`GET /alerts` fixtures is also created and expired through v2 setup: v1 silence operations belong
to story 060, and putting them after every `GET /alerts` comparison keeps the two endpoints
independently verifiable.
