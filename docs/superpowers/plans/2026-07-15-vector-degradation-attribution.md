# Vector Degradation Attribution Plan

**Goal:** Attribute W12 vector degradation without changing production retrieval
unless the evidence identifies an actual query defect.

## Task 1: Freeze And Test Competing Explanations

- [x] Freeze the W12 schedule, current-control shape, and failure-code invariants.
- [x] Preserve the final case SHA-256.
- [x] Run a 100,000-vector projection-current control with 550 vector queries.
- [x] Inspect the production query plan rather than a simplified stand-in.
- [x] Reject the scoped-HNSW hypothesis after the current control passes 550/550.

## Task 2: Instrument The Original Schedule

- [x] Add retrieval-audit aggregation to the W12 harness.
- [x] Require requested vector count, effective count, projection-lag count, and zero other degradation.
- [x] Replay 100,000 active facts, 450,000 revisions, 1,000 deletes, and competing tail workers.
- [x] Confirm effective vector plus projection lag equals 550 and other degradation equals zero.

## Task 3: Correct Evidence

- [x] Save both raw logs and normalized attribution evidence with hashes.
- [x] Correct W12 evidence, JSON, READMEs, evaluation matrix, hypothesis register, and Draft PR.
- [x] Preserve the rejected HNSW hypothesis and fixture failures as an explicit evidence trail.

## Task 4: Delivery

- [x] Run full PostgreSQL, race, vet, module, OpenClaw, and release gates.
- [x] Commit, push, update the Draft PR, and close protected CI/artifact verification.
- [x] Keep the overall Vermory goal active after W13.
