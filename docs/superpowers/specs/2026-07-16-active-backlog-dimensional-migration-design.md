# Active-Backlog Dimensional Migration Design

Date: 2026-07-16

Status: execution boundary under the active Vermory goal

## Goal

Prove that Vermory can build and operate a candidate embedding projection with
a different physical vector dimension while the incumbent projection continues
to serve requests and authoritative PostgreSQL events continue to arrive.

W17 qualifies one explicit transition:

```text
incumbent: siliconflow-bge-m3-1024-v1 / vector(1024) / active
candidate: siliconflow-qwen3-embedding-4b-2560-v3 / halfvec(2560) / candidate
```

The qualification must cover snapshot bootstrap, active tail backlog,
PostgreSQL immediate restart, retry, deletion and revision races, candidate
reset and rebuild, real direct-provider compatibility, and an explicit decision
to retain the incumbent as the default. PostgreSQL governed memories remain the
only authority throughout the run.

## Why This Slice Comes Next

Migration 15 proves that two 1024-dimensional profiles can keep independent
cursors and rows while sharing one `vector(1024)` table. That is a versioned
generation mechanism, not a dimensional migration. It cannot safely accept a
2560-dimensional vector, and changing the existing column type would make the
incumbent unavailable during rebuild.

W11 proves retry and restart behavior for one profile. W12 proves a 100,000-row
current projection and a 1,000,000-event history. W13 attributes controlled
vector degradation. None of those runs proves that two physical vector classes
can coexist while both consume a live event stream.

This slice closes that specific platform risk without changing the lexical
default, ranking policy, or public benchmark labels.

## Approaches Considered

### A. Independent typed projection tables

Keep the existing 1024-dimensional table and add a separate
`halfvec(2560)` table. The profile registry records a small closed-set projection
class, and Go code maps that class to fixed SQL statements.

Benefits:

- pgvector enforces dimensions at the database boundary;
- each physical class has its own HNSW index and can be rebuilt independently;
- incumbent and candidate rows cannot be mixed accidentally;
- rollback clears only the candidate class;
- table names are never taken from user or database input.

Cost:

- each newly qualified dimension needs an explicit migration and code branch;
- common store behavior must be kept consistent across typed tables.

### B. One unbounded `vector` column with expression indexes

Use an unconstrained pgvector column and partial expression indexes that cast
rows to `vector(1024)` or `halfvec(2560)` according to profile metadata.

This reduces the number of tables but moves dimension safety into predicates,
casts, and index-selection rules. A missing predicate can mix incompatible
profiles or force an unindexed scan. It is not selected for the first
production-shaped dimensional qualification.

### C. Arrays or binary payloads with application-side distance

Store vectors as arrays or bytes and calculate distance in Go. This avoids a
schema branch but discards the existing pgvector query and index contract. It
would test a different retrieval system rather than dimensional migration.

### Decision

Use approach A. Vermory deliberately supports a closed set of qualified
physical projection classes rather than pretending arbitrary dimensions are
safe. A future class is added only after its own migration and evidence run.

## Schema 16 Contract

Migration 16 adds:

```text
memory_retrieval_profiles.projection_class
  allowed values: vector_1024, halfvec_2560

memory_vector_documents_2560
  profile_id       text
  tenant_id        text
  continuity_id    uuid
  memory_id        uuid
  content_sha256   text
  embedding        halfvec(2560)
  updated_at       timestamptz
```

The new table has:

- primary key `(profile_id, tenant_id, memory_id)`;
- a profile foreign key to `memory_retrieval_profiles`;
- tenant-aware continuity and governed-memory foreign keys;
- row-level security using `vermory.tenant_id`;
- a scope index over profile, tenant, continuity, and memory;
- a `halfvec_cosine_ops` HNSW index over the 2560-dimensional embedding;
- no trigger from governed authority and no independent source of truth.

Existing profiles are assigned `vector_1024`. Migration 16 registers exactly
one new candidate:

| Field | Value |
|---|---|
| profile ID | `siliconflow-qwen3-embedding-4b-2560-v3` |
| provider | `https://api.siliconflow.cn/v1` |
| model | `Qwen/Qwen3-Embedding-4B` |
| dimensions | `2560` |
| projection class | `halfvec_2560` |
| lifecycle | `candidate` |

