# Projection Event Retention And Pruning Design

Date: 2026-07-16

Status: frozen for W18 implementation

## Objective

Qualify bounded retention of PostgreSQL projection events without weakening
Vermory's authoritative memory, deletion, continuity, audit, or profile
isolation contracts.

W18 addresses one concrete long-running deployment failure:

> `memory_projection_events` is append-only and currently grows without bound,
> while cursors and reset behavior assume the complete event history still
> exists.

Deleting old rows without changing that cursor contract would create a false
current state. A new or reset profile could start from cursor zero, see no
remaining historical events, report zero lag, and still have an incomplete or
empty projection.

The required result is an explicit, auditable retention floor. Profiles below
that floor must rebuild from current PostgreSQL authority before incremental
processing or vector retrieval can resume.

## Fixed Boundaries

- PostgreSQL governed memories remain the only authority.
- `memory_projection_events` is a disposable delivery log, not memory truth.
- W18 does not automatically delete governed memories, observations,
  supersession history, conversation turns, bridge records, retrieval audits,
  source-governance audits, or user-visible evidence.
- Authorized forgetting continues to erase or redact sensitive authoritative
  content and remove it from projections. Event pruning is not a substitute
  for forgetting.
- Lexical remains the product default.
- Semantic profiles remain explicit opt-in projections.
- No Redis or second authoritative queue is introduced.
- Pruning is an explicit operator action in W18. There is no hidden scheduler.
- Every prune operation is tenant-scoped, idempotent, and auditable.
- A pruned profile never silently skips missing history.
- Failed attempts and injected failures remain in evidence.

## Approaches Considered

### A. Age-only deletion

Delete events older than a configured duration.

This is simple but unsafe. A slow profile may still require an old event, and
cursor lag cannot prove recoverability after the event is removed.

### B. Cursor floor plus explicit rebuild-required state

Track the highest event ID intentionally pruned for each tenant. Prune only
through the minimum safe cursor and the configured age/tail limits. Any cursor
below the retained floor becomes `rebuild_required` and cannot run incrementally.

This is selected. It makes the recovery boundary visible and works with the
existing current-authority snapshot rebuild.

### C. Time partitions and partition drop

Partition events by time and drop old partitions after cursor checks. This can
improve very large operational profiles, but it does not remove the need for a
retention floor or rebuild-required semantics. It adds deployment and migration
complexity before the behavioral contract is qualified.

Partitioning remains a future physical optimization.

## Schema 17 Contract

Migration 17 adds two tenant-isolated tables and extends cursor state.

### `memory_projection_retention`

```text
tenant_id                    text primary key
pruned_through_event_id      bigint not null default 0
last_pruned_at               timestamptz
updated_at                   timestamptz not null
```

`pruned_through_event_id` is monotonic. It records the highest event ID removed
by an acknowledged prune operation for that tenant. Gaps caused by other
tenants or authority cascades do not move the explicit floor.

### `memory_projection_prune_runs`

```text
id                           uuid primary key
tenant_id                    text not null
operation_id                 text not null
request_fingerprint          text not null
cutoff                       timestamptz not null
retain_tail_events           integer not null
safe_cursor_event_id         bigint not null
previous_floor_event_id      bigint not null
new_floor_event_id           bigint not null
deleted_events               bigint not null
result                       text not null
created_at                   timestamptz not null
unique (tenant_id, operation_id)
```

Allowed results are `pruned` and `noop`. The run contains no memory content,
embedding, credential, raw DSN, or provider response.

Both tables use `vermory.tenant_id` RLS. The ordinary runtime role receives
read-only access to `memory_projection_retention` so workers and retrieval can
enforce the floor. It receives no delete or write privilege on the retention
or prune-run tables. Pruning requires the operator/admin database boundary.

### Cursor state

`memory_projection_cursors.status` gains:

```text
rebuild_required
```

The meaning is exact: the profile's current physical projection cannot be made
complete by consuming the retained event tail alone.

## Cursor And Retrieval Contract

### Incremental worker start

Before setting a cursor to `running`, the worker reads the tenant retention
floor.

- If no floor exists, the effective floor is zero.
- If `last_event_id >= pruned_through_event_id` and the cursor is not
  `rebuild_required`, incremental processing may continue.
