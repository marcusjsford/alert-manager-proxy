# 060 — Silences

**Depends on:** 040 · **Status:** Not started

## Goal

`GET /api/v1/silences`, `GET /api/v1/silence/:id`, `POST /api/v1/silences` (create and
update-by-id), `DELETE /api/v1/silence/:id` via their v2 equivalents.

## Scope

- Matcher translation both ways (`name`/`value`/`isRegex`, and `isEqual` if present in 0.26 v1 —
  confirm from fixtures).
- Create response: v2 `{"silenceID"}` → v1 envelope with `silenceId` (confirm exact casing from
  fixtures).
- Update semantics: v1 POST with an `id` expires the old and creates a new silence — confirm the v2
  path behaves identically, including the returned ID.
- Error mapping: unknown ID, invalid matchers, `endsAt` before `startsAt`.
- Telemetry: silence ID, matchers, `createdBy` (subject to 030's redaction config), operation type.

## Explicit non-goals

- Changing how silences replicate — cluster gossip already handles it.

## Key files

`internal/v1/silences.go`.
