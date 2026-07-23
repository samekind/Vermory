# Retrieval Profile Promotion Decision

Date: 2026-07-15

Status: H-011 supported; candidate profile retained without promotion

## Scope

This evidence closes the first measured cutover decision for versioned semantic
projection generations. It compares the registered active and candidate
profiles on the same W10 governed authority and query set through Vermory's
production projection worker, `memory_vector_documents`, retrieval
coordinator, audit path, and reset/rebuild flow:

| Profile | Model | Registry status |
|---|---|---|
| `siliconflow-bge-m3-1024-v1` | `BAAI/bge-m3` | `active` |
| `siliconflow-bge-large-zh-1024-v2` | `BAAI/bge-large-zh-v1.5` | `candidate` |

The comparison is a deployment-profile migration decision. It is not a general
model ranking and does not restrict which chat or coding models Vermory can
serve.

## Frozen Promotion Policy

A candidate can be promoted only when all safety and projection gates pass,
query coverage is identical, quality stays within the frozen regression
limits, latency stays within the calibrated bound, and the candidate has a
clear quality or latency benefit.

| Gate | Threshold |
|---|---:|
| Forbidden, ineligible, degraded results | `0` |
| Projection lag | `0` |
| Reset/rebuild result equivalence | required |
| Maximum Recall@K regression | `0.0000` |
| Maximum MRR regression | `0.0200` |
| Maximum nDCG@K regression | `0.0200` |
| Maximum P95 ratio | `1.25` |
| Maximum P95 absolute increase | `75 ms` |
| Minimum clear Hit@1 gain | `0.0200` |
| Minimum clear MRR gain | `0.0200` |
| Minimum clear P95 improvement | `15%` |

The policy does not permit a latency improvement to compensate for a quality
regression outside the frozen limits.

## Formal Run

The formal run used a fresh dedicated PostgreSQL 18 database and the full
implementation revision
`47b39f674a5ec46d1404f2e8ecf96b467fb3aece`.

| Field | Value |
|---|---|
| Run | `w10-profile-comparison-20260715-v4` |
| W10 corpus version | `2` |
| Corpus SHA-256 | `720707b1c2d01fd428132ac371139ed2391855221acba34c2b42d7de3da1ed51` |
| PostgreSQL schema | `15` |
| Governed lifecycle rows | `30 active / 3 proposed / 3 superseded / 3 deleted` |
| Vector rows | `30` per profile |
| Retrieval audits | `36` per profile |
| Embedding requests | `96` per profile |
| Degraded or failed retrieval audits | `0` |
| Report SHA-256 | `4316d9b9761e5cf0af14bfd388a1f64b4e7170965d5669206c643e93c9b3e26d` |

Each profile made 30 initial projection requests, 18 query requests, 30 rebuild
requests, and 18 post-rebuild query requests. Both profiles reached zero lag,
kept exactly 30 active vector rows, and returned byte-stable record-ID order
after reset and rebuild.

| Profile | Hit@1 | Recall@K | MRR | nDCG@K | P50 | P95 | Build |
|---|---:|---:|---:|---:|---:|---:|---:|
| v1 active | `1.0000` | `1.0000` | `1.0000` | `0.9919` | `111.449 ms` | `118.283 ms` | `3.415 s` |
| v2 candidate | `0.8889` | `1.0000` | `0.9352` | `0.9416` | `105.803 ms` | `125.794 ms` | `3.625 s` |

The candidate preserved complete Recall@K but missed the relevant fact at rank
1 on two queries: shopping delivery and the Chinese thesis-method query. Its
Hit@1 delta was `-0.1111`, MRR delta was `-0.0648`, and nDCG@K delta was
`-0.0503`. MRR and nDCG therefore exceeded the allowed regression by more than
three and two times respectively.

Machine-readable formal evidence:
[v4 qualified report](snapshots/retrieval-profile-comparison/2026-07-15-v4-qualified-full-revision.json).

## Repeatability And Latency

Two additional corrected-corpus runs used the same implementation and produced
the exact same quality values and promotion decision:

| Run | v1 P95 | v2 P95 | v1 build | v2 build | Decision |
|---|---:|---:|---:|---:|---|
| `v2` | `264.863 ms` | `144.565 ms` | `3.705 s` | `4.907 s` | `keep_candidate` |
| `v3` | `132.958 ms` | `116.684 ms` | `3.484 s` | `3.858 s` | `keep_candidate` |
| `v4` | `118.283 ms` | `125.794 ms` | `3.415 s` | `3.625 s` | `keep_candidate` |

The remote P95 direction changed across runs, while the quality deltas remained
identical. The candidate projection build was slower in all three runs. The
evidence therefore records latency as measured provider behavior, not as a
stable candidate advantage.

Repeat snapshots:

- [v2 qualified report](snapshots/retrieval-profile-comparison/2026-07-15-v2-qualified.json)
- [v3 qualified repeat](snapshots/retrieval-profile-comparison/2026-07-15-v3-qualified-repeat.json)

## Preserved Classification Failure

The first run correctly failed its declared hard gate because the W10 v1 corpus
had classified the active same-scope shopping-budget record as forbidden for
the order-date query. That contradicted W10's own contract: ordinary
same-scope distractors affect ranking quality, while forbidden IDs are reserved
for isolation, lifecycle, deletion, or explicit task exclusions.

Corpus version 2 removed only that erroneous forbidden label. It did not alter
the record, query text, relevant set, model output, or returned order. The
validator now rejects same-scope active forbidden records unless the corpus
also declares them in `task_excluded_record_ids`. The failed attempt remains
available as [v1 classification failure](snapshots/retrieval-profile-comparison/2026-07-15-v1-classification-failure.json).

## Decision

- Keep `siliconflow-bge-m3-1024-v1` active and as the runtime default.
- Keep `siliconflow-bge-large-zh-1024-v2` registered as a candidate.
- Do not mutate PostgreSQL authority or delete either rebuildable projection.
- Accept H-011's versioned-generation mechanism as supported: parallel build,
  profile-specific retrieval and audit, rollback, quality comparison, and an
  explicit cutover decision are now executable and evidenced.
- Require a new corpus and a new recorded decision before any future candidate
  promotion; do not reinterpret this result as a permanent model ranking.

## Non-Claims

- This does not switch the product's lexical default to semantic retrieval.
- This does not qualify arbitrary embedding dimensions or automatic cutover.
- This does not establish stable provider latency from three short runs.
- This does not replace scale, backlog, restart, sealed-evaluation, signing, or
  final release gates.
