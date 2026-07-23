# Server Qualification Scale Design

Date: 2026-07-15

Status: frozen for implementation

## Goal

W12 must determine whether the PostgreSQL-authoritative Vermory runtime can
operate a named server profile with 100,000 active governed facts and 1,000,000
durable projection events while preserving tenant isolation, deletion, cursor,
fallback, and rebuild semantics. It must also replace historical event replay
as the bootstrap mechanism for a large current projection.

The result is an operational qualification on fixed hardware. Synthetic data
measures database, projection, concurrency, and recovery behavior only. W08,
W10, LongMemEval, and real-client cases remain the quality evidence.

## Reality Findings

The existing 10k profile is useful but cannot simply be multiplied:

- `memory_projection_events.event_id` is global, while cursors are tenant
  scoped. `latest_event_id - last_event_id` therefore counts other tenants'
  identity gaps as backlog.
- replaying one million historical events to build 100,000 current vectors can
  call the embedder repeatedly for obsolete versions of the same fact;
- the product needs a bootstrap path from current PostgreSQL authority, then
  normal event replay only after a captured watermark;
- semantic vectors remain optional and lexical remains the default, but an
  enabled profile must still have a bounded path to become current.

## Frozen Profile

`server-qualification-v1` uses:

- Apple M4 Pro, 48 GiB memory, local NVMe;
- Darwin arm64;
- PostgreSQL 18.4 and pgvector 0.8.5;
- 10 tenants;
- 10 continuities per tenant;
- 1,000 active facts per continuity;
- 100,000 current active facts;
- 450,000 append-only governed revisions;
- 450,000 superseded facts and 550,000 total governed facts;
- 1,000,000 durable projection events;
- 100,000 current lexical documents;
- 100,000 current vectors for the active v1 profile;
- 50 concurrent query clients and 1,000 total queries;
- 1,000 concurrent deletions;
- two competing tail workers per tenant;
- an explicit pgx pool maximum of 64 connections.

The calibrated limits are in the frozen case manifest. They are deliberately
generous relative to the measured 10k self-hosted profile. Changing a limit
requires a new case version and preserves the failed run.

## Tenant Lag Contract

`ProjectionStatus.Lag` is the number of pending event rows for that tenant with
`event_id > last_event_id`. It is not arithmetic distance between two global
identity values. `LatestEventID` remains the greatest tenant event ID, or the
cursor when retained history contains no later row.

This contract is required before any multi-tenant backlog metric is accepted.

## Snapshot Bootstrap Contract

Add a current-authority bootstrap operation to `ProjectionWorker` and expose it
through an operator CLI command.

For one tenant and profile it must:

1. acquire the existing tenant/profile advisory lock;
2. capture the tenant's current event high watermark;
3. clear only that tenant/profile's disposable vectors;
4. mark the cursor `running` without advancing it;
5. page through current active, non-redacted facts in stable UUID order;
6. embed each current content value once;
7. recheck tenant, continuity, lifecycle, content, and `updated_at` before each
   upsert;
8. skip a row changed while embedding rather than committing a stale vector;
9. advance the cursor to the captured watermark only after the full snapshot
   succeeds;
10. leave events after the watermark pending for the ordinary worker.

Provider or database failure leaves the cursor behind and the projection
non-current. A retry clears partial vectors and starts a new snapshot. Retrieval
continues to degrade to exact lexical behavior while lag is non-zero.

The bootstrap is not an authority transaction and does not hold a database
transaction across provider calls. It may hold one session advisory lock and
one pool connection for the operation.

## Scale Harness

The opt-in W12 harness uses a disposable PostgreSQL 18 cluster under `/tmp`.
It never resets the developer's shared database.

Initial authority seeding uses the runtime observation/governance transaction
path in bounded transactions. Synthetic history then creates new source-update
observations and new active governed revisions with `supersedes_memory_id`, and
marks the previous revision superseded in the same tenant-scoped transaction.
Four complete revision rounds plus one 50,000-record partial round create
450,000 revisions. Each revision produces one active and one absent projection
event, so the 100,000 initial events plus 900,000 revision events total exactly
1,000,000. After the final revision, the disposable lexical projection is
rebuilt from current authority.

Scale vectors use a deterministic 1024-dimension embedder keyed by stable
record markers. This avoids 100,000 billable provider calls while exercising
the real pgvector schema, HNSW index, worker locking, current-authority recheck,
and retrieval coordinator. A separate small tenant performs a two-request
direct SiliconFlow projection/query probe after the scale gates.

## Hard Gates

- authority, active, superseded, lexical, event, and vector counts match the
  manifest;
- every tenant's lag is exact despite interleaved global event IDs;
- snapshot embedding requests do not exceed current active fact count;
- no proposed, superseded, deleted, redacted, cross-tenant, or cross-continuity
  fact enters a result;
- concurrent deletion removes both lexical and vector eligibility;
- competing workers preserve one vector per active memory and monotonic cursors;
- all tenants reach zero lag after tail processing;
- 1,000 concurrent queries complete without errors or empty expected hits;
- p95, p99, phase durations, database size, and resource context are reported;
- the direct SiliconFlow post-scale probe succeeds without writing a key.

## Non-Claims

W12 does not claim:

- one million vector rows;
- HA, failover, replication, PITR, or cross-region delivery;
- unlimited event retention;
- external queue rejection for every future deployment profile;
- memory formation quality, benchmark superiority, or sealed generalization;
- a semantic-default switch;
- signing, publication, or final release acceptance.
