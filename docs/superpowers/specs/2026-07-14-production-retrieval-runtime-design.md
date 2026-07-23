# Production Retrieval Runtime Design

Date: 2026-07-14

Status: frozen for implementation

## Goal

Vermory must connect the W08-qualified active-only PostgreSQL/pgvector path to
the real workspace MCP and conversation Web Chat runtimes without making it the
default. Authoritative memory writes must remain independent of the embedding
provider, vector failures must preserve lexical behavior, and every non-lexical
request must leave enough durable evidence to explain whether vector retrieval
was used, shadowed, stale, or degraded.

W09 is a production-integration slice. It is not a second retrieval benchmark,
an RRF experiment, an embedding migration, a default switch, or scale
qualification.

## Frozen Assumptions

- PostgreSQL governed memory remains the only authority.
- `memory_search_documents` remains the lexical projection and product default.
- The first production semantic profile is `siliconflow-bge-m3-1024-v1`.
- The profile calls `https://api.siliconflow.cn/v1` directly with
  `BAAI/bge-m3` and validates 1024 response dimensions.
- Provider credentials are supplied only through a named environment variable.
- Mac mini NewAPI, Gemini CLI, mem0, MemOS, Supermemory, Redis, RRF, and an LLM
  reranker are not part of W09.
- One worker instance processes one tenant and one embedding profile. A
  privileged cross-tenant worker is deliberately deferred.
- The production vector column is fixed to 1024 dimensions in W09. A later
  generation-migration slice must qualify other dimensions and zero-downtime
  profile changes.

## Approaches Considered

### Synchronous Embedding In Authority Writes

Embedding inside `CommitGovernedObservation`, candidate acceptance, bridge
promotion, confirmation, or deletion would make authoritative writes depend on
an external provider. A timeout or rate limit could reject a valid user
correction or leave a database transaction open across a network call. This is
rejected.

### Manual Rebuild Only

A command that rebuilds vectors when an operator remembers to run it is simple,
but newly confirmed, corrected, bridged, superseded, or deleted memory would not
have a durable path to projection. This is acceptable for W08 measurement but
not for a runtime integration. This is rejected.

### Durable Authority Events With A Tenant Worker

Every governed-memory lifecycle change appends a small projection event in the
same PostgreSQL transaction. A separate tenant/profile worker reads the event,
rechecks current authority, performs the external embedding call only for a
currently active fact, updates or deletes the vector projection, and advances a
durable cursor only after success. This is the selected design.

## Data Model

Migration 14 adds four disposable or operational tables. None becomes memory
authority.

### `memory_projection_events`

An append-only authority-change stream:

```text
event_id          bigint identity primary key
tenant_id         text
continuity_id     uuid
memory_id         uuid
desired_state     active | absent
authority_version timestamptz
created_at        timestamptz
```

An `AFTER INSERT OR UPDATE` trigger on `governed_memories` appends an event when
the lifecycle, content, memory kind, tenant, or continuity changes. An active
`fact` produces `desired_state=active`; proposed, superseded, deleted, redacted,
or non-fact memory produces `desired_state=absent`.

The migration seeds one event for every existing governed memory so a new
profile can rebuild from the event stream. Events contain identifiers and state
only, never memory content, query text, provider credentials, or embeddings.

### `memory_projection_cursors`

One row per tenant and profile:

```text
tenant_id          text
profile_id         text
last_event_id      bigint
status             idle | running | failed
attempt_count      integer
last_error_code    text
last_attempt_at    timestamptz
updated_at         timestamptz
primary key (tenant_id, profile_id)
```

The cursor advances monotonically. A provider or database failure records a
bounded error code and leaves `last_event_id` unchanged. Reprocessing the same
event is therefore required to be idempotent.

### `memory_vector_documents`

The active-only semantic projection:

```text
profile_id      text
tenant_id       text
continuity_id   uuid
memory_id       uuid
content_sha256  text
embedding       vector(1024)
updated_at      timestamptz
primary key (profile_id, tenant_id, memory_id)
```

