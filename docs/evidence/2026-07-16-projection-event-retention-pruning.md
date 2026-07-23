# Projection Event Retention And Pruning Qualification Evidence

Date: 2026-07-16

Revision under test: `a04e978e76dcd6b59867238a2cc4644fe9783d0d`

Formal run ID: `projection-retention-20260716T014352Z`

## Scope

This evidence covers the frozen
`W18-projection-event-retention-pruning` case. It qualifies bounded retention
for `memory_projection_events` without treating that delivery log as memory
authority.

The formal run exercised:

- four tenants, twenty workspace continuities, and 20,000 initial current
  governed facts;
- one active 1,024-dimensional projection and one physically separate
  2,560-dimensional candidate projection;
- twenty-four accelerated epochs containing 72,000 revisions, 4,800 deletes,
  and 4,800 new facts;
- exactly 153,600 tail events and 173,600 total generated events;
- ninety-six slow-cursor prune attempts, four final bounded prunes, one
  interrupted transaction, and one successful retry receipt;
- sixteen concurrent query clients and 320 scoped requests during authority
  writes and prune attempts;
- one future subscriber, one profile-scoped reset/rebuild, one restricted-role
  tenant-isolation proof, one PostgreSQL immediate-stop rollback, and one
  direct SiliconFlow projection/query probe.

PostgreSQL remained the only authority. Pruning did not delete governed
memories, source history, observations, conversation turns, bridge records,
retrieval audits, or governance audits. Lexical remained the product default,
and no semantic profile was promoted.

## Environment

```text
Host: darwin/arm64, Apple M4 Pro, 48 GiB
PostgreSQL: 18.4
pgvector: 0.8.5
Schema: 17
Authority: PostgreSQL
Provider route: direct SiliconFlow OpenAI-compatible embeddings
```

The run used one disposable PostgreSQL cluster bound only to loopback. It did
not inspect, stop, copy, or modify an existing PostgreSQL service or user data
directory.

## Formal Result

| Measurement | Result |
|---|---:|
| initial current facts | `20,000` |
| accelerated epochs | `24` |
| revisions / deletions / new facts | `72,000 / 4,800 / 4,800` |
| generated events | `173,600` |
| pruned events | `169,600` |
| retained events | `4,000` |
| final current facts | `20,000` |
| final lexical rows | `20,000` |
| final incumbent vectors | `20,000` |
| final dimensional vectors | `20,000` |
| epoch evidence rows | `96` |
| prune receipts | `101` |
| recorded profile states | `9` |
| hard gates | `14 / 14` |
| formal duration | `30m37.759s` |

The twenty-four epochs compress a high-churn workload. They are not evidence
of twenty-four months or any other wall-clock retention duration.

## Retention Bound

The candidate projection cursor was intentionally frozen while authority
writes and the incumbent worker continued. Every epoch prune used the slowest
non-`rebuild_required` cursor as an upper bound. No prune advanced beyond that
cursor.

After the candidate caught up, the four final prunes retained exactly 1,000
events per tenant:

| Tenant | Final cursor | Retention floor | Retained events |
|---|---:|---:|---:|
| `w18-profile-tenant-00` | `168,800` | `167,800` | `1,000` |
| `w18-profile-tenant-01` | `170,400` | `169,400` | `1,000` |
| `w18-profile-tenant-02` | `172,000` | `171,000` | `1,000` |
| `w18-profile-tenant-03` | `173,600` | `172,600` | `1,000` |

The 101 durable receipts consist of ninety-six epoch attempts, four final
prunes, and the retry after the interrupted restart transaction. Replaying a
completed final operation returned the same receipt bytes and did not create a
duplicate receipt. Retention floors remained monotonic.

## Query Availability During Pruning

All sixteen clients were released through a barrier immediately before the
first prune window. Their measured request interval overlapped the prune
interval, and all 320 requests completed with zero cross-scope results.

