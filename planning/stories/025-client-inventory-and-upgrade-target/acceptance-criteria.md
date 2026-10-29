# 025 — Acceptance criteria

- [ ] Inventory records client versions, owners, used endpoints, fanout and sanitised example calls;
      unavailable evidence is named, with an owner and a concrete way to obtain it.
- [ ] Oldest real legacy Prometheus and each required read/silence/status client are identified and
      reproducible for 040/060/070/072 tests. Missing access blocks the relevant claim of compatibility.
- [ ] Production version, exact upgrade target and deployment/auth/path/topology requirements are
      recorded and confirmed with the operator; a test default is not presented as that confirmation.
- [ ] Any newly discovered request shapes have fixtures recorded from real 0.26; determinism and
      existing harness checks still pass.
