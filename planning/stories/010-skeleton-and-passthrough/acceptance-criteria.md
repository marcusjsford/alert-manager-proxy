# 010 — Acceptance criteria

- [ ] `go build ./...`, `go vet ./...`, `golangci-lint run` and `go test -race ./...` pass locally and
      in CI on a real push.
- [ ] An integration test against a real Alertmanager (current release, in a container) shows
      `GET /api/v2/status`, `POST /api/v2/alerts`, `GET /api/v2/silences` and `GET /` (UI) return
      byte-identical bodies and the same status codes through the proxy as direct.
- [ ] `/api/v1/*` returns `501` (not a pass-through `404` from upstream), with a JSON error body.
- [ ] Upstream unreachable → `502` within the configured timeout, not a hang.
- [ ] Docker image builds; image size and idle RSS are recorded in `findings.md` as the first Go
      baseline for story 120.
- [ ] `ADR.md` status reads `accepted`.