| Measurement | Result |
|---|---:|
| successful scoped requests | `320 / 320` |
| cross-scope results | `0` |
| lexical degradations | `320` |
| p50 / p95 / p99 | `2 / 23 / 23 ms` |

Every explicit vector request degraded to lexical because the active
projection was temporarily non-current while writes were being drained. This
is availability, scope, and safe-degradation evidence. It is not evidence of
vector-serving latency or embedding quality during backlog.

## Rebuild And Deletion Safety

A future profile first appeared after history had been pruned. Its incremental
worker was rejected with `projection_rebuild_required` before making any
embedding request. `RebuildCurrent` then projected the current authority
snapshot and advanced to the retained watermark.

For a separate tenant, resetting only the 2,560-dimensional candidate removed
that profile's rows, set its cursor to `rebuild_required`, and left incumbent
rows unchanged. Snapshot rebuild restored exact current-authority ID
equivalence.

The fact selected for deletion in epoch 24 was absent from lexical retrieval,
the incumbent projection, the dimensional projection, and the future profile
after all rebuilds. Final authority, lexical, incumbent, and dimensional counts
were each exactly 20,000.

## Restart And Transaction Rollback

The harness opened a prune transaction, deleted ten retained events, advanced
the floor, and inserted a prune receipt. Before commit, it stopped only the
dedicated PostgreSQL cluster with `immediate`.

| Gate | Result |
|---|---:|
| event deletion rolled back | `true` |
| retention-floor advance rolled back | `true` |
| prune receipt insertion rolled back | `true` |
| existing pgx pool recovered | `true` |
| measured recovery | `14 ms` |
| same operation ID retry succeeded | `true` |

The interruption is preserved as an expected failure record rather than
removed from the report.

## Runtime Role And Tenant Isolation

The restricted runtime role could read its own retention floor but failed the
operator-role validator. It had no permission to mutate retention state or
read prune receipts. Under tenant context, the role observed its own floor,
observed zero rows for another tenant, could not read the receipt table, and
could not prune another tenant.

This formal proof complements migration tests that require RLS on both W18
tables, revoke PUBLIC privileges, and reject unsafe owner, superuser, or
`BYPASSRLS` runtime identities.

## Direct Provider Probe

After pruning and rebuild verification, the harness used the active production
profile directly, without NewAPI or another proxy:

| Measurement | Result |
|---|---:|
| endpoint | `https://api.siliconflow.cn/v1` |
| model | `BAAI/bge-m3` |
| dimensions | `1,024` |
| requests | `2` |
| total duration | `418 ms` |
| projection response SHA-256 | `07ad9028c05d4496631dc034731bae274c2626edd510d57c82c71fe8bcf03321` |
| query response SHA-256 | `71c76838a2e8a74bb7ee95f615d68a8914a02cbce0742d655e2438f068073921` |

The first request projected one governed fact through the production worker.
The second embedded a paraphrased query and retrieved that fact through the
production coordinator. No credential, raw vector, or provider response body
is present in the report or committed evidence.

## Deterministic Replay

The completed report was replayed with `SILICONFLOW_API_KEY` unset and both the
PostgreSQL root and binary directory set to invalid paths. The replay returned
before any database startup or provider check and validated the report against
the clean implementation revision.

An initial replay invocation supplied an incorrectly transcribed expected
revision and was rejected without changing either artifact. A second replay
used `git rev-parse HEAD` and passed.

| Artifact | SHA-256 |
|---|---|
| frozen case | `57bde8b13a7783cb981bcaa623d2f6f316928fd21ff6619da4983ff3b31fc2d4` |
| report JSON | `5dbb93dbfe144e162774f85570786bff45462c83887d4c9fcd24475c2368d817` |
| generated report Markdown | `84d96534812381360070b33a79e2794e766d2b6efb9c35facaa60443809ed3a8` |

