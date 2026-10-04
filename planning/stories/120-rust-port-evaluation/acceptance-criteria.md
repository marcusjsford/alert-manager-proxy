# 120 — Acceptance criteria

- [ ] Go baseline numbers for every metric in `plan.md` recorded with method and hardware.
- [ ] Rust port passes the **unchanged** contract harness from 020, plus a differential fuzz test of
      matcher parsing against the Go implementation (≥ 1M inputs, zero divergences).
- [ ] Telemetry parity: the same dashboards and rules (100) render and evaluate with the Rust binary unmodified.
- [ ] Same benchmark run for Rust; comparison table in `findings.md`.
- [ ] Keep/drop decision recorded as a new ADR decision with the numbers that justify it.
