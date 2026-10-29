# 040 — Findings

_Append-only. Log discoveries, wrong assumptions, decisions made under ambiguity (and why), and
deviations from the plan **as they happen**, not reconstructed afterwards._

No findings recorded yet — story not started.

## 2026-10-29 — MVP replan

025 identifies the real legacy sender; 035 now owns shared envelopes/routing/headers/transport.
040 is the sender-delivery milestone, while 074 remains the full compatibility release boundary.
Added explicit failed-write acknowledgement and pending-entry removal criteria.

## 2026-10-29 — Docker development baseline confirmed in 025

The operator requested a local Docker lab; actual client versions are unknown. 040's development
sender is the real, pinned Prometheus 2.55.1 configured for v1 in that lab. Oldest-estate-client
validation remains a separate mandatory 072/074 release gate, not an inferred compatibility claim.
