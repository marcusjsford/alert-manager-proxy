# 010 — Skeleton + transparent pass-through

**Depends on:** none · **Status:** Done

## Goal

A Go service that sits in front of one Alertmanager and passes **every** request through unchanged,
so it can be deployed with zero behaviour change before any translation exists. Accepts the
[ADR](../../ADR.md).

## Scope

- Go module, `cmd/alertmanager-proxy`, `internal/` layout; v1 router mounted at `/api/v1/*` that
  returns `501` with a v1-shaped error envelope until each endpoint's story lands — pending verification
  of the exact envelope in story 020.
- Config (file + env): listen address, upstream Alertmanager URL, separate admin listener
  (`/-/healthy`, `/-/ready`, own `/metrics` if needed), timeouts.
- `httputil.ReverseProxy` pass-through for everything not under `/api/v1/`, including streaming and
  the UI; preserves method, headers, query, body, status code.
- Dockerfile (distroless/static), `Makefile`, Forgejo Actions CI: build, `go vet`, `golangci-lint`,
  `go test -race`.
- Flip the ADR header to `accepted`.

## Explicit non-goals

- Any v1 translation (040–070). Telemetry (030). Fan-out (090).

## Key files

`go.mod`, `cmd/alertmanager-proxy/main.go`, `internal/config/`, `internal/passthrough/`,
`internal/v1/router.go`, `Dockerfile`, `.forgejo/workflows/ci.yml`, `../../ADR.md`.
