# 060 — Silences

**Depends on:** 040 · **Status:** Done

## Goal

`GET /api/v1/silences`, `GET /api/v1/silence/:id`, `POST /api/v1/silences` (create and
update-by-id), `DELETE /api/v1/silence/:id` via their v2 equivalents.

## Scope

- Matcher translation both ways (`name`/`value`/`isRegex`/`isEqual`), as recorded in 020.
- Create response: v2 `{"silenceID"}` → v1 envelope with `silenceId`.
- Update semantics: preserve the stored ID when the recorded in-place-update conditions hold;
  changed matchers/startsAt can expire the old silence and create a new ID. Verify against fixtures
  and the chosen target rather than assuming every update replaces the ID.
- Error mapping: unknown ID, invalid matchers, `endsAt` before `startsAt`.
- Telemetry: silence ID, matchers, `createdBy` (subject to 030's redaction config), operation type.
- Reuse 035's shared HTTP contract; remove the four silence endpoint pending entries as they pass.
  Exercise required real silence clients from 025, not only synthetic JSON payloads.
- Inputs recorded in [020's findings](../020-v1-contract-capture-and-harness/findings.md): update is
  in place only when `startsAt` matches the stored value (else a new ID); unknown ID is a plain-text
  404 on GET and a 400 envelope on DELETE in v1 but an empty 404 for both in v2; `isEqual` exists in
  v0.26.0 v1 matchers.

## Explicit non-goals

- Changing how silences replicate — cluster gossip already handles it.

## Key files

`internal/v1/silences.go`.
