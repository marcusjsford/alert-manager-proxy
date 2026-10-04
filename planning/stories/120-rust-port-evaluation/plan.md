# 120 — Rust port evaluation

**Depends on:** 110 · **Status:** Not started

## Goal

Find out whether a Rust implementation is measurably smaller and faster than the Go one, and keep it
only if it is (ADR decision 1).

## Scope

- Go baseline first, on fixed hardware with a recorded load profile derived from real production
  traffic (from 100): idle RSS, RSS under load, CPU per 1k requests, p50/p99 added latency, image
  size, cold-start time.
- Rust port (axum/hyper + `opentelemetry` crates), same config format, same telemetry names and
  attributes so dashboards don't change.
- The hard parts to port: Alertmanager's matcher grammar (Go reference + harness to diff against),
  config YAML → JSON rendering with secret masking (070).
- Decision: keep, drop, or partial (e.g. only if a clear win). Recorded in the ADR as a new decision
  either way.

## Explicit non-goals

- Feature changes — the port is a like-for-like replacement.

## Key files

`rust/` (or a separate repo — decide and record), `bench/`.
