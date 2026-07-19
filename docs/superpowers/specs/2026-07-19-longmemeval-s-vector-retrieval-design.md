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

The proper one-shot v2 run then projected 2,560 of 23,867 current vectors and
failed after one logical embedding operation exhausted all five linearly
delayed attempts. It executed no retrieval queries and contributes no score.
Together with the successful read-only replay of the v1 failing window, this
shows that operation-local retries alone do not cover transient provider
availability across the full qualification.

The next append-only operational profile is therefore
`casebook/benchmarks/profiles/longmemeval-s-vector-retrieval-v3.json`:

| Field | v3 value |
|---|---|
| Maximum attempts per embedding operation | `5` |
| Attempt delay | linear: `2 / 4 / 6 / 8 seconds` |
| Maximum projection recoveries | `10` |
| Projection recovery cooldown | `30 seconds` |

A projection recovery is available only when `ProjectionWorker.RunOnce`
returns the structured `embedding_unavailable` failure. The failed worker pass
leaves its PostgreSQL cursor at the last committed event with status `failed`.
After the cooldown, the same process invokes `RunOnce` again; the existing
worker state machine changes the cursor back to `running` and resumes
idempotently. Other failure codes, an already-running worker, context
cancellation, and an exhausted recovery budget fail closed without recovery.

Evidence keeps three counts separate:

- `embedding.terminal_failures` is the number of logical embedding operations
  that exhausted all operation-local attempts;
- `recovered_projection_failures` is the number of those projection failures
  followed by a successful worker pass;
- `unrecovered_projection_failures` is the number still unresolved when the
  projection phase ends, and must be zero for qualification.

The final hard gate permits exhausted operations only when their count equals
the recovered projection-failure count. A query-phase embedding exhaustion
therefore cannot be misclassified as a recovered projection failure: it still
produces a degraded audited query and fails the qualification. v3 changes no
retrieval condition, ranking label, source data, or production default.

The exact-head v3 full run then reproduced the earlier provider boundary at
2,560 committed vectors. Same-process recovery correctly moved the cursor from
`failed` back to `running` without restarting the LaunchAgent, but all ten
30-second recovery windows eventually exhausted. The run retained 500
continuities, 23,867 active memories, 2,560 matching vectors, zero retrieval
audits, and no partial cursor advancement; it contributes no score.

A post-failure diagnostic sent the same event 2,561 as a one-item request and
events 2,561-2,576 as one 16-item request. Both returned HTTP 200 with the
expected 1 and 16 vectors at 1,024 dimensions. Payloads and response bodies
were deleted; retained evidence contains only status, byte counts, dimensions,
and response hashes. This again falsifies invalid content, deterministic batch
shape, and a persistent account block.

Code inspection identified the remaining progress defect. A 256-event worker
pass calls the 16-item embedding provider up to sixteen times before
`prepareEvents` returns. Event transactions and cursor advancement begin only
after all sixteen calls succeed. A transient failure near the end therefore
discards the successful provider-call prefix, and every recovery repeats that
prefix before reaching the failing operation.

The next append-only profile is
`casebook/benchmarks/profiles/longmemeval-s-vector-retrieval-v4.json`:

| Field | v4 value |
|---|---|
| Worker batch | `16` events |
| Embedding batch | `16` texts |
| Maximum attempts per embedding operation | `5` |
| Attempt delay | linear: `2 / 4 / 6 / 8 seconds` |
| Maximum projection recoveries | `10` |
| Projection recovery cooldown | `30 seconds` |

v4 aligns one provider operation with one durable worker pass. Once the
provider returns sixteen valid vectors, those sixteen events are authority
rechecked and committed before another provider operation begins. A later
failure can repeat only its own uncommitted operation, not up to fifteen
already successful provider calls. Tests inject a failure after one committed
provider batch and prove that the committed vector/cursor prefix remains
visible during recovery. v4 does not weaken atomic provider-response
validation, increase retry budgets, or change any retrieval label.

The exact-head v4 run validated that durable alignment: it committed 2,592
vectors, beyond the earlier 2,560 boundary, before the next one-event worker
pass exhausted its recovery budget. It executed no retrieval query and
contributes no score. A post-run diagnostic then replayed the exact physical
operation for events 2,593-2,608. The 16-item request contained 174,727 input
bytes and returned HTTP 400 with provider code `20015`. Individual replay
returned HTTP 200 and one 1,024-dimensional vector for 15 items; the remaining
43,406-byte item returned the same HTTP 400 and provider code. Response bodies,
headers, credentials, and input payloads were not retained.