- If `last_event_id < pruned_through_event_id`, the worker atomically marks the
  cursor `rebuild_required`, records `projection_rebuild_required`, and exits
  without embedding or advancing the cursor.
- A new profile cursor created after pruning starts as `rebuild_required` at
  the retained floor rather than pretending it consumed deleted history.

### Retrieval

Semantic retrieval is current only when:

```text
status = idle
last_event_id >= pruned_through_event_id
lag = 0
```

`running`, `failed`, or `rebuild_required` profiles degrade to lexical. The
audit failure code for the new state is `projection_rebuild_required`.

### Reset

Reset clears only the selected physical projection and changes its cursor to:

```text
last_event_id = current retention floor
status = rebuild_required
last_error_code = projection_rebuild_required
```

It does not replay a potentially incomplete event history and does not mutate
authority, lexical state, another profile, or another tenant.

### Rebuild

`RebuildCurrent` remains the recovery operation:

1. capture the tenant's current event watermark;
2. clear the selected physical projection;
3. snapshot current eligible authority;
4. reject rows changed during embedding;
5. set the cursor to at least the captured watermark;
6. drain newer retained events normally.

Successful rebuild clears `projection_rebuild_required`.

## Prune Algorithm

The operator supplies:

```text
tenant ID
operation ID
cutoff timestamp
retain-tail event count
```

The request is normalized and fingerprinted. Reusing the same tenant and
operation ID with the same fingerprint returns the original receipt. Conflicting
reuse is rejected.

One transaction performs the prune:

1. acquire a tenant-scoped transaction advisory lock;
2. lock or create the tenant retention row;
3. calculate `safe_cursor_event_id` as the minimum `last_event_id` among cursor
   rows whose status is not `rebuild_required`;
4. if no incremental subscriber exists, use the latest tenant event as the
   cursor-safe bound because every later subscriber must snapshot;
5. calculate the highest tenant event older than `cutoff`;
6. calculate the highest event that still retains the newest
   `retain_tail_events` rows for that tenant;
7. choose the minimum of the cursor, cutoff, and tail bounds;
8. delete only that tenant's events above the previous floor and through the
   chosen bound;
9. update the floor monotonically to the maximum deleted event ID;
10. insert the prune receipt in the same transaction.

If PostgreSQL stops before commit, event deletion, floor movement, and audit
insertion all roll back together. A retry may reuse the same operation ID only
after the failed transaction left no committed receipt.

## Concurrency Rules

- Event inserts committed after the selected bound are never pruned by that
  operation.
- A worker cursor may advance while a prune waits or runs; that can only make a
  later prune less restrictive.
- A failed cursor remains retention-blocking because it is expected to retry.
- An intentionally reset or abandoned projection must be moved to
  `rebuild_required`; it then stops blocking event retention.
- A newly created cursor below the committed floor cannot race into incremental
  processing because worker start rechecks the floor.
- Pruning one tenant cannot inspect, count, delete, or audit another tenant's
  events under the restricted tenant role.

## Operator Interface

Add:

```text
vermory retrieval-prune-events
  --database-url <admin URL>
  --tenant-id <tenant>
  --operation-id <stable id>
  --before <RFC3339 timestamp>
  --retain-tail-events <non-negative integer>
```

The command returns only the normalized prune receipt. It never prints the
database URL or credentials.

Existing status output gains:

```text
pruned_through_event_id
rebuild_required
```

## Frozen W18 Qualification Case

Case ID:

```text
W18-projection-event-retention-pruning
```

The profile is accelerated long-duration evidence, not a claim of months of
wall-clock uptime.

```text
tenants                         4
continuities per tenant         5
initial current facts           20,000
accelerated write epochs        24
revisions                       72,000
deletions                       4,800
new facts                       4,800
tail events                     153,600
total generated events          173,600
final current facts             20,000
retained tail per tenant        1,000
incumbent query clients         16
queries per client              20
total incumbent queries         320
```

The harness uses deterministic embedders for scale mechanics and one separate
direct SiliconFlow `BAAI/bge-m3` projection/query probe after pruning.

## Formal Trajectory

### Phase 1: Establish subscribers

- seed 20,000 current governed facts;
- build the active `vector_1024` projection;
- build the `halfvec_2560` candidate projection;
- leave the same-dimension candidate without a cursor so it acts as a future
  subscriber;
