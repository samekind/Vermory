# Server Qualification Scale Implementation Plan

**Goal:** Qualify the frozen W12 `server-qualification-v1` profile with correct
multi-tenant lag, current-authority vector bootstrap, tail replay, concurrent
deletion/query pressure, and reproducible evidence.

**Architecture:** PostgreSQL remains authority and outbox. A tenant/profile
snapshot bootstrap captures an event watermark, rebuilds vectors from current
active authority, and then hands off events after the watermark to the existing
worker. Lag counts pending tenant rows rather than global event-ID distance.

## Constraints

- [x] Keep lexical as the product default and semantic retrieval opt-in.
- [x] Do not call SiliconFlow 100,000 times; deterministic vectors are only for
      operational scale and the real provider probe remains separate.
- [x] Do not touch the shared developer database during W12.
- [x] Preserve every failed full-profile attempt and its classification.
- [x] Do not claim HA, PITR, one million vectors, or memory quality.

## Task 1: Freeze W12

- [x] Commit the versioned case manifest, README, design, and this checklist.
- [x] Validate manifest arithmetic.
- [x] Preserve the final manifest SHA-256 in the evidence.

## Task 2: Correct Multi-Tenant Lag

- [x] Add a failing interleaved-tenant test that proves ID-distance lag is
      wrong.
- [x] Count actual pending tenant rows and preserve latest/cursor semantics.
- [x] Run retrieval store, worker, coordinator, and race gates.

## Task 3: Current-Authority Snapshot Bootstrap

- [x] Add failing tests for history collapse, watermark tail handling,
      concurrent deletion, provider failure, dimensions, and advisory locking.
- [x] Implement `ProjectionWorker.RebuildCurrent` with stable paging and
      authority recheck.
- [x] Add a production CLI surface using the registered direct provider profile.
- [x] Verify normal tail worker behavior and exact lexical degradation.

## Task 4: W12 Harness

- [x] Start a disposable PostgreSQL 18 cluster.
- [x] Seed 100,000 governed active facts across 10 tenants and 100 continuities.
- [x] Create 450,000 append-only governed revisions and verify 100,000 active,
      450,000 superseded, and 1,000,000 real trigger events.
- [x] Rebuild 100,000 lexical documents from current authority.
- [x] Run ten concurrent snapshot bootstraps for 100,000 deterministic vectors.
- [x] Run 50 clients, 1,000 queries, 1,000 deletes, and competing tail workers.
- [x] Verify counts, lag, isolation, deletion, cursor, latency, duration, and
      database-size gates.
- [x] Run the separate direct SiliconFlow post-scale projection/query probe.

## Task 5: Evidence And Delivery

- [x] Save normalized JSON and raw log with hashes and zero credential matches.
- [x] Update the evaluation matrix, hypothesis register, and scoped READMEs.
- [x] Run full serial PostgreSQL suite, CI race set, vet, tidy drift, OpenClaw,
      release snapshot, and diff checks.
- [x] Commit and push clean changes to the Draft PR.
- [x] Record protected CI and independently verify the uploaded artifact.
- [x] Keep the overall Vermory goal active after W12.