The table has tenant-aware foreign keys to governed memory and continuity,
RLS, a scope lookup index, and an HNSW cosine index. It contains no proposed,
superseded, deleted, redacted, or non-fact row. Search still joins current
`governed_memories` and rechecks `lifecycle_status='active'`, tenant,
continuity, memory kind, and content hash before delivery.

### `memory_retrieval_runs`

One durable audit row per runtime retrieval operation:

```text
id                    uuid
tenant_id             text
primary_continuity_id uuid
continuity_ids        uuid[]
operation_id          text
request_fingerprint   text
requested_mode        lexical | shadow | vector
effective_mode        lexical | shadow | vector
profile_id            text
query_sha256          text
lexical_memory_ids    uuid[]
vector_memory_ids     uuid[]
delivered_memory_ids  uuid[]
projection_current    boolean
degraded              boolean
failure_code          text
lexical_latency_ms    integer
vector_latency_ms     integer
created_at            timestamptz
unique (tenant_id, operation_id)
```

The primary continuity has a tenant-aware foreign key. The full linked set is
stored for evidence and is recomputed from current bridge authority before
search; PostgreSQL arrays are not treated as a substitute for relational
authorization. The row stores no raw query, context, memory content, provider
output, headers, database URL, or API key. Replaying the same operation ID
requires the same request fingerprint. A conflicting replay fails.

## Projection Event Contract

The database trigger, not individual Go call sites, is responsible for durable
event creation. This covers existing and future lifecycle paths uniformly:

- active source update;
- user correction and supersession;
- conversation confirmation;
- accepted source candidate;
- accepted document-formation candidate;
- bridge promotion;
- deletion and redaction;
- Global Defaults changes, which produce `absent` because Global Defaults are
  delivered through their dedicated thin layer rather than semantic search.

The trigger and event insert occur in the same authority transaction. If the
transaction rolls back, the event rolls back. Embedding is never called from a
trigger or authority transaction.

## Worker Contract

Add a fixed-tenant worker surface:

```text
vermory retrieval-worker
```

Required flags:

```text
--database-url
--tenant-id
--profile-id
--embedding-base-url
--embedding-api-key-env
--embedding-model
--embedding-dimensions
--once
--poll-interval
--batch-size
```

W09 accepts only profile `siliconflow-bge-m3-1024-v1`, model `BAAI/bge-m3`, and
dimensions `1024`. The explicit fields remain visible so a mismatched worker is
rejected instead of silently corrupting a projection.

For one tenant/profile, the worker:

1. acquires a PostgreSQL advisory lock so only one worker owns the cursor;
2. reads the next event after `last_event_id` under tenant RLS;
3. loads the current governed memory and continuity;
4. deletes the vector row if the current memory is not an active fact;
5. embeds current content and upserts the row if it is an active fact;
6. verifies tenant, continuity, lifecycle, profile, content hash, and dimensions;
7. advances the cursor only after the projection mutation commits;
8. records a bounded failure code without content if processing fails.

The worker does not trust the historical event's desired state over current
authority. A late active event for a memory that is now deleted therefore
deletes the vector row rather than restoring it.

`--once` drains at most `batch-size` events and exits. Without `--once`, the
worker polls until its context is cancelled. Shutdown does not abandon a
committed authority write; the unchanged cursor makes the event retryable.

## Runtime Retrieval Modes

The shared coordinator implements workspace and conversation retrieval.
Workspace uses one confirmed continuity. Conversation expands the existing
durable link root into the authorized set of linked conversation continuities
before lexical or vector search.

### `lexical`

- Calls the existing lexical runtime only.
- Does not require an embedding profile or provider credential.
- Records no new retrieval audit unless explicitly requested by a test or
  diagnostics command.
- Remains the CLI and product default.

### `shadow`

- Calls lexical and returns the lexical IDs and order unchanged.
- If the projection cursor is current, also calls vector retrieval and records
  both ranked ID lists and latency.
- If the projection is behind, provider fails, or vector SQL fails, returns the
  unchanged lexical result and records a bounded failure/degradation state.
- Never changes the context delivered to the AI client.

### `vector`

- Calls lexical first so an exact fallback is always available.
- Requires a current cursor for the requested tenant/profile.
- Calls vector retrieval with the requested authorized continuity set.
- Rechecks every candidate against PostgreSQL authority and content hash.
- Returns eligible vector results when the projection is current and the call
  succeeds.