This identifies a deterministic per-input provider limit. More retries cannot
make the operation valid, and silent truncation would change the governed
memory's meaning. The next append-only profile is therefore
`casebook/benchmarks/profiles/longmemeval-s-vector-retrieval-v5.json`:

| Field | v5 value |
|---|---|
| Retrieval profile | `siliconflow-bge-m3-1024-chunked-mean-v2` |
| Lifecycle | registered candidate; production default remains v1 |
| Worker batch | `1` logical memory |
| Provider batch | at most `16` physical chunks |
| Input policy | `utf8-byte-chunks-v1` |
| Maximum chunk size | `7,500` bytes |
| Chunk overlap | `500` bytes |
| Pooling | arithmetic mean in float64, then L2 normalization |

The chunk boundary is moved only to a valid UTF-8 boundary. Invalid UTF-8,
non-progressing policies, wrong vector counts or dimensions, and zero-norm
pooled vectors fail closed. No source bytes are dropped. One governed memory
still produces one rebuildable vector document with the full source-content
hash, and query embeddings use the same input policy. A single-chunk input
preserves the provider vector unchanged, so the candidate introduces no
unnecessary normalization on short inputs.

The profile identity includes the transformation policy. The v1-v4 profile
tuples retain a zero input policy and their original runtime behavior. The
candidate is stored in PostgreSQL as `candidate`; this public qualification
cannot activate it or change Vermory's default retrieval profile.

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
- each physical input obeys the registered UTF-8 and byte-bound contract;
- physical chunks are transient request material and are not new authority or
  separately stored memories;
- one logical memory or query is counted as successful only after all of its
  physical chunks produce one valid logical vector.

This is a production runtime capability used by W28, not a benchmark-side
shortcut. It applies to durable event projection and current-authority rebuilds.

## Execution Phases

### 1. Governed import

The runner streams all 500 records into one dedicated benchmark tenant. Each
record receives one conversation continuity and each timestamped session is
committed as one active governed source memory. Duplicate upstream session IDs
remain distinct occurrences exactly as in W14.

### 2. Durable vector projection

After import, the production projection worker drains the tenant's durable
projection events using the retrieval profile selected by the frozen W28
profile. It uses the frozen event and embedding batch sizes.
The phase is complete only when:

- cursor status is `idle`;
- projection lag is zero;
- vector count equals the number of active governed session memories;
- every physical provider input obeys the selected profile's byte limit;
- successful logical and physical item counts equal their independently
  computed expectations;
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
embedding operation, logical-item, and physical provider-item counts.

## Failure And Resume Semantics

Every formal run has a unique run ID, tenant, database, and artifact root.
Atomic per-record checkpoints remain available for query-phase resume, but a
formal qualification claim requires one complete run with internally
consistent request accounting. An interrupted run, a terminal provider
failure, a projection failure, or a degraded query is retained as append-only
evidence and cannot be relabeled as a clean run.

Retries and projection recoveries are bounded by the frozen profile. Transient
failed attempts, exhausted logical operations, recovery cooldowns, recovered
projection failures, and final unrecovered failures are counted separately.
Retrying must never advance the projection cursor or create duplicate governed
memories.

## Hard Gates

W28 is execution-qualified only when all of the following hold:

1. The exact pinned dataset and all 500 records execute.
2. All 470 non-abstention records have all three deterministic condition scores.
3. Governed import produces exactly 23,867 active source memories in 500 isolated conversation continuities.
4. The vector projection is `idle`, has zero lag, and contains exactly 23,867 current vectors.
5. Every vector query is effective `vector`; degraded queries are zero.
6. Unrecovered embedding/provider failures are zero; every exhausted operation, recovery cooldown, attempt, and item is counted.
7. Successful logical embedding items equal 23,867 memories plus 500 queries, and successful physical provider items exactly equal the count independently derived with the registered input policy.
8. Every physical candidate-profile input is valid UTF-8 and no larger than 7,500 bytes; truncation is forbidden.
9. Cross-tenant, cross-continuity, unmapped, non-active, superseded, archived, deleted, and redacted retrieval results are zero.
10. Query and projection operation identities remain idempotent under verified replay.
11. Old W14 lexical execution and v1-v4 retrieval profiles remain accepted without input-policy drift.
12. PostgreSQL, race, vet, repository policy, release, and protected CI gates pass.

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
