# 010 — Findings

_Append-only. Log discoveries, wrong assumptions, decisions made under ambiguity (and why), and
deviations from the plan **as they happen**, not reconstructed afterwards._

## 2026-10-04 — Ground truth before code

Ran `prom/alertmanager:latest` and `:v0.26.0` side by side and probed v1.

- **Latest Alertmanager is v0.34.1** (revision `73c6bfe`). The integration test, compose file and CI
  service pin it.
- **Wrong assumption in this story's own acceptance criteria:** they said a v1-less Alertmanager
  answers `/api/v1/*` with a pass-through **404**. It actually answers **`410 Gone`** with
  `{"status":"deprecated","error":"The Alertmanager v1 API was deprecated in version 0.16.0 and is
  removed as of version 0.27.0 - please use the equivalent route in the v2 API"}` (same for
  `POST /api/v1/alerts`). The criterion is corrected to "not the upstream's 410"; this is the one
  edit made to `acceptance-criteria.md` beyond ticking boxes. Relevant later: story 110's
  `v1.enabled=false` must reproduce exactly this 410 body, and the integration test now asserts
  upstream returns 410 so a future Alertmanager changing it gets noticed.
- **v0.26.0 v1 error envelope confirmed:** `400` with
  `{"status":"error","errorType":"bad_data","error":"invalid character 'o' in literal null ..."}`
  for a malformed alerts POST. Success envelope `{"status":"success","data":{...}}` seen on
  `/api/v1/status`, whose `data` has `configYAML`, `configJSON` (secrets shown as `<secret>`),
  `versionInfo`, `uptime`, `clusterStatus{name,status,peers[{name,address}]}` — input for 070.
- **v0.26.0 v1 responses carry CORS headers** (`Access-Control-Allow-Origin: *`,
  `-Methods: GET, POST, DELETE, OPTIONS`, `-Headers: Accept, Authorization, Content-Type, Origin`,
  `-Expose-Headers: Date`) and `Cache-Control: no-cache, no-store, must-revalidate`. Browser-based
  legacy clients may depend on them, including `OPTIONS` preflight. Flagged for 020 to record and
  040–070 to reproduce.

## Decisions made under ambiguity

- **501 placeholder uses `errorType: "not_implemented"`.** Not a value 0.26 ever returned, but the
  placeholder is temporary — each translation story replaces its routes — and a distinct type makes
  "proxy hasn't got this yet" unmistakable in client logs vs. a real upstream error.
- **Module path `alertmanager-proxy`** (no host). It's a binary, not imported anywhere; revisit if
  a package ever needs to be importable.
- **Default ports `:9095` (clients) / `:9096` (admin).** 9093/9094 are Alertmanager's HTTP and
  cluster ports, which the proxy will sit next to. Final port layout (e.g. proxy takes 9093 and
  Alertmanager moves) is a 110 decision.
- **Separate admin listener.** `/-/healthy` and `/-/ready` on the client port pass through, so
  existing health checks against the client port keep meaning "Alertmanager is healthy". The
  proxy's own `/-/ready` on the admin port is unconditional for now; real readiness semantics are
  110's (ADR decision 6).
- **Dispatch by prefix check, not `http.ServeMux`.** ServeMux cleans and redirects paths
  (`//`, `/./`), which would alter pass-through requests. A raw-socket test asserts
  `/api/v2//alerts/./groups` reaches upstream verbatim. `/api/v1x` is not v1 (tested).
- **Upstream errors: 502 for unreachable, 504 for slow.** Dial timeout → 502 (never accepted the
  connection), response-header timeout → 504.

## Bug found in own code

- **Dial timeout was classified as 504**, failing `TestBlackholedUpstreamTimesOutAtDialTimeout`
  (upstream `10.255.255.1`, SYNs unanswered). Go's net timeout errors satisfy
  `errors.Is(err, context.DeadlineExceeded)`, so the slow-upstream branch matched first. Fixed by
  checking for a `dial` `*net.OpError` before timeouts.

## Test weakness found by mutation

Integration test passed with a deliberately broken proxy that dropped every query string. Cause: on
a fresh Alertmanager the only alerts are the test's own, so a filtered and unfiltered
`GET /api/v2/alerts` return the same set. Added an unrelated "noise" alert posted directly; the
same mutation now fails the test, and the restored code passes.

## Lint configuration

`golangci-lint` v2.14.0 with `standard` + bodyclose, errorlint, gosec, misspell, noctx, revive,
unconvert; gofmt/goimports. Two production findings fixed (`net.Listen` → `ListenConfig` with ctx;
gosec G304 on the operator-supplied config path suppressed with a reason). `noctx`, `errcheck` and
gosec `G104` are excluded **in `_test.go` only** — test HTTP clients and deferred `Close()`s.

## Go baseline (first numbers for story 120)

Measured 2026-10-04, WSL2 host, image `alertmanager-proxy:010-baseline`, pass-through only (no
telemetry yet):

| Metric | Value |
|---|---|
| Stripped static binary (`-s -w -trimpath`) | 7,233,698 B (6.9 MiB) |
| Image (distroless/static nonroot), disk usage | 16.5 MB |
| Image content size | 3.8 MB |
| RSS idle, after start | 2.5 MiB |
| RSS after 2,000 sequential `GET /api/v2/status` | 8.0 MiB |

Graceful shutdown on SIGTERM confirmed (`docker stop` → exit code 0).
