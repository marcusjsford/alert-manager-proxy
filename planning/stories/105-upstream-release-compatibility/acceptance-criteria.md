# 105 — Acceptance criteria

- [ ] CI runs the full harness against both the production and latest Alertmanager versions on every
      push; both green.
- [ ] The scheduled job, pointed at a release newer than the pinned one (or simulated by bumping the
      pin), runs the matrix and reports the result without manual steps.
- [ ] Deliberately breaking a translation (test branch) makes the matrix fail — proves it's a real gate.