- verify all established projections match current authority.

### Phase 2: Accelerated epochs and blocked pruning

- apply 24 deterministic revision/delete/new-fact epochs;
- keep the active profile current;
- intentionally stop the dimensional candidate at a frozen cursor;
- run pruning after each old-enough epoch;
- prove no prune passes the slow candidate cursor;
- serve all 320 active-profile queries without scope leakage while writes and
  prune attempts continue.

### Phase 3: Catch-up and bounded retention

- catch the slow candidate to current;
- prune again with the frozen tail reserve;
- require the event table to fall to the calibrated retained bound;
- prove authority, lexical rows, active vectors, candidate vectors, and audit
  records remain unchanged except for expected current updates.

### Phase 4: Restart during prune

- pause a real prune transaction after delete/floor/audit mutations and before
  commit;
- stop only the dedicated PostgreSQL cluster with `immediate`;
- restart and reuse the same pool;
- prove the interrupted operation committed none of its three mutations;
- retry under a new attempt record and complete atomically.

### Phase 5: Rebuild-required behavior

- start the previously unseen same-dimension candidate after pruning and
  require `projection_rebuild_required` with zero embedding calls;
- rebuild it from current authority and reach zero lag;
- reset the dimensional candidate after pruning and require the same state;
- rebuild it and prove exact ID/hash equivalence;
- delete a fact after pruning and prove late or replayed work cannot restore it.

### Phase 6: Direct provider probe

- project one post-prune governed fact through the direct provider;
- issue one paraphrased vector query through the production coordinator;
- require the expected fact, correct profile-scoped audit, and two provider
  requests;
- record only non-secret hashes and timing.

## Hard Gates

1. Schema 17 creates tenant-isolated retention and prune-audit state.
2. Runtime workers can read but cannot mutate retention control tables.
3. No prune passes the slowest incremental cursor.
4. Authority writes and active-profile queries continue during prune attempts.
5. Interrupted prune deletion, floor, and audit changes roll back together.
6. The same pool recovers after PostgreSQL restart.
7. The event table reaches the calibrated retained bound after catch-up.
8. Retention floor is monotonic and idempotent replay is byte-stable.
9. A new subscriber below the floor requires rebuild with zero embedding work.
10. Reset after pruning requires rebuild and leaves other profiles unchanged.
11. Rebuilds match current authority and deleted memories remain absent.
12. Cross-tenant pruning and receipt access are blocked.
13. The direct provider projection/query succeeds after pruning.
14. Lexical and the incumbent profile remain default; no profile is promoted.

Any failed gate fails the formal profile. Average success cannot hide a cursor,
deletion, isolation, or false-current failure.

## Report And Replay

The normalized report records:

- case and implementation hashes;
- schema and retention policy values;
- generated, pruned, and retained event counts per epoch and tenant;
- cursor/floor positions for every profile;
- prune receipts and idempotency results;
- restart rollback and same-pool recovery evidence;
- query success, scope leakage, degradation, and latency;
- rebuild-required and rebuild equivalence evidence;
- provider request count, dimensions, timing, and response hashes;
- chronological failures and explicit non-claims;
- all fourteen hard-gate booleans.

A completed run ID replays the existing normalized report without starting
PostgreSQL or calling the provider. Conflicting reuse is rejected. Reports are
scanned for credential-shaped values before writing.

## Non-Claims

W18 does not claim:

- months of uninterrupted wall-clock operation;
- automatic retention scheduling;
- automatic deletion of authoritative memory or history;
- PostgreSQL table partitioning or cross-region queueing;
- embedding-model ranking or profile promotion;
- cross-host HA;
- external sealed evaluation;
- artifact signing or final release acceptance.

## Acceptance

W18 is complete only when:

- schema, state-machine, pruning, reset, retrieval, RLS, role, CLI, and
  idempotency tests pass;
- the accelerated formal profile completes from a clean dedicated root;
- the direct provider probe succeeds;
- same-run replay is byte-identical and offline;
- failures and non-claims are committed with normalized evidence;
- full local release gates pass;
- protected CI passes on the final checklist head;
- the final artifact and synthetic merge are independently verified;
- Draft PR 1 receives exactly one W18 delivery section;
- the overall Vermory goal remains active for remaining platform boundaries.