### Provider preflight correction

The first candidate tuple was not accepted on assumption. Direct provider
preflight produced the following retained evidence:

- the first shell wrapper failed before any HTTP request because zsh reserves
  `status` as a read-only variable;
- `BAAI/bge-small-zh-v1.5` then returned HTTP 400, provider error 20012,
  `Model does not exist. Please check it carefully.`;
- `Qwen/Qwen3-Embedding-0.6B` returned 1024 dimensions;
- `Qwen/Qwen3-Embedding-4B` returned 2560 dimensions;
- `Qwen/Qwen3-Embedding-8B` returned 4096 dimensions.

W17 therefore freezes the direct, non-Pro `Qwen/Qwen3-Embedding-4B` profile at
2560 dimensions. It is physically different from the incumbent 1024 class and
avoids the greater storage, HNSW, and rebuild cost of the available 4096 class.
The unavailable 512 model and the pre-request shell failure remain evidence;
they are not deleted, reclassified as provider success, or hidden by a model
substitution.

A fresh migration test then exposed the pgvector 0.8.5 HNSW limit: `vector`
supports at most 2000 indexed dimensions, while `halfvec` supports 4000. The
database rejected `vector(2560) vector_cosine_ops` with SQLSTATE 54000. A
minimal PostgreSQL probe proved `halfvec(2560) halfvec_cosine_ops` can create
the index, store a 2560-dimensional value, and execute cosine search. The
qualified physical class is therefore `halfvec_2560`; no dimension is dropped
and the precision boundary is explicit rather than hidden.

The migration does not activate the candidate, change an existing profile, or
rewrite governed memory. The runtime role receives only the same tenant-scoped
projection privileges it already has for the 1024 class.

Migration down may discard candidate projection rows, candidate cursors, and
candidate retrieval-run rows because they cannot exist in schema 15. It must
not modify governed memories, observations, continuity bindings, bridges,
conversation turns, Global Defaults, credentials, or incumbent 1024 vectors.

## Runtime Projection-Class Boundary

`RetrievalProfileSpec` and `RetrievalProfile` gain a projection class. Profile
validation checks the full immutable tuple:

```text
profile ID
provider base URL
model
dimensions
projection class
```

The application uses a compile-time switch for `vector_1024` and `halfvec_2560`.
No SQL identifier is interpolated from a flag, registry row, provider response,
or user input.

The following operations must route through the selected physical class:

- projection status vector count;
- candidate reset;
- worker delete and upsert;
- current-authority snapshot rebuild;
- vector search;
- projection recovery and authority recheck.

Events and cursors remain profile-scoped and shared across physical classes.
The existing advisory lock remains keyed by tenant and profile, so incumbent
and candidate workers may run concurrently for the same tenant without sharing
a lock or cursor.

Wrong-dimensional provider output fails before any row is written and records
`embedding_dimension_mismatch`. Provider failure does not advance the cursor.
An authority change committed while embedding is in flight still wins.

## Frozen W17 Profile

Add `W17-active-backlog-dimensional-migration` as a public synthetic operations
case. Synthetic content measures migration mechanics, not memory quality.

The reference profile uses:

- PostgreSQL 18.4 and pgvector 0.8.5;
- Darwin arm64 on Apple M4 Pro with 48 GiB memory;
- one dedicated disposable PostgreSQL cluster;
- 4 tenants;
- 5 continuities per tenant;
- 1,000 initial active facts per continuity;
- 20,000 initial active facts;
- 2,000 current-fact revisions during migration;
- 500 deletions during migration;
- 500 new facts during migration;
- 5,000 tail events during migration;
- 16 incumbent query clients and 320 scoped queries;
- independent deterministic 1024- and 2560-dimensional embedders for the scale
  mechanics;
- one separate small tenant for the real direct SiliconFlow 2560-dimensional
  projection and retrieval probe.

The active count returns to exactly 20,000 after 500 deletions and 500 new
facts. A revision creates an absent event for the superseded memory and an
active event for the new revision, so 2,000 revisions create 4,000 events. The
deletions and new facts add 500 events each, giving exactly 5,000 migration-tail
events.

