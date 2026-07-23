# Vector Degradation Attribution Design

Date: 2026-07-15

Status: measured; initial HNSW hypothesis rejected

## Question

Why did W12 report 412 degraded results among 550 requested vector queries even
though all 100,000 vectors existed and every task returned the correct memory?

## Rejected Hypothesis

The first hypothesis blamed high-selectivity tenant and continuity filtering
against HNSW. It was plausible from pgvector's documented post-scan filtering
behavior, but it did not survive repository-specific evidence.

Production-query `EXPLAIN` on the 100,000-vector fixture selected the existing
scope B-tree and exact sort rather than the HNSW index. More importantly, a
100,000-vector control with no concurrent authority changes completed 550 of
550 requested vector queries as effective vector results with zero degradation.
No iterative-scan production change is justified by W12.

## Confirmed Cause

The W12 schedule contains:

- 50 vector requests before deletion starts;
- 450 mixed-window vector requests while 1,000 deletes create projection events
  and tail workers catch up;
- 50 vector requests after every tenant returns to zero lag.

Vermory intentionally refuses vector delivery while `ProjectionStatus.Lag` is
non-zero and returns exact lexical results with failure code `projection_lag`.
The instrumented replay measured:

```text
requested vector: 550
effective vector: 145
projection_lag fallback: 405
other degraded: 0
```

The effective/lag split may vary with scheduling. The stable contract is that
the two categories sum to every requested vector query and no other degraded
path appears.

## Product Decision

- Keep the current vector query and pgvector settings unchanged.
- Keep lexical as the product default.
- Keep the projection-current gate and exact lexical fallback.
- Record failure-code distributions in future scale evidence.
- Treat a healthy current projection separately from a projection that is
  intentionally behind active authority changes.

## Non-Claims

- This does not prove general semantic quality.
- This does not rank embeddings or ANN algorithms.
- This does not qualify one million vectors.
- This does not remove the need for future query-planner and recall monitoring.
