# Verified Tool Outcome Formation Implementation Plan

Design: [Verified Tool Outcome Formation Design](../specs/2026-07-18-verified-tool-outcome-formation-design.md)

## Task 1: Freeze Reality Case

- [x] Add and freeze `F03-verified-tool-outcome-formation` before implementation.
- [x] Validate all public cases and update the authoritative coverage counts.

## Task 2: Tool Observation Authority

- [x] Add the `tool_result` observation kind and tenant-scoped metadata migration.
- [x] Implement exact turn, run, session, tool, call-ID, content, size, and replay validation.
- [x] Reject sensitive result content before persistence and keep reset/deletion complete.

## Task 3: OpenClaw Capture

- [x] Add an explicit tool allowlist and bounded documented result extractors.
- [x] Register `after_tool_call` without registering another model tool.
- [x] Persist only successful exact-identity results and remain fail-open.

## Task 4: Mixed Formation And Review

- [x] Schedule completed turns through eligible user and tool observations.
- [x] Form from labeled `user_message` and `tool_result` evidence while rejecting assistant input.
- [x] Expose bounded source kind and tool label in HTTP and direct OpenClaw review.
- [x] Keep every candidate proposed until explicit operator governance.

## Task 5: Automated Qualification

- [x] Pass migration, RLS, FK, idempotency, drift, replay, size, sensitive-data,
  cross-continuity, deletion, reset, scheduler, worker, and review tests.
- [x] Pass OpenClaw extraction, allowlist, fail-open, identity, request-bound, and package tests.
- [x] Pass full PostgreSQL, race, Reality, vet, module drift, build, and packaging gates.
- [x] Preserve every rejected input and failed attempt in the evidence ledger.

## Task 6: Mac Mini Real-Client Evidence

- [x] Deploy W23 to isolated user-owned Mac mini paths without `sudo`.
- [x] Produce real OpenClaw `after_tool_call` events from allowed successful and failed tools.
- [x] Review and accept supported C01-derived outcomes and consume them in a fresh real-model turn.
- [x] Prove assistant-only, failed, unallowed, sensitive, duplicate, and cross-session input remains absent.
- [x] Forget one accepted tool-origin memory and prove covered deletion surfaces are clean.

## Task 7: Protected Delivery

- [x] Write evidence and update public documentation with only proven claims.
- [x] Commit without amending earlier commits and push the exact head.
- [x] Verify protected CI, artifacts, packages, signatures, and Mac mini evidence.
- [x] Update the existing Draft PR with exactly one W23 section.
- [x] Keep the overall Vermory platform goal active.
