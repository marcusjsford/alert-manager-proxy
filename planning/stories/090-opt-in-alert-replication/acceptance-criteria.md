# 090 — Acceptance criteria

- [ ] With mode `off`, behaviour is byte-identical to before (harness + no peer traffic observed).
- [ ] A client posting to one proxy in `clients:` mode: killing that client's target AM after the
      post still results in a notification from another node.
- [ ] A peer that hangs does not increase client-facing p99 latency beyond the no-replication
      baseline + 5% (measured, numbers in `findings.md`).
- [ ] No request loops: a test asserts replicated writes never reach a proxy's listen address.
- [ ] Measured cost of `all` mode on a 4-node cluster recorded, with the recommendation.
