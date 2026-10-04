# 015 — Acceptance criteria

- [ ] Root cause of the CI not working is recorded in `findings.md`, verified, not guessed.
- [ ] A real push runs all four jobs (`build-test`, `lint`, `integration`, `docker-build`) green
      on the local Forgejo (moved here from story 010).
- [ ] `integration` really runs against the Alertmanager service: a deliberately broken branch
      fails it.
- [ ] `make ci` and `ci.yml` run the same set of checks (diffed by hand, listed in `findings.md`).
