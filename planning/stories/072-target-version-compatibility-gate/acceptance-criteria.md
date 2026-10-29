# 072 — Acceptance criteria

- [ ] Strict release verification passes every fixture against proxy → 0.26 and proxy → the pinned
      target, with no XFAIL/XPASS and an empty implementation pending list; both versions run in CI.
- [ ] A deliberately broken translation makes the strict CI gate fail; docker/release jobs depend
      on that gate. A non-empty pending list also fails the release command.
- [ ] Client versions and reproducible end-to-end results are recorded for every required client
      class from 025 and the actual oldest estate sender, including firing/resolved notifications
      and silence suppression; representative-only lab success is insufficient for production.
- [ ] versionInfo and timestamp policy have focused assertions beyond the normalised diff; known
      differences are documented and emit the required lossy telemetry.
- [ ] Upstream refusal/timeout/recovery, estate auth/path requirements and non-v1 pass-through are
      verified; failed upstream writes do not produce success responses.
