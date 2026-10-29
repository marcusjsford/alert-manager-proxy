# 072 — Acceptance criteria

- [x] Strict release verification passes every fixture against proxy → 0.26 and proxy → the pinned
      0.34.1 target, with no XFAIL/XPASS and an empty implementation pending list; both versions run
      in CI.
- [x] A deliberately broken translation makes the strict CI gate fail; docker jobs depend on both
      strict contract jobs. A non-empty pending list fails the release command before verification.
- [x] Representative client evidence is reproducible for 025's client classes: the pinned Prometheus
      2.55.1 sender and the Python v1 client probe cover firing/resolved alerts, filters, silence
      suppression/gossip, status parsing/versionInfo and receivers. Actual estate versions, oldest
      sender, production auth/path/manifests and fanout are explicitly transferred to 074.
- [x] versionInfo and timestamp policy have focused assertions beyond the normalised diff; the real
      version policy and millisecond truncation are tested for both versions, and known differences
      emit the required lossy telemetry.
- [x] Upstream refusal/timeout/recovery, required auth forwarding and non-v1 pass-through are
      verified; failed upstream writes do not produce success responses. Estate-specific auth/path
      arrangements remain 074 evidence.
