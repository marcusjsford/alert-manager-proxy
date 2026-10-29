# 070 — Findings

_Append-only. Log discoveries, wrong assumptions, decisions made under ambiguity (and why), and
deviations from the plan **as they happen**, not reconstructed afterwards._

No findings recorded yet — story not started.

## 2026-10-29 — MVP replan

Real status-client evidence now comes from 025, since 020 captured no production traffic. Added
focused versionInfo verification because the contract normaliser masks that object entirely.

## 2026-10-29 — Development baseline clarified in 025

Estate client versions are unknown and the operator requested isolated Docker reproduction.
Implement/test fixture parsing and the versionInfo policy here; known clients can be exercised
when supplied, while actual estate status-client evidence remains mandatory in 072 before release.
