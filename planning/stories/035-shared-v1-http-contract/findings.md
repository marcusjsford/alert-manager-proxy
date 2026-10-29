# 035 — Findings

_Append-only. Record discoveries, evidence gaps, decisions and deviations as they happen._

## 2026-10-29 — Added during compatibility review

OPTIONS and unknown-route/method categories were in pending.txt without a clear implementation
owner. Shared envelope work moved here from 040 so each endpoint inherits the same HTTP contract.

## 2026-10-29 — Implementation and verification

### Ground truth beyond the fixtures

Probed the real v0.26.0 container for edge cases the recorded scenario does not cover:

- `GET /api/v1` and `GET /api/v1/` are 404 plain text; `/api/v1/alerts/` and `/api/v1/status/`
  redirect 301 to the slashless path.
- `HEAD /api/v1/alerts` is **405**, not an implicit GET.
- 405 includes `Allow` (`GET, OPTIONS, POST` for alerts; `DELETE, GET, OPTIONS` for a silence ID),
  and both 404/405 omit the CORS/cache headers that implemented endpoints send.
- `OPTIONS /api/v1/alerts`, `/api/v1/nope` and `/api/v1/` all return 200 with empty body and the
  recorded CORS/cache headers; the bare `OPTIONS /api/v1` is a 307 to `/api/v1/`.
- `/api/v1/alerts/groups` is 404 for both GET and POST in 0.26.

The router reproduces these. Trailing-slash redirects are implemented only when the trimmed path
matches a known v1 route; arbitrary paths still 404. `OPTIONS /api/v1` keeps the observed 307.
The v1 router is still reached only through the existing `Owns` prefix check, so `/api/v1x` and all
non-v1 traffic continue to pass through untouched.

### Shared envelope contract

`internal/v1/envelope` owns the five recorded CORS/cache headers and three response forms:
`{"status":"success"}`, `{"status":"success","data":...}`, and
`{"status":"error","errorType":"...","error":"..."}`. JSON is marshalled without a trailing newline
because the fixtures contain none; `Content-Type` is exactly `application/json`. `SuccessData`
always emits `data`, including empty slices and `null`, so endpoint stories can preserve 0.26's
`[]`/`null` distinction. `Text` writes the recorded plain-text form (for example 060's
`Error getting silence: <nil>\n`) with the same CORS/cache contract headers. Unknown routes and
wrong methods intentionally bypass those headers and use `http.Error`, matching 0.26's bare
`text/plain` 404/405 responses.

### Shared v2 transport and failure choices

`internal/v1/upstream` builds requests below any configured upstream path prefix, preserves repeated
query parameters, copies safe end-to-end headers (notably `Authorization`, `Content-Type`,
`Accept`, custom auth headers) and drops hop-by-hop headers plus `Accept-Encoding`/`Content-Length`
so Go's transport negotiates and decodes the body. Redirects are **not** followed: a translated
write must never be replayed as a redirect. Dial and response-header timeouts reuse the existing
configuration values.

`DoJSON` treats every non-2xx response as an error, including 3xx, so a failed alert/silence write
cannot be acknowledged as success. Fallback mappings (recorded here as required by the criteria):

| Condition | v1 status | errorType | Message |
|---|---|---|---|
| Upstream non-2xx response | 502 | `bad_gateway` | `upstream Alertmanager returned <status>` |
| Dial failure/upstream unreachable | 502 | `bad_gateway` | `upstream Alertmanager unreachable` |
| Response-header timeout/deadline | 504 | `gateway_timeout` | `upstream Alertmanager timed out` |
| Client cancellation | 499 | `canceled` | `client canceled request` |
| Other transport failure | 502 | `bad_gateway` | `upstream request failed` |

Expected upstream 4xx statuses (silence-not-found and similar) are endpoint-owned translations;
those stories must inspect `*upstream.ResponseError` before falling back to `WriteFailure`. The
fallback deliberately reports the upstream status class as a gateway failure rather than leaking a
v2 error body into the v1 contract. Failure logs include method, v1 path, mapped status and fixed
message only — never headers, query strings, raw transport errors or upstream URLs, because a dial
error can contain credential-bearing URLs.

The router passes `telemetry.Runtime.Transport` into this client, so every future endpoint call
inherits 030's client span/metric instrumentation and the request's identity/route state. No v1
endpoint performs an upstream write yet (all are 501), so this story can only verify the wiring
structurally: an upstream test proves the injected transport wrapper is used, and the existing 030
tests prove that wrapper records spans/metrics. Runtime v1 upstream telemetry becomes observable
when 040 lands.

### Non-root paths and scope boundaries

Story 025 records the production client path prefix and authentication as unknown. No prefix
behavior was invented: the router owns only root `/api/v1` exactly as before, and pass-through
behavior is unchanged. Endpoint validation, payload translation and 4xx mappings remain in
040–070; only OPTIONS and unknown-route/method pending entries were removed.

### Verification evidence

- `make contract`: OPTIONS 2/2 PASS, unknown routes 3/3 PASS; all eight endpoint groups remain
  XFAIL with 0 matches, as intended. Setup warnings are expected because POST endpoints still 501.
- `make integration`: pass-through, v2 writes/silences and the v1-owned 501 check pass.
- `go test -race ./...` and `go vet ./...`: pass, including new envelope/router/upstream tests.
- `make lint`: 0 issues.
- Full `make ci`: build, vet, race tests, lint, integration, contract and Docker build pass.
