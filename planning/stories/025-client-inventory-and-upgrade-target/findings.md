# 025 — Findings

_Append-only. Record discoveries, evidence gaps, decisions and deviations as they happen._

## 2026-10-29 — Added during compatibility review

Story 020 could not capture production traffic. Client versions, the production upgrade target and
deployment platform remain unconfirmed; this story makes that evidence an explicit deliverable.

## 2026-10-29 — Started from merged 015

Created `story/025-client-inventory-and-upgrade-target` and its worktree from main at `d53d294`.
Read the story plan/criteria and ADR before changing files. The only other worktree is the isolated
CI mutation test; no unmerged dependency work is used.

Initial evidence: fixture metadata proves the contract reference is Alertmanager 0.26.0, not the
production version. Compose/CI pin 0.34.1 for testing; they do not confirm an upgrade decision.
The contract receiver config is synthetic (loopback port 1, sample teams/users), not an estate
inventory. No running Alertmanager or Prometheus container appears in the local Docker listing;
that does not establish anything about remote hosts or non-container services.

## 2026-10-29 — Operator confirmed current version; inventory started

The operator confirmed: “all ccurrent alertmanagers are 0.26.0”. Record this as a confirmed
production fact in `planning/client-inventory.md`. It does not select the upgrade version or
identify client versions/deployment/auth details.

Production telemetry service discovery for names alertmanager and prometheus each returned zero
matches. Project configuration contains the synthetic scenario/fixtures and disposable Compose/CI
test arrangements, not production client/deployment manifests. Searches by filename in the local
environment and alert-authoring repositories also found no Alertmanager-named files; those narrow
searches do not prove the absence of configuration elsewhere.

The inventory separates operator-confirmed facts, ADR intentions, verified test evidence and
unknowns. No estate-specific new request shapes have been discovered, so no speculative fixtures
or arbitrary legacy client version pins were added. Remaining evidence owners are not yet named.

## 2026-10-29 — Operator selected target and corrected topology

Operator answers: target “0.34.1”; deployed in Kubernetes with “3 nodes instead of 4”; existing v1
APIs are deprecated once clients successfully reach the three-instance v2 set through the proxy.
Legacy client versions are “Not known yet”. Three Alertmanager instances do not establish how
many physical Kubernetes workers exist or how pods are scheduled.

Updated the inventory, active ADR/master-plan topology, silence-gossip criteria, 074 Kubernetes/
cutover requirements, and 080/090 fanout/replication counts. On three instances, replication of
already fully fanned-out traffic means 9 total writes instead of 3, not the old 16 versus 4.
Historical append-only findings retain the original four-node assumptions; active plans supersede
them. The four silence endpoint categories and four CI prerequisite jobs are unrelated counts.

074 must establish parallel replacement versus rolling cutover and verify state continuity and
rollback for the chosen path. Moving a v1 client behind the proxy preserves its v1 requests, so
deprecating old direct endpoints is distinct from disabling proxy translation in 110. Concrete
manifests, TLS/auth/path details, exact client versions and reproducible calls remain unavailable.

## 2026-10-29 — Development scope clarified; representative Docker reproduction

The operator clarified: “we're on a dev machine so we need to spin up our own for testing via
docker”. Before that clarification, read-only local Kubernetes checks found no Alertmanagers in
the reachable context; a local Prometheus ConfigMap had no alerting block, and the other two API
contexts refused connections. No Kubernetes resources were modified.

Amended 025's scope/criteria to deliver the confirmed contract and a Docker reproduction, with
actual estate versions/auth/manifests and migration mechanics explicitly carried to 072/074.
040 uses a real representative sender for development; 070 uses fixture/policy tests while
unknown actual clients remain a production gate. These are recorded scope changes, not claims
that arbitrary test versions are the estate's oldest clients.

Selected the locally available, pinned Prometheus 2.55.1 as a representative v1-capable binary.
Its real promtool successfully validates api_version: v1 and the synthetic alert rule. The lab has
three clustered AM 0.34.1 instances, three corresponding proxies, an independent non-clustered
AM 0.26.0 reference, a bounded Go webhook receiver and Prometheus. All exposed ports bind loopback;
plain HTTP/no-auth applies only to this synthetic Docker environment. Persistent make lab targets
are separate from the existing disposable CI projects and ports.

## 2026-10-29 — Ground truth and startup races

Real status calls confirmed all target members report 0.34.1, ready, and exactly three peers;
each proxy fronts its corresponding distinct member. Prometheus reports 2.55.1 and discovers
explicit /api/v1/alerts URLs for all three proxies plus the independent reference. The extra
reference is a test-only fourth destination, not a cluster member or production fanout change.

The first alert evaluation could precede discovery and drop its notification. Default resend was
one minute; a 5s lab resend flag makes the reference reproduction prompt and repeatable. Also,
Prometheus's healthy endpoint can return before destination discovery finishes: the first smoke
run failed on an empty discovered set while all other checks passed. Fixed the test to poll actual
discovery rather than treating health as proof of discovery. The next smoke run passed all four
tests with race detection: membership/1:1 mapping, real representative v1 delivery to 0.26.0,
v2 firing+resolved webhooks, and silence gossip across three instances. V1 proxy status logs 501,
honestly reflecting the missing translation.

Local rtk proxy make ci passed build, vet, race tests, lint (0 issues), pass-through integration,
the unchanged contract harness and Docker image build. The contract is still 0/48 XFAIL by design;
no translation or contract scenario/fixtures were changed in this story.

## 2026-10-29 — Fresh-start verification and completion

Ran make lab-down, make lab and make lab-check from an empty lab project. All nine containers
started; all four smoke tests passed with race detection in 12.1s, including the startup wait for
actual cluster membership and Prometheus discovery. The lab is left running for subsequent work.
Tagged lab lint also reports 0 issues. git diff confirms test/contract and test/integration are
unchanged; whitespace and the 19-story dependency/status consistency check pass.

Re-verified all five amended criteria against the operator answers, inventory, documented command
paths, actual Prometheus/reference/cluster results, fresh-start check and full local CI. Marked
025 Done on its branch. Production evidence is still outstanding in the inventory and explicitly
required in 072/074; this completion is the agreed development baseline, not production approval.