Changing these counts or calibrated limits requires a new case version. A
failed formal run is retained rather than overwritten.

## Execution Trajectory

### Phase 1: Incumbent current state

1. Start a dedicated PostgreSQL 18 cluster and migrate to schema 16.
2. Create the 4-tenant, 20-continuity authority dataset through Vermory's
   observation and governance transactions.
3. Rebuild the lexical projection from current authority.
4. Build the incumbent 1024-dimensional snapshot for every tenant.
5. Prove all incumbent cursors have zero lag and exactly 20,000 incumbent
   vector rows exist.

### Phase 2: Candidate snapshot with active arrivals

1. Start a 2560-dimensional candidate snapshot for each tenant.
2. After every candidate worker has captured its watermark, start the writer
   workload that performs the frozen revisions, deletions, and new facts.
3. Start incumbent tail workers and incumbent vector-query clients while the
   candidate snapshot is still running.
4. Candidate snapshot workers may skip rows changed after their watermark;
   they must not commit stale content.
5. Events after the captured watermark remain pending for candidate tail
   workers.

The writer never waits for either embedding provider. Authority commits and
lexical eligibility remain independent of candidate progress.

### Phase 3: Restart during candidate work

The harness blocks one candidate embedding after the worker has loaded current
authority but before it can commit the 2560-dimensional row. It then stops the
dedicated PostgreSQL cluster with `immediate`.

During the database outage:

- no partial candidate vector may appear;
- no cursor may advance for the interrupted event;
- incumbent requests may return a bounded database-unavailable error;
- a failed request may not return a successful audit receipt or invented
  memory result.

After PostgreSQL restarts, the same incumbent and candidate store pools are
reused. Both worker classes retry without recreating the service process.

### Phase 4: Tail convergence and deletion safety

Incumbent and candidate workers drain their own cursors concurrently. Final
state must satisfy:

- both classes have zero lag for every tenant;
- both classes contain exactly the same 20,000 eligible memory IDs;
- every vector row content hash matches current governed authority;
- all superseded and deleted memory IDs are absent from both classes;
- new facts exist in both classes;
- one row exists per profile, tenant, and active memory;
- no tenant or continuity leakage occurs in either retrieval class.

### Phase 5: Candidate rollback rehearsal

For one tenant, record incumbent row count, cursor, and a retrieval result.
Reset only the 2560-dimensional candidate projection. The reset must produce:

- zero candidate rows for that tenant;
- candidate cursor zero;
- unchanged incumbent row count and cursor;
- unchanged incumbent retrieval eligibility;
- no governed-memory or lexical mutation.

Rebuild the candidate from current authority and drain its tail to zero lag
again. This is a rollback and retry rehearsal, not a production promotion.

### Phase 6: Real direct-provider probe

The formal profile uses `VERMORY_LIVE_EMBEDDING_API_KEY` only from the process
environment and sends no credential to logs, JSON, Markdown, Git, or GitHub.

The probe calls `https://api.siliconflow.cn/v1/embeddings` directly with
`Qwen/Qwen3-Embedding-4B`. It must:

- return exactly one 2560-dimensional vector;
- project one governed fact into `memory_vector_documents_2560`;
- embed one paraphrased query;
- retrieve the expected fact through the production coordinator;
- record only model, dimensions, request count, duration, status category, and
  non-secret response hashes.

If the provider no longer exposes that exact model or dimension, the formal
run fails with `real_provider_model_unavailable`. The harness must not silently
substitute another model, resize a vector, or mark a deterministic fixture as a
real provider result.

## Query-Service Contract

The incumbent 1024-dimensional profile remains active and the product default
throughout W17. The candidate is addressed only by an explicit profile ID in
test and operator paths.

Before the injected database outage, incumbent vector queries must continue
while candidate lag is non-zero. During the database outage, bounded failures
are accepted and recorded. After restart, the same store and coordinator must
resume successful incumbent queries.

Candidate vector requests degrade to lexical with `projection_lag` until their
tenant cursor is current. Candidate results may become effective vector only
after lag reaches zero. No condition in W17 automatically changes profile
lifecycle status or CLI defaults.

