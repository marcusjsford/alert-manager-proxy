# 025 — Acceptance criteria

- [x] Inventory records operator-confirmed current 0.26.0, selected target 0.34.1 and three-instance
      Kubernetes destination, plus the Docker-only development scope and old-endpoint retirement intent.
- [x] Unknown production client versions, owners, auth/path/manifests and cutover details have a
      concrete collection plan and remain explicit 072/074 validation requirements. Representative
      test clients are not labelled as the estate's actual or oldest clients.
- [x] Start/check/stop commands bring up an isolated three-instance 0.34.1 cluster with three proxies,
      an independent 0.26.0 reference, notification receiver and pinned v1-capable Prometheus.
- [x] Checks prove three-member readiness, v2 pass-through, firing/resolved notifications, silence
      gossip and real Prometheus v1 delivery to 0.26.0; current proxy v1 failures are reported honestly.
- [x] Newly discovered request shapes, if any, are recorded from 0.26 before implementation; existing
      fixtures/scenario are preserved otherwise, and required existing checks remain green.
