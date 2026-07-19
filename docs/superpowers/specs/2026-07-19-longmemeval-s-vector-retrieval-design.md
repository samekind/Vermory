# LongMemEval-S Production Vector Retrieval Qualification Design

## Purpose

W28 measures Vermory's registered production vector retrieval path over every
record in the pinned cleaned LongMemEval-S artifact. W14 and W15 established a
real quality deficit for the current lexical path: at K=10, the deterministic
token-overlap baseline reached `0.8383` RecallAll and `0.7580` reader accuracy,
while Vermory lexical reached `0.7340` RecallAll and `0.6820` reader accuracy.

W28 asks one bounded question:

> When the same governed session authority is projected by the registered
> production embedder and queried through the production retrieval
> coordinator, what evidence-session ranking does Vermory deliver, and does it
> do so without degradation, lifecycle leakage, or continuity leakage?

This is a retrieval qualification, not a model competition and not a default
retrieval cutover. PostgreSQL remains the sole semantic authority. Embeddings
remain rebuildable projections.

## Frozen Inputs

W28 reuses the exact W14 upstream source contract:

| Field | Frozen value |
|---|---|
| Dataset | LongMemEval-S cleaned |
| Dataset revision | `98d7416c24c778c2fee6e6f3006e7a073259d48f` |
| Artifact SHA-256 | `d6f21ea9d60a0d56f34a05b609c79c88a451d2ae03597821ea3d5a9678c3a442` |
| Records / scored / abstention | `500 / 470 / 30` |
| Sessions / turns | `23,867 / 246,750` |
| Sorted record-ID SHA-256 | `f038965c54b03632f86a59104dd77848b66e3f80c08d5fbabdd3984d16457811` |
| Retrieval limit | `12` |
| Reported prefixes | K=5, K=10, K=12 |

The new execution manifest has a distinct run identity and exactly three
conditions:

1. `plain_token_overlap`
2. `vermory_lexical`
3. `vermory_vector`

The first two conditions retain the W14 algorithms. The third condition must
come from `RetrievalCoordinator.Retrieve` with `Mode=vector`; direct SQL,
in-memory cosine ranking, or a benchmark-only vector store cannot count.

## Frozen Vector Profile

The versioned profile is stored at
`casebook/benchmarks/profiles/longmemeval-s-vector-retrieval-v1.json` and binds:

| Field | Value |
|---|---|
| Provider path | SiliconFlow direct, no NewAPI |
| Retrieval profile | `siliconflow-bge-m3-1024-v1` |
| Model | `BAAI/bge-m3` |
| Dimensions | `1024` |
| Projection class | `vector_1024` |
| Event batch | `256` |
| Embedding batch | `16` texts per provider request |
| HTTP timeout | `120 seconds` |
| Attempts | at most `3` per provider operation |
| Retry delay | `2 seconds` |

The initial v1 retry envelope is retained as an executed historical profile.
The first proper one-shot full run projected 1,280 current vectors and then
failed closed after three consecutive transient embedding failures. A
read-only diagnostic immediately replayed the complete failing 256-event
window as sixteen independent 16-item requests; every request returned HTTP
200 with 16 vectors of 1024 dimensions, including the 31,855-byte longest
input. This falsified deterministic input-size and batch-shape explanations.

The formal retry profile is therefore revised, without changing retrieval
conditions or quality labels, at
`casebook/benchmarks/profiles/longmemeval-s-vector-retrieval-v2.json`:

| Field | v2 value |
|---|---|
| Maximum attempts | `5` |
| Retry delay | `2 seconds` base |
| Retry backoff | linear: `2 / 4 / 6 / 8 seconds` |

The v1 file and failed runs remain unchanged evidence. v2 is an operational
fault-recovery revision, not benchmark-label tuning.

The profile contains no credential. The CLI accepts only the name of an
environment variable and reads the secret inside the process. The profile's
endpoint, model, dimensions, and projection class must match the registered
runtime profile exactly; runtime flags cannot silently replace them.

## Batch Embedding Contract

The existing single-text `Embed` contract remains valid. W28 adds an optional
`BatchEmbedder` capability:

