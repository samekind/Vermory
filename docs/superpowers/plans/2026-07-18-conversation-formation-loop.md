# Conversation Formation Loop Implementation Plan

Date: 2026-07-18

Design: [Conversation Formation Loop Design](../specs/2026-07-18-conversation-formation-loop-design.md)

## Task 1: Freeze F01 And Public Acceptance

- [x] Add the authorized F01 transcript and governance trajectory.
- [x] Add the F01 manifest, events, fixture hashes, and deterministic lock.
- [x] Add reality validation assertions for formation, correction, deletion,
  transient-noise exclusion, and Global Default non-promotion.

## Task 2: Add Schema 19 And Failure-First Tests

- [x] Add failing migration tests for input kind, manifest, fingerprint,
  evidence observation, RLS, and tenant-continuity foreign keys.
- [x] Add type and parser tests for `source_observation_id` and document
  compatibility.
- [x] Add store tests for conversation continuity, cross-scope rejection,
  idempotency, input drift, active snapshot drift, and deletion redaction.
- [x] Add service tests for bounded user-only windows, provider packets,
  abstention, malformed evidence, and no provider replay.

## Task 3: Implement The Shared Formation Extension

- [x] Add migration 19 without renaming or replacing the existing formation
  tables.
- [x] Generalize run types and fingerprints for `document` and `conversation`.
- [x] Persist and verify ordered observation manifests.
- [x] Validate exact spans inside the referenced observation.
- [x] Preserve document formation behavior and audit compatibility.
- [x] Redact conversation evidence when its formed candidate is forgotten.

## Task 4: Add Operator Lifecycle Commands

- [x] Add `memory form-conversation` and
  `memory inspect-conversation-formation`.
- [x] Add conversation-scoped candidate accept and reject commands.
- [x] Keep provider credentials environment-only and output semantic receipts
  without raw provider payloads.

## Task 5: Automated Qualification

- [x] Run focused migration, store, service, CLI, and F01 tests.
- [x] Run all PostgreSQL-backed runtime tests and `go test -race` for affected
  packages.
- [x] Run OpenClaw O01, Hermes H01, retrieval, RLS, deletion, projection,
  recovery, and operations regressions.
- [x] Record all failures rather than removing inconvenient cases.

## Task 6: Mac Mini Real-Client Evidence

- [x] Deploy the protected build and fixture to Mac mini user-owned paths.
- [x] Persist the F01 trajectory through a real OpenClaw or Hermes client.
- [x] Form candidates with a real configured model.
- [x] Accept the initial candidates, form and accept the correction, forget the
  temporary code, and rebuild projections.
- [x] Prove a fresh real-client delivery contains Saturday and concierge, but
  not Friday, the code, rain, lunch, sibling history, or a permanent English
  default.
- [x] Prove invalid evidence, cross-continuity input, replay, fail-open, and
  deletion gates from database and client evidence.

## Task 7: Protected Delivery

- [x] Write the W21 evidence report and update the evaluation matrix and public
  documentation with only proven claims.
- [x] Commit without amending earlier delivery commits.
- [x] Push the branch and run protected CI on the exact head.
- [x] Verify artifact inventory and checksums on Mac mini without writing
  credentials or environment dumps.
- [x] Update the existing PR body with one W21 delivery section.
