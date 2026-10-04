# 010 — Acceptance criteria

- [x] `go build ./...`, `go vet ./...`, `golangci-lint run` and `go test -race ./...` pass locally
      (`make ci`, 2026-10-04).
- [ ] …and in CI on a real push — **not verified**: CI on the local Forgejo runner isn't working
      yet; local `make ci` is the gate for now (see `findings.md`).
- [x] An integration test against a real Alertmanager (current release, in a container) shows
      `GET /api/v2/status`, `POST /api/v2/alerts`, `GET /api/v2/silences` and `GET /` (UI) return
      byte-identical bodies and the same status codes through the proxy as direct.
- [x] `/api/v1/*` returns `501` with a JSON v1 error envelope — not the upstream's pass-through `410 Gone`
      (corrected from "404", see `findings.md`).
- [x] Upstream unreachable → `502` within the configured timeout, not a hang.
- [x] Docker image builds; image size and idle RSS are recorded in `findings.md` as the first Go
      baseline for story 120.
- [x] `ADR.md` status reads `accepted`.
