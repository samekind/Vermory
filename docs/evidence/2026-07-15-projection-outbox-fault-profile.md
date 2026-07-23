# Projection Outbox Fault Profile Evidence

Date: 2026-07-15

Status: H-012 supported for the current self-hosted profile

## Scope

W11 tests whether PostgreSQL can remain both Vermory's authority and its
transactional projection outbox without a required Redis service. The frozen
case covers the failure modes that matter before a semantic projection can be
treated as operational rather than best-effort:

- a real backlog larger than one worker batch;
- provider failure and retry without cursor loss;
- at-least-once replay of already processed events;
- PostgreSQL immediate restart while embedding work is in flight;
- recovery through the same runtime pool after restart;
- deletion racing a late embedding completion;
- direct provider projection and retrieval after recovery.

The test starts a disposable PostgreSQL 18 cluster under `/tmp`, migrates it to
schema 15, and removes it after execution. It does not stop or mutate the
developer's shared PostgreSQL service.

## Execution

| Field | Value |
|---|---|
| Run | `w11-projection-outbox-fault-profile-20260715-v1` |
| Implementation | `9dcfa8599b456d5ae1f419553061bb14fdca1580` |
| Case SHA-256 | `c2922e1ce832d74435b8aefffa0bd32bfc740108d6f2dbf50c768549e2400366` |
| Raw log SHA-256 | `ffdb9fd0701467abe36fb71ab3fa24455d963bc2789629176c2fbb78688a44a8` |
| OS | `Darwin 27.0.0 arm64` |
| CPU / memory | Apple M4 Pro / 48 GiB |
| Go | `go1.26.5 darwin/arm64` |
| PostgreSQL / pgvector | `18.4` / `0.8.5` |
| Retrieval profile | `siliconflow-bge-m3-1024-v1` |
| Real provider route | direct SiliconFlow `BAAI/bge-m3` |
| Test duration | `5.92 s` |

Normalized evidence:
[W11 JSON](snapshots/2026-07-15-projection-outbox-fault-profile.json).

Raw execution:
[W11 log](snapshots/2026-07-15-projection-outbox-fault-profile.log).

## Results

| Gate | Result |
|---|---:|
| Governed active facts seeded | `1,000` |
| Initial projection backlog | `1,000` |
| First bounded worker pass | `128` processed / `872` lag |
| Provider failure cursor advance | `0` |
| Vector rows after initial catch-up | `1,000` |
| Vector rows after cursor rewind and full replay | `1,000` |
| Partial vector committed during PostgreSQL stop | `0` |
| Same runtime pool recovered after restart | PASS |
| Pending restart fact projected after retry | PASS |
| Deleted in-flight fact vector after retry | `0` |
| Final backlog | `0` |
| Real provider requests after restart | `2` |
| Real provider projection and vector retrieval | PASS |

The duplicate-delivery gate deliberately rewound the profile cursor to event
zero while leaving all vector rows present. Reprocessing the entire event
stream used idempotent upserts and preserved exactly 1,000 vector rows rather
than creating duplicates.

For restart behavior, the worker loaded an active authority row and blocked in
the embedder. PostgreSQL was then stopped with `immediate`, the embedding was
released, and the worker failed without committing a vector. After the same
cluster restarted, the original pgx pool recovered and the pending event was
processed normally.

For deletion behavior, a second worker blocked after loading an active fact.
The authoritative fact was deleted before embedding completion. The worker
returned `authority_changed`; replay consumed the original and deletion events
against current authority and left zero vectors for the deleted memory.

The final direct-provider check used a separate tenant so deterministic fault
vectors could not mix with real embedding space. One real SiliconFlow request
projected the fact and one embedded the query; production vector retrieval
returned the governed memory after the PostgreSQL restart.

## Preserved Failure

The first attempt failed before PostgreSQL startup because macOS Unix-domain
socket paths have a small length limit and `t.TempDir()` produced a long
`/var/folders/.../socket` path. The same cluster options started successfully
under a short `/tmp` path. The harness now creates its disposable root under
`/tmp/vermory-w11-pg-*` and registers cleanup. This was a test-infrastructure
failure, not an outbox or recovery failure.

## Decision

H-012 is supported for the current developer-local and self-hosted profile.
Authoritative transactions and projection events remain in PostgreSQL; workers
are idempotent, cursor-based, retryable, restart-safe, and deletion-safe under
the measured case. Redis remains optional rather than a default dependency.

The decision must be reopened if sustained server-scale backlog, multiple
competing workers, cross-region delivery, or queue retention requirements show
that PostgreSQL contention or operations are no longer acceptable.

## Non-Claims

- This is not a 100k active-memory or 1M projection qualification.
- This is not an HA, failover, replication, or PITR result.
- This does not define a sustained events-per-second SLO.
- This does not test a dimensionality migration while backlog is active.
- This does not complete sealed evaluation, signing, or final release gates.