## Hard Gates

- Schema 16 contains one isolated 2560-dimensional projection class with RLS,
  tenant-aware foreign keys, and an HNSW cosine index.
- The 2560 candidate cannot write to or query the 1024 table, and the incumbent
  cannot write to or query the 2560 table.
- Initial authority, lexical rows, incumbent rows, event counts, and cursor
  states match the frozen manifest.
- Authority writes complete without waiting for candidate embedding work.
- Incumbent vector queries continue while the candidate snapshot and backlog
  are active, except for the bounded injected PostgreSQL outage.
- The immediate restart commits no partial candidate vector and advances no
  interrupted cursor.
- The same incumbent and candidate pools recover after PostgreSQL restart.
- Revisions, deletions, and new facts create exactly 5,000 tail events.
- Both physical classes converge to the same 20,000 current eligible memory
  IDs with zero lag and matching authority hashes.
- Superseded, deleted, redacted, proposed, cross-tenant, and cross-continuity
  memories never enter an effective vector result.
- Candidate reset and rebuild do not change incumbent rows, cursor, retrieval,
  governed authority, or lexical projection.
- The real direct SiliconFlow probe returns and uses a 2560-dimensional vector.
- Retrieval audits identify the requested profile and never replay one
  profile's operation as another profile.
- `siliconflow-bge-m3-1024-v1` remains active/default and the 2560 profile
  remains candidate.
- No Redis, mem0, MemOS, Supermemory, or second authoritative store is added.
- No existing PostgreSQL service or user data directory is modified.

## Measured Outputs

The normalized report records:

- run ID, case hash, implementation revision, OS, CPU, memory, PostgreSQL, and
  pgvector versions;
- schema version and projection-class registry rows;
- initial and final authority, lexical, event, cursor, and per-class vector
  counts;
- snapshot watermarks, skipped-changed counts, tail lag, and worker results;
- writer throughput and proof that provider work was not in authority
  transactions;
- incumbent query success, bounded outage failure, recovery, effective mode,
  degradation reason, p50, p95, and p99;
- restart timing and interrupted-row/cursor assertions;
- candidate reset isolation and rebuild equivalence;
- real provider model, dimensions, request count, duration, and response hashes;
- every injected or observed failure in chronological order;
- hard-gate booleans and explicit non-claims.

Raw database logs and provider responses remain outside Git. A repeated run ID
with the same request fingerprint may replay a completed normalized report.
Conflicting reuse is rejected.

## Failure Preservation

The following failures are evidence and must not be hidden:

- provider model unavailable or wrong dimensions;
- candidate snapshot or tail provider failure;
- database outage during an incumbent request;
- database restart during candidate embedding;
- stale authority detected after a late embedding completion;
- a calibrated duration or count gate exceeded;
- any cross-class, tenant, continuity, lifecycle, or deletion violation.

The formal run directory is append-only by attempt. A later passing attempt
does not delete an earlier failure or reuse its run ID.

## Non-Claims

W17 does not claim:

- that the 2560-dimensional model is better than the incumbent;
- that semantic retrieval becomes the default;
- automatic promotion, automatic rollback, or arbitrary dimensions;
- external sealed quality or benchmark superiority;
- long-duration event-retention safety;
- cross-host HA, split-brain prevention, or cross-region operation;
- artifact signing or final release acceptance;
- a universal throughput, latency, RPO, or RTO SLO.

## Delivery Boundary

W17 is complete only after:

- the frozen W17 case validates;
- migration 16, RLS, role, reset, backup, and restore contracts pass;
- test-first worker/store/coordinator coverage proves both physical classes;
- the opt-in formal profile completes from a clean dedicated root;
- the real direct-provider 2560-dimensional probe succeeds;
- failures and non-claims are preserved in committed evidence;
- the full local release gates pass;
- a protected Draft PR head passes CI and its artifact chain is independently
  verified.

The overall Vermory goal remains active after W17 for genuine external sealed
evaluation, long-duration retention and pruning, cross-host HA evidence,
artifact signing, and final release acceptance.