The committed raw snapshot is
[2026-07-16-projection-event-retention-pruning.json](snapshots/2026-07-16-projection-event-retention-pruning.json).

## Failure Ledger

Failures were retained and changed the harness rather than being deleted or
relabeled:

| Phase | Attempt | Failure | Resolution |
|---|---:|---|---|
| formal profile | 1 | revision `9dc6d08` reached the 30-minute test timeout while serially draining the first candidate tenant | kept all frozen counts and batch size, parallelized only independent per-tenant catch-up, and reran from a fresh root |
| formal restart injection | 1 | dedicated PostgreSQL stopped after delete, floor, and receipt mutations but before commit | required all three mutations to roll back, recovered the same pool, and retried the operation |
| offline replay | 1 | a manually transcribed full revision did not match the report | conflict was rejected without overwrite; replay was rerun with `git rev-parse HEAD` |
| full local test gate | 1 | retrieval ablation still used reset followed by incremental `RunOnce`, and one test still expected schema 16 | changed the ablation rebuild path to `RebuildCurrent`, updated the latest-schema assertion to 17, and reran focused, package, full, and race tests |
| protected CI run `29477995101` | 1 | Linux race instrumentation consumed a fixed 50 ms request deadline before source-formation provider entry, so the test failed while committing the begin transaction | replaced the wall-clock assumption with a controllable deadline context that expires exactly at provider entry; the focused race case passed 20 repetitions and the complete CI race set passed locally |

The failed first-run root and PostgreSQL log remain outside Git for local audit.

## Local Release Gates

After the formal report was committed, delivery compatibility was verified on
code revision `0c31816b64bbd1a27220316757bb7ae101705f67`:

- PostgreSQL-backed `go test -p 1 -count=1 ./...` passed on schema 17;
- the CI race package set passed, including runtime, authn, Web Chat, CLI, MCP,
  provider, memory backend, and retrieval ablation;
- `internal/reality` passed independently under the race detector;
- `go vet ./...`, `go mod tidy`, module-file diff, and `git diff --check`
  passed;
- operations acceptance passed migration replay, database-outage recovery,
  trimpath migration outside the repository, schema-17 dump/restore, and
  disposable projection rebuild;
- OpenClaw passed 43 tests, TypeScript typecheck, build, and package dry-run;
- GoReleaser v2.17.0 validated the config and produced Darwin and Linux
  `amd64`/`arm64` archives with passing checksums;
- the Darwin arm64 archive executed `vermory version`, while the Linux archives
  were independently identified as static x86-64 and aarch64 ELF binaries.

The local release snapshot is delivery evidence only. Protected GitHub CI and
its synthetic-merge artifact are verified separately against the final
checklist head.

## Hard Gates

All fourteen frozen hard gates passed:

- schema 17 creates tenant-isolated retention and prune audit state;
- runtime workers can read but cannot mutate retention control tables;
- no prune passes the slowest incremental cursor;
- authority writes and active-profile queries continue during prune attempts;
- interrupted prune deletion, floor, and audit changes roll back together;
- the same pool recovers after PostgreSQL restart;
- event retention reaches the calibrated bound after catch-up;
- retention floor is monotonic and idempotent replay is byte-stable;
- new subscribers below the floor rebuild with zero embedding work;
- reset after pruning requires rebuild and leaves other profiles unchanged;
- rebuilds match authority and deleted memories remain absent;
- cross-tenant pruning and receipt access are blocked;
- direct provider projection and query succeed after pruning;
- lexical and the incumbent remain default with no promotion.

## Non-Claims

- This is not months of uninterrupted wall-clock operation.
- This does not add automatic retention scheduling.
- This does not define authoritative-memory deletion or forgetting policy.
- This does not add table partitioning or cross-region queueing.
- This is not an embedding-model ranking or profile-promotion decision.
- This is not cross-host high-availability evidence.
- This is not an external sealed evaluation.
- This is not artifact signing or final release acceptance.
