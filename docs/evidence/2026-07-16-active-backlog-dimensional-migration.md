# Active-Backlog Dimensional Migration Qualification Evidence

Date: 2026-07-16

Revision under test: `86fb8d0a25fefa4efe47a0b4618c139facafde16`

Formal run ID: `active-backlog-dimension-20260716-v3`

## Scope

This evidence covers the frozen `W17-active-backlog-dimensional-migration`
case. It qualifies one explicit coexistence path:

```text
incumbent  siliconflow-bge-m3-1024-v1              vector_1024   active
candidate  siliconflow-qwen3-embedding-4b-2560-v3  halfvec_2560  candidate
```

The run exercised:

- 20,000 current governed facts across four tenants and twenty continuities;
- an incumbent 1024-dimensional snapshot and a separate 2560-dimensional
  candidate snapshot;
- 2,000 revisions, 500 deletions, and 500 new facts while candidate work was
  active, producing exactly 5,000 projection-tail events;
- 16 concurrent incumbent query clients and 320 scoped vector requests;
- PostgreSQL `immediate` restart while candidate embedding was blocked;
- recovery through the same incumbent and candidate pgx pools;
- profile-scoped cursor convergence, deletion safety, reset, and rebuild;
- a direct SiliconFlow projection and query probe using
  `Qwen/Qwen3-Embedding-4B`.

PostgreSQL remained the only authority. The candidate was not promoted, the
incumbent remained active, and lexical remained the product default.

## Physical Projection Decision

The provider returned 2,560 dimensions for the selected model. A fresh schema
run proved pgvector 0.8.5 rejects HNSW indexes on `vector` columns above 2,000
dimensions. pgvector documents a 4,000-dimensional HNSW limit for `halfvec`,
and a focused PostgreSQL probe successfully created a
`halfvec(2560) halfvec_cosine_ops` index, inserted a 2,560-dimensional value,
and executed cosine search.

The qualified candidate class is therefore `halfvec_2560`. No model dimension
was truncated. The half-precision storage boundary is explicit in the profile
registry, schema constraint, typed table, fixed SQL routing, and evidence.

## Environment

```text
Host: darwin/arm64, Apple M4 Pro, 48 GiB
PostgreSQL: 18.4
pgvector: 0.8.5
Schema: 16
Authority: PostgreSQL
Candidate provider: direct SiliconFlow OpenAI-compatible embeddings
```

The profile used one disposable PostgreSQL cluster bound only to loopback. It
did not inspect, stop, copy, or modify an existing PostgreSQL service or user
data directory.

## Formal Result

| Measurement | Result |
|---|---:|
| initial current facts | `20,000` |
| revisions / deletions / new facts | `2,000 / 500 / 500` |
| migration-tail events | `5,000` |
| final current facts | `20,000` |
| final lexical rows | `20,000` |
| final incumbent vectors | `20,000` |
| final candidate vectors | `20,000` |
| final candidate lag | `0` |
| incumbent query clients / samples | `16 / 320` |
| successful scoped queries | `320` |
| cross-scope results | `0` |
| query p50 / p95 / p99 | `12 / 20 / 30 ms` |
| authority writer duration | `2,258 ms` |
| authority completed while candidate blocked | `true` |

The deterministic scale embedders qualified storage, routing, worker, cursor,
restart, and lifecycle mechanics. These measurements are not an embedding
quality comparison.

## Restart And Recovery

The harness blocked candidate embedding after each worker had loaded current
authority, then stopped only the dedicated PostgreSQL cluster with
`immediate`. The observed result was:

| Gate | Result |
|---|---:|
| partial candidate rows committed | `0` |
| interrupted cursor advance | `0` |
| bounded database-unavailable request | `1` |
| false result or audit receipt during outage | `0` |
| same pools recovered | `true` |
| measured recovery after restart | `13 ms` |

After restart, both projection classes drained to zero lag and contained the
same 20,000 current eligible memory IDs with current authority hashes.
Superseded, deleted, redacted, proposed, cross-tenant, and cross-continuity
records remained absent from effective results.

## Candidate Reset And Rebuild

For one tenant, the candidate projection was reset independently:

