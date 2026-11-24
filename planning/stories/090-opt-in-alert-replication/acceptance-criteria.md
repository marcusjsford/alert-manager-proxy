# 090 — Acceptance criteria

- [ ] 074's production fanout evidence is recorded: either the partial clients (named or unknown,
      with observed coverage) or an explicit "none found" over the agreed observation window.
- [ ] If the evidence shows no partial clients, the story is closed with that evidence and no
      traffic-path code is added.
- [ ] If replication is implemented, mode `off` is byte-identical to before (harness + no peer
      traffic observed).
- [ ] A client posting to one proxy in `clients:` mode: killing that client's target AM after the
      post still results in a notification from another node.
- [ ] The unknown-client targeting decision is recorded and tested (address-based opt-in, or a
      documented `all`-only limitation).
- [ ] A peer that hangs does not increase client-facing p99 latency beyond the no-replication
      baseline + 5% (measured, numbers in `findings.md`).
- [ ] No request loops: a test asserts replicated writes never reach a proxy's listen address.
- [ ] Measured cost of `all` mode on a 3-instance cluster is recorded with the recommendation, and
      production enablement is operator-approved with the observed need recorded.
