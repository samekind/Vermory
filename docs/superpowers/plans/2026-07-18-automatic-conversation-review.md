# Automatic Conversation Formation And Review Implementation Plan

Design: [Automatic Conversation Formation And Review Design](../specs/2026-07-18-automatic-conversation-review-design.md)

## Task 1: Freeze Reality Case

- [x] Add and freeze `F02-automatic-conversation-review` before implementation.
- [x] Validate expected and forbidden behavior through the Reality Program.

## Task 2: Durable Scheduling

- [x] Add the tenant-scoped conversation formation schedule migration and RLS.
- [x] Enqueue exactly once from completed conversation turns.
- [x] Implement bounded claim, lease recovery, retry, and cursor advancement.
- [x] Preserve fail-open chat behavior and idempotent turn replay.

## Task 3: Worker

- [x] Add `conversation-formation-worker` with fixed tenant, direct provider,
  non-thinking, once, polling, and bounded-attempt options.
- [x] Reuse the W21 formation verifier and candidate lifecycle.
- [x] Emit semantic receipts without raw provider payloads or credentials.

## Task 4: Review API And OpenClaw

- [x] Add review-safe list, accept, and reject HTTP routes.
- [x] Keep correct and forget operator-only and exact-continuity scoped.
- [x] Register one direct `/vermory` OpenClaw command with separate operator
  token and safe short-reference resolution.
- [x] Keep governance unavailable as an OpenClaw or Hermes model tool.

## Task 5: Automated Qualification

- [x] Pass migration, store, service, worker, HTTP, RLS, replay, concurrency,
  lease, retry, deletion, and reset tests.
- [x] Pass OpenClaw tests, Hermes tests, full PostgreSQL suite, race tests, vet,
  module drift, and packaging gates.
- [x] Preserve every failed or rejected path in the evidence ledger.

## Task 6: Mac Mini Real-Client Evidence

- [x] Deploy W22 to user-owned Mac mini paths without `sudo`.
- [x] Run real OpenClaw automatic scheduling and direct in-client review.
- [x] Run a separate Hermes scheduling isolation control.
- [x] Use a real direct provider without Mac mini NewAPI.
- [x] Prove correction, rejection, forgetting, worker restart, and fail-open.

## Task 7: Protected Delivery

- [x] Write W22 evidence and update public documentation with proven claims.
- [x] Commit without amending earlier commits and push the exact head.
- [x] Verify protected CI, artifacts, packages, signatures, and Mac mini evidence.
- [x] Update the existing Draft PR with exactly one W22 section.
- [x] Keep the overall Vermory platform goal active.