| Gate | Result |
|---|---:|
| candidate rows immediately after reset | `0` |
| incumbent rows unchanged | `true` |
| governed authority unchanged | `true` |
| candidate rebuilt from current authority | `true` |

The rehearsal did not change profile lifecycle state and did not activate an
automatic rollback or promotion policy.

## Direct Provider Probe

The direct provider probe made exactly two requests:

1. project one governed fact through the candidate worker;
2. embed a paraphrased query and retrieve that fact through the production
   coordinator.

| Measurement | Result |
|---|---:|
| provider | `https://api.siliconflow.cn/v1` |
| model | `Qwen/Qwen3-Embedding-4B` |
| dimensions | `2,560` |
| requests | `2` |
| total provider duration | `317 ms` |
| projection response SHA-256 | `a5605dabd67ae93092dacf2095ff707608c4e8270f481d853f3ead07e3746996` |
| query response SHA-256 | `4d77c06da93c5daac4be59e0da7755113a5e42331d9480d9d50acb039d1c1b57` |

No credential, raw vector, or provider response body is present in the report
or committed evidence.

## Deterministic Replay

The first v3 invocation completed in `407.59s`. A second invocation used the
same run ID without a provider credential, found no running profile PostgreSQL
process, returned the completed report in `2s`, and did not start a cluster or
call the provider. JSON and Markdown hashes were identical before and after
replay:

| Artifact | SHA-256 |
|---|---|
| frozen case | `ebab12a98f6b848f37296bb30e6c8261de3194341935cf2823d267bda9278e34` |
| report JSON | `afa72b3387b901dfccdbe8ab2135fb41d031b107dc1abcd65e66e7b7119a72d0` |
| generated report Markdown | `7af84c9b11cc20310ce31d473385aa34ced4b0c59243c44c65e750ad5b8924e8` |

The committed raw snapshot is
[2026-07-16-active-backlog-dimensional-migration.json](snapshots/2026-07-16-active-backlog-dimensional-migration.json).

## Failure Ledger

Failures were retained and changed the implementation or frozen tuple rather
than being deleted or relabeled:

| Phase | Attempt | Failure | Resolution |
|---|---:|---|---|
| provider preflight wrapper | 1 | zsh rejected the read-only variable `status`; no HTTP request was sent | corrected the wrapper and did not count it as a provider result |
| provider preflight | 2 | `BAAI/bge-small-zh-v1.5` returned HTTP 400, provider code 20012, model does not exist | probed available non-Pro models and froze Qwen3-Embedding-4B/2560 |
| fresh schema qualification | 1 | pgvector rejected `vector(2560)` HNSW with SQLSTATE 54000 | qualified explicit `halfvec_2560` storage and index routing |
| formal run v1 | 1 | Homebrew version suffix was parsed as the PostgreSQL version | added vendor-suffix parser tests and consumed a new run ID |
| formal run v2 audit | 1 | unsorted concurrent latency samples produced non-monotonic `10/7/5 ms` percentiles | added sorting and validator monotonicity gates; v2 remains local failed evidence |
| v3 restart injection | 1 | dedicated PostgreSQL unavailable during blocked candidate embedding | expected failure retained; zero partial row, cursor advance, false result, or receipt |

The v1 and v2 run IDs were not reused. Their local roots and logs remain
outside Git for audit.

## Hard Gates

All twelve frozen hard gates passed:

- schema classes isolated;
- authority writes provider-independent;
- incumbent served during candidate backlog;
- restart committed no partial candidate vector;
- same pools recovered;
- tail event count exact;
- both physical classes converged;
- ineligible rows absent;
- candidate reset isolated;
- real provider returned 2,560 dimensions;
- retrieval audits remained profile-scoped;
- incumbent/default profile remained unchanged.

## Non-Claims

- This is not an embedding-model ranking or promotion decision.
- The 2560-dimensional candidate is not the product default.
- Half-precision operational qualification is not a semantic-quality result.
- This is not long-duration retention or event-pruning evidence.
- This is not cross-host HA or cross-region evidence.
- This is not an external sealed evaluation.
- This is not artifact signing or final release acceptance.