- Returns the exact lexical IDs and order when the projection is stale,
  unavailable, empty because of an operational failure, or the provider call
  fails.
- Records `effective_mode=lexical`, `degraded=true`, and a bounded failure code
  on fallback.

W09 does not fuse lexical and vector lists. W08 measured no RRF quality gain,
so adding RRF to the runtime would be unsupported complexity.

## Runtime Surfaces

Add shared retrieval flags to these commands:

```text
vermory mcp-stdio
vermory web-chat
vermory serve
```

Flags:

```text
--retrieval-mode lexical|shadow|vector
--retrieval-profile siliconflow-bge-m3-1024-v1
--embedding-base-url
--embedding-api-key-env
--embedding-model
--embedding-dimensions
```

When mode is `lexical`, embedding flags are ignored and no credential is read.
When mode is `shadow` or `vector`, all profile fields must match the frozen
profile and the named environment variable must be non-empty. Errors must never
include the API key or database URL.

The authenticated server shares one coordinator but passes the authenticated
tenant into every search. It does not start a privileged cross-tenant worker.
Operators run one fixed-tenant worker for each tenant/profile they enable. If a
tenant has no current projection, authenticated requests safely fall back to
lexical.

MCP and Web Chat output schemas do not expose internal mode names, candidate
IDs, profile IDs, or failure codes. Those fields remain in admin diagnostics
and audit records only.

## Recovery And Administration

Add:

```text
vermory retrieval-status
vermory retrieval-rebuild
```

`retrieval-status` reports profile, tenant, cursor event, latest tenant event,
lag, vector row count, status, attempt count, last bounded error code, and last
attempt time. It never reports content or credentials.

`retrieval-rebuild` requires an admin database URL, a tenant ID, and the frozen
profile. It deletes that tenant/profile's vector rows, resets the cursor to
zero, and leaves authority plus lexical projection unchanged. The worker then
replays the event stream. Rebuild acceptance requires result-ID equivalence on
the frozen W09 cases and an unchanged authority fingerprint.

PostgreSQL dump/restore includes events, cursors, retrieval audits, and vector
documents. Vector documents remain disposable: restore acceptance deletes and
rebuilds them from authority events before claiming semantic retrieval is
ready.

## RLS And Role Boundary

All four W09 tables enable RLS with the existing `vermory.tenant_id` policy.
Tenant-aware foreign keys reject cross-tenant memory, primary continuity,
vector, event, and audit references. Cursor rows are tenant-scoped by RLS and
their composite tenant/profile primary key. Linked audit continuity IDs are
recomputed and validated against bridge authority before retrieval.

`database grant-runtime` and `ValidateRuntimeRole` include the W09 tables. The
restricted runtime role must not own them, bypass RLS, access auth token
digests, or gain access to legacy Phase 1 tables. Filter-omission probes against
the W09 tables must return zero rows without a tenant context, only the selected
tenant's rows with a context, and zero rows after switching to another tenant.

The worker uses the same restricted runtime role for a fixed tenant. Migration,
profile rebuild reset, and extension/index creation remain admin operations.

## Idempotency And Concurrency

- Projection events are append-only and transactional.
- Reprocessing an event produces the same vector row or absence.
- One advisory lock serializes a tenant/profile cursor.
- A second worker for the same tenant/profile exits with `already_running`
  rather than processing concurrently.
- Retrieval audit operation IDs are tenant-scoped and fingerprinted.
- A deletion or supersession committed while embedding is in flight wins: the
  worker rechecks authority immediately before upsert and must delete or skip if
  the memory is no longer active.
- A rebuild can run only when the tenant/profile worker lock is free.
- Search never treats cursor-current as sufficient; returned rows are always
  rechecked against current authority.

## Frozen W09 Acceptance Cases

Create `runtime/cases/W09-production-retrieval-runtime` with complete
trajectories rather than isolated prompts.

### Workspace Semantic Recall

A software release workspace contains an active Chinese fact stating that the
release rollback requires two maintainers, plus similar distractors in another
workspace and tenant. Lexical search misses a paraphrased Chinese task while
vector mode recalls the correct fact. Grok consumes it through the real MCP
`prepare_context` tool, creates a deterministic release artifact, and writes one
proposed observation back through `commit_observation`.

