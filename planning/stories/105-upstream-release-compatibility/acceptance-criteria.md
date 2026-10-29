# 105 — Acceptance criteria

- [ ] CI runs the full harness against both the production and latest Alertmanager versions on every
      push; both strictly green without pending exemptions, plus pass-through/inspection tests.
- [ ] The scheduled job, pointed at a release newer than the pinned one (or simulated by bumping the
      pin), runs the matrix and reports the result without manual steps.
- [ ] Deliberately breaking a translation (test branch) makes the matrix fail — proves it's a real gate.
