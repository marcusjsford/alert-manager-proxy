# 090 — Findings

_Append-only. Log discoveries, wrong assumptions, decisions made under ambiguity (and why), and
deviations from the plan **as they happen**, not reconstructed afterwards._

No findings recorded yet — story not started.

## 2026-10-29 — MVP replan

Deferred this opt-in behaviour change until after 110, using 080/100 evidence to justify the client
need. Neither replication nor automatic fanout detection gates initial v1 compatibility release.

## 2026-10-29 — Three-instance topology confirmed in 025

Changed the replication-cost baseline to three instances: fully fanned-out all-mode traffic makes
nine writes rather than three, replacing the original four-instance 16-versus-4 assumption.