### Exact Technical Recall

The same workspace contains a path, flag, error code, and model identifier.
Vector mode must return all requested exact facts without adding another
continuity's near-duplicate values.

### Conversation Linked Recall

Two explicitly linked conversation threads share one ongoing deployment
decision. Web Chat in vector mode recalls the accepted current fact across the
link, but not an unlinked thread or another tenant. The same case runs in
`shadow` and proves the delivered context is byte-identical to lexical.

### Lifecycle Transition

An active fact is indexed, corrected, superseded, and then the replacement is
deleted. Event replay must remove both stale rows at the correct steps. Exact,
semantic, and related queries must return neither deleted nor superseded
content.

### Provider Outage

The embedding endpoint returns HTTP 503. Authority writes and lexical search
remain available. Shadow and vector requests return exact lexical IDs/order and
record bounded degradation without credentials or raw content.

### Projection Lag And Rebuild

An active correction is committed while the worker is stopped. Vector mode
detects cursor lag and falls back instead of using stale rows. After worker
catch-up, vector mode uses the new fact. Deleting all vector rows and resetting
the cursor, then replaying events, preserves result IDs and the authority
fingerprint.

### Restricted Role Isolation

The fixed-tenant worker, MCP runtime, Web Chat runtime, retrieval audit, cursor,
events, and vector tables run under a non-owner restricted role with RLS.
Cross-tenant foreign keys and filter-omission probes must fail closed.

## Hard Gates

W09 fails if any of these occurs:

- an embedding/provider failure rejects or rolls back governed memory;
- proposed, superseded, deleted, redacted, or Global Defaults rows enter the
  vector projection;
- cross-tenant or unauthorized cross-continuity content is returned;
- vector output bypasses a PostgreSQL authority/content-hash recheck;
- vector mode uses a projection whose tenant/profile cursor is behind;
- shadow changes the lexical delivered IDs or order;
- vector fallback differs from lexical IDs or order;
- a late embedding completion restores stale or deleted content;
- projection rebuild changes authoritative state or frozen-case result IDs;
- a restricted role sees another tenant's projection or audit rows;
- an API key, raw authorization header, database URL, private path, or raw
  private query is written to audit, artifacts, logs, arguments, or Git.

## Verification

The implementation must pass:

- migration 14 up/down replay and schema-version checks;
- trigger/event coverage for every existing lifecycle path;
- cursor, duplicate event, advisory lock, late completion, and retry tests;
- workspace and linked-conversation retrieval tests against real PostgreSQL and
  pgvector;
- shadow byte-equivalence and vector exact-fallback tests;
- projection lag, provider HTTP 503, delete, supersede, and rebuild tests;
- RLS, runtime-role grant/validation, foreign-key, and filter-omission tests;
- MCP, Web Chat, authenticated server, CLI flag, secret-redaction, and replay
  tests;
- full serial PostgreSQL suite with `-p 1`;
- runtime and new-package race tests;
- reality race, vet, tidy, module diff, Actionlint, GoReleaser, four-platform
  release snapshot, OpenClaw check/package, migration replay, and native
  dump/restore;
- one real direct SiliconFlow embedding run;
- one real logged-in Grok MCP workspace task and one real conversation turn;
- protected-CI artifact download, digest, checksum, layout, package, and
  darwin/arm64 execution verification.

## Non-Claims

W09 does not prove:

- that vector retrieval should become the default;
- that vector is superior on a second independent or sealed corpus;
- any RRF, reranker, or learned fusion benefit;
- zero-downtime embedding profile or dimension migration;
- a privileged all-tenant worker or hosted control plane;
- million-record, high-concurrency, HA, PITR, or multi-region operation;
- source-authority ranking across conflicting active sources;
- broad automatic memory-formation quality;
- final signing, publication, or release acceptance.

W09 succeeds when real clients can explicitly use or safely shadow one governed
semantic retrieval profile, lifecycle projection is durable and rebuildable,
and every failure returns to the unchanged lexical path without weakening
authority, isolation, deletion, or auditability.
