# Story-Driven Implementation — Agent Workflow Prompt

**Status: v1** (adapted from `code-indexer/planning/STORY_WORKFLOW_PROMPT.md`). A living prompt —
refine it as this project's own history produces lessons; replace inherited examples with local ones
as they happen.

## What this is for

A drop-in task prompt for an agent (fresh context, no memory of how this got here) that needs to pick
up and execute one story from `planning/stories/`: one `MASTER_PLAN.md` index, plus `NNN-slug/`
folders each holding `plan.md`, `acceptance-criteria.md`, and `findings.md`. If you're an agent
reading this to start work, this whole file is your instructions.

## The folder contract

- `MASTER_PLAN.md` — links to source docs (`../ADR.md`), a workflow section (pointing back here), and
  an ordered story index table (story, status, one-line summary).
- `NNN-slug/` — one folder per story, numbered in steps of 10 so a story can be inserted later
  without renumbering. Number order is planned implementation order, not a hard sequence — see
  "Dependencies" below.
  - `plan.md` — header line (`**Depends on:** … · **Status:** …`), goal, scope, explicit non-goals,
    key files. Points at ADR sections rather than duplicating their prose.
  - `acceptance-criteria.md` — a checklist. Every item must be independently, concretely verifiable —
    "works correctly" is not an acceptance criterion; "the contract harness diff for `GET
    /api/v1/silences?filter=…` is empty" is.
  - `findings.md` — an append-only log of what actually happened: discoveries, bugs, deviations from
    the plan, decisions made under ambiguity and why. Starts as a template and fills in as work
    happens.

## The workflow

1. **Pick a story.** Lowest-numbered story in `MASTER_PLAN.md` that is not `Done` and whose
   `Depends on` stories are `Done`. If nothing qualifies, say so — don't skip a dependency to find
   something to do. **Branch before touching anything**: a worktree for `story/NNN-slug` off latest
   `main` (see "Git discipline").
2. **Read before writing.** The story's `plan.md` and `acceptance-criteria.md` in full, then the ADR
   sections they point at.
3. **Verify ground truth before implementing against an assumption.** This project is a translation
   layer between two wire protocols, so almost every bug available to it is a wrong assumption about
   one side's shape. The v1 side's source of truth is the **recorded fixtures from story 020**, not
   memory, docs, or the Swagger spec; the v2 side's is a real running Alertmanager of the target
   version. If a fixture doesn't cover the case you're implementing, record a new one from the real
   0.26 instance first, then implement. (Inherited lesson from `code-indexer`: a wrong field
   assumption there silently misnamed every C/C++ function; an "it's probably the same as the other
   case" guess turned out to be a genuinely different shape.)
4. **Implement in small, verifiable increments.** Run whatever check each acceptance criterion
   implies as you go — for translation stories that is the contract harness, not just unit tests.
5. **Log findings as you go**, not reconstructed from memory afterward. Wrong assumptions, decisions
   under ambiguity (and why), lossy translations discovered, scope questions resolved. Don't polish it
   into something that reads as if everything went to plan if it didn't.
6. **Re-verify every acceptance criterion explicitly before marking a story Done.** A passing test
   suite is evidence, not proof.
7. **On completion**, update, in the same commit set on the story branch:
   - the story's `acceptance-criteria.md` (check the boxes that are actually true);
   - the story's `plan.md` header `**Status:**` — easy to forget because `MASTER_PLAN.md` looks
     authoritative; both carry status because `plan.md` is what a reader sees when opening the
     folder directly;
   - `MASTER_PLAN.md`: flip the status and add a 2–5 sentence Findings Summary under "Completed Story
     Summaries" (the highlight reel; the full log stays in `findings.md`).
8. **If a story turns out blocked, oversized, or wrongly scoped, say so.** Mark it `Blocked` or
   `Needs re-scoping`, explain in `findings.md`, and stop. Forcing a mis-scoped story to `Done` is
   worse than leaving it honestly incomplete.

## Non-negotiables

- **Explicit non-goals over silent scope creep.** If `plan.md` names something out of scope, don't
  build it "while you're in there" — propose a new story.
- **Explicit scope decisions over silent assumptions.** Where a decision is needed, make it, write
  down *why* in `findings.md`.
- **Dependencies are real, not advisory.**
- **"Blocked" and "Needs re-scoping" are valid, honest outcomes.**
- **Never silently lossy.** When a v1 field has no v2 equivalent (or vice versa), the behaviour is a
  recorded decision in `findings.md` *and* is observable at runtime (a span event / counter, per
  ADR decision 4) — never a quiet default.

## Git discipline

- **One branch per story**, `story/NNN-slug` exactly matching the folder name, cut from latest `main`
  before any work starts. Never more than one story per branch.
- **Never branch a story off another story's branch, and never merge one story branch into
  another.** Check: `git merge-base main story/NNN-slug` equals `main`'s tip at branch creation; if
  another unmerged story's commit appears in `git log main..story/NNN-slug`, stop. (Inherited lesson:
  `code-indexer` stories 020/030/060 got tangled exactly this way and had to be rebuilt by
  cherry-pick.)
- **If a story genuinely needs another unmerged story's code**, merge the dependency to `main` first,
  or mark the new story `Blocked`. Don't stack branches "just to keep moving."
- **Merge each story promptly once approved** — don't let several sit unmerged. Translation stories
  040–070 will all touch the router and the shared v1 envelope code; merging the first promptly means
  the next only resolves conflicts against `main`, once.
- **One worktree per story**: `git worktree add ../alert-manager-proxy-worktrees/story-NNN-slug
  story/NNN-slug`. Remove it once merged. When spawning a subagent for a story, pass
  `isolation: "worktree"` instead of building this by hand.
- **Commit messages reference the story** (`[story/040] …`).
- **One PR per story**, opened once every acceptance criterion is checked. The PR description
  restates the acceptance-criteria checklist with each item marked.
- **No direct pushes to `main` for story work.** Planning-doc edits *within* a story travel on that
  story's branch.
- **Push local commits to the `origin` remote promptly, without stopping to ask.** This covers
  pushing already-made commits, not merging PRs or deploying, which remain explicit decision points.

## Autonomous execution

Inherited policy from `code-indexer` (decided there 2026-08-18), adopted here unchanged until this
project produces its own reason to revise it.

- **Merge authority.** An autonomous agent may merge its own story PR only when all three are
  independently true: (1) `go build ./...`, `go vet ./...` and the linter pass clean; (2) every
  acceptance criterion is re-verified and checked; (3) no regressions — `go test ./...` **and the
  full contract harness** pass unchanged. Otherwise stop. Merge authority does **not** extend to
  deploying the proxy in front of any real Alertmanager — that always needs a human.
- **Stop conditions.** Any merge-authority check fails and can't be fixed in scope; a destructive
  action would be needed (force-push, rewriting another story's history, deleting silences on a real
  Alertmanager); a real product/priority call comes up; **3 consecutive failed attempts** at the same
  acceptance criterion (mark `Blocked`, record what was tried); a `Depends on` story turns out not
  actually done.
- **Budget.** No wall-clock timer; a story running far longer than comparable completed ones is a
  signal to stop and report — likely `Needs re-scoping`.
- **Escalation.** Write the blocker into `findings.md`, set status honestly, and make the blocker the
  *first* thing in the final report.
- **Concurrency.** Sequential: one story fully merged before the next one's worktree is created.
- **Rollback.** Reverting a previously `Done` story always needs human sign-off. An agent may revert
  its *own* just-merged story within the same run, loudly, with the reason in `findings.md`.
