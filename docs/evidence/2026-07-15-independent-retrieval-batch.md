# W10 Independent Retrieval Batch Evidence

Date: 2026-07-15

Status: completed as a second independent retrieval-quality batch

## Scope

W10 is independent from W08 in corpus composition and query wording. It covers
software release work, thesis research, home maintenance, and household
purchasing across six workspace/conversation scopes and four tenants. The
frozen corpus contains 39 records:

| Lifecycle | Count |
|---|---:|
| active | 30 |
| proposed | 3 |
| superseded | 3 |
| deleted | 3 |

The 18 queries cover exact identifiers, paths, feature flags, error codes,
Chinese semantic requests, English paraphrases, mixed-language requests,
dates, durations, numeric constraints, multi-fact retrieval, technical
commands, and continuity isolation.

The corpus and its validation test are committed in
`runtime/cases/W10-independent-retrieval-batch`.

## Execution

The run used a fresh PostgreSQL 18 database, Vermory's authoritative runtime
seeding path, and the direct SiliconFlow OpenAI-compatible embedding endpoint.
Mac mini NewAPI was not used. No chat model or LLM judge was used for this
retrieval-quality run; the scored conditions were deterministic retrieval
conditions over the same governed authority.

| Field | Value |
|---|---|
| Run | `w10-siliconflow-bge-m3-20260715-v5` |
| Implementation | `0526ef34f243ad330654b6fb6b65a318c182ad82` |
| PostgreSQL schema | `15` |
| Embedding endpoint | `https://api.siliconflow.cn/v1` |
| Embedding model | `BAAI/bge-m3` |
| Dimensions | `1024` |
| Embedding requests | `102` |
| Corpus SHA-256 | `6a615f06a0598556b566e10bb089d506282990cceaedf91c8188ae6bbbe40f37` |
| Authority fingerprint | `df27021d34a75ae30043bba4d04f20af58478d2c6756400641a490586ef7739d` |
| Database | dedicated `vermory_w10_clean` |

## Results

| Condition | Hit@1 | Recall@K | MRR | nDCG@K | P95 | Forbidden | Ineligible |
|---|---:|---:|---:|---:|---:|---:|---:|
| `lexical_runtime` | 0.7222 | 0.7593 | 0.7500 | 0.7353 | 3.894 ms | 0 | 0 |
| `vector_pg` | 1.0000 | 1.0000 | 1.0000 | 0.9919 | 154.697 ms | 0 | 0 |
| `hybrid_rrf` | 1.0000 | 1.0000 | 1.0000 | 0.9908 | 154.953 ms | 0 | 0 |

All hard gates passed. The vector projection was rebuilt from authoritative
records and produced equivalent result IDs. A second invocation with the same
run identity returned `replayed=true` without reseeding or overwriting the
existing report.

## Interpretation

This batch strengthens the evidence that a direct pgvector projection can
improve semantic and mixed-language retrieval over the current lexical path
while preserving lifecycle and tenant boundaries. It does not establish that
the current RRF formula adds value: W10 again matched vector quality and added
latency. H-009 therefore remains `testing/measured`, with no product-default
switch.

The first three local attempts were excluded from qualification because they
reused one authority database across different run IDs. That exposed two
correctness defects: duplicate seeded records could contaminate a later run,
and the report did not include forbidden results in `hard_gates.pass`. The
dedicated-database execution contract, forbidden hard-gate fix, and duplicate
metric fix were committed before the qualified W10 run. The discarded reports
remain local diagnostic artifacts and are not used as evidence.

## Corpus Revision Note

The qualified v5 report remains evidence for the exact corpus version 1 bytes
and SHA-256 recorded above. The later profile-migration comparison found one
scoring-label defect: the order-date query marked the active same-scope
shopping-budget record as zero-tolerance forbidden. Corpus version 2 removes
only that label and adds a validator requiring explicit
`task_excluded_record_ids` before an active same-scope fact can be forbidden.
The v5 bge-m3 result did not return the budget record inside the query limit, so
its metrics and hard-gate outcome are unchanged; future W10 executions use
version 2 and its new SHA-256. See the
[profile promotion decision](2026-07-15-retrieval-profile-promotion-decision.md).

## Non-Claims

- This is not a full benchmark score or a sealed evaluation.
- This does not rank or select an LLM provider.
- This does not qualify production scale, embedding migration, or long-running fault behavior.
- This does not establish source-authority ranking.
- This does not switch the default runtime from lexical to vector or hybrid retrieval.

Machine-readable evidence: [W10 report JSON](snapshots/2026-07-15-independent-retrieval-batch-report.json)

Deterministic rendering: [W10 report Markdown](snapshots/2026-07-15-independent-retrieval-batch-report.md)