- embedders without batch support continue one text at a time;
- batch use is opt-in through `EmbeddingBatchSize`;
- responses are reordered by the provider's explicit `index` field;
- missing, duplicate, out-of-range, or wrong-dimensional vectors fail the
  complete batch;
- no partial batch advances the projection cursor;
- authority is rechecked before each vector mutation;
- projection events remain resumable and idempotent;
- neither input text nor provider error bodies enter public evidence.

This is a production runtime capability used by W28, not a benchmark-side
shortcut. It applies to durable event projection and current-authority rebuilds.

## Execution Phases

### 1. Governed import

The runner streams all 500 records into one dedicated benchmark tenant. Each
record receives one conversation continuity and each timestamped session is
committed as one active governed source memory. Duplicate upstream session IDs
remain distinct occurrences exactly as in W14.

### 2. Durable vector projection

After import, the registered production projection worker drains the tenant's
durable projection events. It uses the frozen event and embedding batch sizes.
The phase is complete only when:

- cursor status is `idle`;
- projection lag is zero;
- vector count equals the number of active governed session memories;
- no rebuild-required or terminal provider failure remains.

### 3. Three-condition retrieval

The source is streamed again. For every record, the runner reconstructs the
official occurrence mapping from idempotent governed receipts, executes the
same token-overlap baseline, calls the production lexical coordinator path,
and calls the production vector coordinator path with a stable operation ID.

Every returned memory must belong to the record's active authority and map to
an official session occurrence. A vector result that degrades to lexical is
retained with its failure code and fails qualification.

### 4. Deterministic scoring

The runner applies the existing deterministic LongMemEval session metrics at
K=5, K=10, and K=12. It reports aggregate and question-type RecallAny,
RecallAll, nDCG, MRR, classification counts, latency, projection state, and
embedding request/item counts.

## Failure And Resume Semantics

Every formal run has a unique run ID, tenant, database, and artifact root.
Atomic per-record checkpoints remain available for query-phase resume, but a
formal qualification claim requires one complete run with internally
consistent request accounting. An interrupted run, a terminal provider
failure, a projection failure, or a degraded query is retained as append-only
evidence and cannot be relabeled as a clean run.

Retries are bounded by the frozen profile. Transient failed attempts are
counted separately from terminal failures. Retrying must never advance the
projection cursor or create duplicate governed memories.

## Hard Gates

W28 is execution-qualified only when all of the following hold:

1. The exact pinned dataset and all 500 records execute.
2. All 470 non-abstention records have all three deterministic condition scores.
3. Governed import produces exactly 23,867 active source memories in 500 isolated conversation continuities.
4. The vector projection is `idle`, has zero lag, and contains exactly 23,867 current vectors.
5. Every vector query is effective `vector`; degraded queries are zero.
6. Terminal embedding/provider failures are zero and all attempts are counted.
7. Cross-tenant, cross-continuity, unmapped, non-active, superseded, archived, deleted, and redacted retrieval results are zero.
8. Query and projection operation identities remain idempotent under verified replay.
9. Old W14 lexical execution remains accepted without embedding configuration.
10. PostgreSQL, race, vet, repository policy, release, and protected CI gates pass.

Retrieval quality is a measured outcome, not a hard-coded pass threshold. A
vector score below the lexical or token-overlap baseline is a valid negative
result and must not be hidden.

## Decision Rule

W28 does not alter the product default. After the rankings are frozen:

- if vector materially improves complete-evidence retrieval without a hard-gate
  failure, a later reader run may consume the frozen K=10 vector ranking;
- if vector does not improve retrieval, the failure cohorts become input to a
  separately frozen hybrid/reranking experiment;
- the same public labels cannot be used both to tune a new algorithm and claim
  unbiased qualification without disclosure and a held-out evaluation.

## Non-Claims

- W28 is not a full LongMemEval QA score.
- W28 does not use an LLM judge.
- W28 does not evaluate LongMemEval-M.
- W28 does not prove automatic memory formation from raw chats.
- W28 does not rank language models or embedding providers.
- W28 does not switch Vermory's default retrieval mode.
- W28 is public benchmark evidence, not sealed or externally withheld evidence.
- W28 does not complete the overall Vermory platform goal.
