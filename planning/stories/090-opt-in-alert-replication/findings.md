# 090 — Findings

_Append-only. Log discoveries, wrong assumptions, decisions made under ambiguity (and why), and
deviations from the plan **as they happen**, not reconstructed afterwards._

No findings recorded yet — story not started.

## 2026-11-24 — Rescoped after 074; production evidence first

Supersedes the 2026-10-29 "deferred until after 110" note. The story is now explicitly post-074 and
conditional: 074 must record production fanout evidence before this work starts. If 074 finds no
partial clients, this story is closed with that evidence and no replication code is added. If partial
clients exist, replication is implemented default-off and enabled only with operator approval and a
measured need.

Rationale: 090's value depends on a client population that 080 can detect but only a production
deployment can characterize. Building it before 074 would add queueing, duplicate writes and
write-amplification (9 vs 3 writes per fully-fanned alert in `all` mode) on a guess. 090 is also no
longer a 074 dependency, so the initial cutover is not blocked on it.

The unknown-client gap is now an explicit acceptance criterion: 080 reports unmatched clients by
address, while `clients:` selects by name, so implementation must either add address-based selection
or document `all` as the only option and measure its cost.

## 2026-10-29 — MVP replan

Deferred this opt-in behaviour change until after 110, using 080/100 evidence to justify the client
need. Neither replication nor automatic fanout detection gates initial v1 compatibility release.

## 2026-10-29 — Three-instance topology confirmed in 025

Changed the replication-cost baseline to three instances: fully fanned-out all-mode traffic makes
nine writes rather than three, replacing the original four-instance 16-versus-4 assumption.
