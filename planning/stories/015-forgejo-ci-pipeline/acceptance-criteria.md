# 015 — Acceptance criteria

- [x] The reported CI gap is investigated using real run/job evidence, with verified causes and
      any unavailable historical evidence explicitly distinguished in `findings.md`.
- [x] A real push runs all five jobs (`build-test`, `lint`, `integration`, `contract`, `docker-build`) green
      on the local Forgejo (moved here from story 010).
- [x] `integration` really runs against the Alertmanager service: a deliberately broken branch
      fails it.
- [x] `make ci` and `ci.yml` run the same set of checks (diffed by hand, listed in `findings.md`).
- [x] The contract job reports the per-endpoint table and docker-build depends on its success;
      current expected failures are explicitly distinguished from release-ready compatibility.
- [x] MVP story order, dependencies, shared HTTP ownership and strict release/cutover boundaries
      agree across the master plan, amended plans and the workflow selection rule.
