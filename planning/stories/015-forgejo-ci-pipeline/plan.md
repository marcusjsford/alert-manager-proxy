# 015 — Forgejo Actions CI pipeline working

**Depends on:** 010, 020 · **Status:** In progress

## Goal

Make `.forgejo/workflows/ci.yml` (written in story 010) actually run green on the local Forgejo, so
`make ci` stops being the only gate. Split out of 010 on 2026-10-05 when CI turned out not to work;
see [010's findings](../010-skeleton-and-passthrough/findings.md#ci-not-working-yet--local-make-ci-is-the-gate-for-now).

## Scope

- Diagnose why the pushed workflow doesn't run or pass. Unverified assumptions to check first:
  - the shared `act_runner` (registered for code-indexer, `code-indexer_indexer_net`) picks up jobs
    from `gitadmin/alertmanager-proxy`, with the `docker` label;
  - job `services:` (the Alertmanager for `integration`) work on this runner build and resolve by
    service name on that network;
  - the in-container `git clone` from `forgejo:3000` works with this repo's token.
- Fix the workflow or the runner config, whichever is actually wrong. Changes to code-indexer's
  runner configuration affect that project's CI too: name them explicitly before making them.
- Keep `make ci` and `ci.yml` running the same checks.
- Include 020's contract harness as a fifth job and gate docker-build on all four prerequisite
  jobs. Preserve development XFAIL handling; the strict release gate belongs to 072.
- Resume the existing clean story branch at `215882b` rather than overwrite it. Reconcile its
  successful push run with stale planning status and complete the isolated mutation test.
- Carry the user-requested MVP roadmap amendments in this branch (025/035/072/074 and dependent
  plan updates), without implementing those future stories.

## Explicit non-goals

- Image publishing to a registry (a later story if wanted).

## Key files

`.forgejo/workflows/ci.yml`, `Makefile`, code-indexer's `runner/config.yml` (read; change only with
sign-off).
