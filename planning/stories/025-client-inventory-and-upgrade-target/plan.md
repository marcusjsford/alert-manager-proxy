# 025 — Client inventory + upgrade target

**Depends on:** 015 · **Status:** Not started

## Goal

Define the real client and deployment contract needed to release the compatibility MVP.

## Scope

- Inventory legacy client product/version, owner, v1 endpoints, parameters, request/response
  expectations, resend intervals and which of the four Alertmanager URLs they use.
- Obtain sanitised requests from existing logs/configuration or an isolated reproduction. Record
  missing production access explicitly; do not deploy the current 501 v1 placeholder to capture traffic.
- Pin the existing production version and selected upgrade target; distinguish the test default
  v0.34.1 from a confirmed production upgrade decision.
- Confirm systemd/compose/k8s deployment, addresses/ports, TLS/authentication, ingress/path prefixes,
  trusted proxies and the existing HA topology. Feed requirements into 030, 035, 072 and 074.
- Extend story 020's scenario/fixtures with uncovered real request shapes, recording from 0.26
  before implementing. Identify the exact legacy client images/binaries for end-to-end tests.

## Explicit non-goals

- Translation implementation, production deployment, changing client fanout or selecting a Rust port.

## Key files

`planning/client-inventory.md`, `test/contract/scenario.yaml`, `test/contract/fixtures/v0.26.0/`.
