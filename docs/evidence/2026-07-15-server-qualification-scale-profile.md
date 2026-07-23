# Server Qualification Scale Profile Evidence

Date: 2026-07-15

Status: `server-qualification-v1` supported for the measured operational gates

## Scope

W12 qualifies one named PostgreSQL-authoritative server profile on fixed
hardware. The case uses real Vermory authority and revision transactions,
durable trigger-generated projection events, current-authority snapshot
bootstrap, PostgreSQL/pgvector/HNSW storage, scoped runtime retrieval,
concurrent deletion, competing tail workers, and a final direct-provider
projection/query probe.

The case is operational rather than a synthetic memory-quality claim. Scale
vectors are deterministic 1,024-dimension fixtures so the run can exercise
100,000 real pgvector rows without making 100,000 billable provider calls. A
separate tenant uses direct SiliconFlow `BAAI/bge-m3` for one projection and one
query after all scale gates.

## Execution

| Field | Value |
|---|---|
| Run | `w12-server-qualification-scale-profile-20260715-v2` |
| Implementation | `c41edfa423de0ccda7b52262942aef3f1f14d101` |
| Case version | `2` |
| Case SHA-256 | `1c67881c73546e88f2b8effa7252773ca38e7b0af315bc6e22f83e4937a9a90f` |
| Raw log SHA-256 | `8854564d5e2ae4ee7f6f5c6447917ea436df474a6c5733b026013e0af8a4cd2f` |
| OS | `Darwin 27.0 arm64` |
| CPU / memory | Apple M4 Pro / 48 GiB |
| Go | `go1.26.5 darwin/arm64` |
| PostgreSQL / pgvector | `18.4` / `0.8.5` |
| Retrieval profile | `siliconflow-bge-m3-1024-v1` |
| Real provider route | direct SiliconFlow `BAAI/bge-m3` |
| Total test duration | `465.35 s` |

Normalized evidence:
[W12 JSON](snapshots/2026-07-15-server-qualification-scale-profile.json).

Raw successful execution:
[W12 log](snapshots/2026-07-15-server-qualification-scale-profile.log).

Preserved first attempt:
[W12 attempt 1](snapshots/2026-07-15-server-qualification-scale-profile-attempt-1-implementation.log).

Both committed logs were scanned for credential-shaped `sk-*` strings and
contained zero matches.

## Authority And Event Results

| Gate | Result |
|---|---:|
| Tenants / continuities | `10 / 100` |
| Initial active facts | `100,000` |
| Append-only governed revisions | `450,000` |
| Final governed memories | `550,000` |
| Active / superseded before deletion | `100,000 / 450,000` |
| Durable projection events before tail | `1,000,000` |
| Concurrent delete events | `1,000` |
| Final durable projection events | `1,001,000` |
| Active facts after deletion | `99,000` |

The 450,000 revisions were not in-place updates created to inflate an event
counter. Each revision wrote a new source-update observation and active governed
memory, linked it with `supersedes_memory_id`, and marked the previous revision
`superseded` in the same tenant-scoped transaction. Initial activations plus
revision activations and supersessions therefore produced exactly 1,000,000
trigger events while preserving 100,000 current facts.

The multi-tenant lag assertion counted actual pending rows for each tenant. It
did not use arithmetic distance between globally interleaved event IDs.

## Projection Results

| Gate | Result |
|---|---:|
| Current lexical rows before deletion | `100,000` |
| Current vector rows before deletion | `100,000` |
| Snapshot embedding requests | `100,000` |
| Historical events avoided during bootstrap | `900,000` |
| Current lexical rows after deletion | `99,000` |
| Current vector rows after deletion | `99,000` |
| Deleted authority/search/vector residue | `0` |
| Final lag for every tenant | `0` |

Each tenant snapshot captured a watermark and rebuilt from current active
authority rather than replaying all historical events. The run therefore made
one deterministic embedding request per current fact, not one per event. The
ordinary workers consumed only the 1,000 deletion events created after the
snapshot watermark.

Two workers raced for every tenant/profile advisory lock. Exactly ten workers
reported `already_running`, the winners processed exactly 1,000 tail events,
and subsequent bounded catch-up left every cursor current without duplicate
vectors or backward movement.

## Query Results

| Metric | Result |
|---|---:|
| Concurrent clients | `50` |
| Total scoped queries | `1,000` |
| Requested lexical | `450` |
| Requested vector | `550` |
| Effective vector | `138` |
| Controlled vector-to-lexical fallback | `412` |
| Correct target deliveries | `1,000` |
| Cross-tenant leaks | `0` |
| Cross-continuity leaks | `0` |
| P50 / P95 / P99 | `8 / 234 / 266 ms` |

All 1,000 requests returned the exact expected active memory while deletion and
tail processing were concurrent. The p95 and p99 values remained below the
frozen `2,000 ms` and `5,000 ms` limits.

The original payload recorded aggregate degradation but did not store its
failure-code distribution. A later attribution replay at implementation
`29a3429` added an audit hard gate and measured 550 requested vector calls, 145
effective vector results, 405 `projection_lag` fallbacks, and zero other
degraded results. The exact `effective/lag` split varies with scheduling, but
the invariant held: every requested vector query was either current and served
by vector retrieval or intentionally rejected while delete events made that
tenant's projection non-current.

A separate 100,000-vector control with no concurrent authority changes served
all 550 requests as effective vector results with zero degradation. W12's
fallbacks are therefore expected projection-lag safety behavior, not evidence
of a server-scale ANN recall defect. See
[the attribution evidence](2026-07-15-vector-degradation-attribution.md).

## Phase Durations And Size

| Phase | Result | Frozen limit |
|---|---:|---:|
| Authority seed | `10.699 s` | `1,800 s` |
| Governed history generation | `102.709 s` | `1,200 s` |
| Lexical rebuild | `11.301 s` | `300 s` |
| Vector snapshot | `317.345 s` | `1,800 s` |
| Tail catch-up | `18.431 s` | `300 s` |
| Concurrent query and deletion phase | `19.381 s` | reported |
| Database size | `2,710,910,655 bytes` | `20 GiB` |

## Direct Provider Probe

After the scale phases, a separate tenant projected one current governed fact
with direct SiliconFlow `BAAI/bge-m3` and embedded one query with the same
profile. The production coordinator returned the expected memory with
`Effective == vector`, `Degraded == false`, and exactly two provider requests.
No API key was written to the command line, repository, evidence, or log.

## Preserved Failure And Fix

The first full-profile attempt at revision `1119d6e` failed before seeding.
`pgxpool.ParseConfig` correctly consumed `pool_max_conns`, but `Store.Migrate`
reopened the original raw DSN through `database/sql`, causing PostgreSQL to
reject the pgxpool-only parameter as an unknown server setting. The failed log
has SHA-256
`a557fc6d745cb80136556955e70b85bf3131034c7b5e7b6f25c36b81e59c2dd2`.

A regression test reproduced the same failure against disposable PostgreSQL.
Revision `c41edfa423de0ccda7b52262942aef3f1f14d101` changed migrations to use
`stdlib.OpenDBFromPool`, preserving the parsed pool configuration and avoiding a
second raw-DSN path. The regression, miniature profile, runtime package, and
runtime race package passed before the successful full run.

## Decision

The measured `server-qualification-v1` profile supports continued use of
PostgreSQL as Vermory's authority, transactional outbox, lexical projection,
and optional vector projection without a required Redis service. Current-state
snapshot bootstrap makes one million retained events compatible with 100,000
current vectors without embedding obsolete history, and ordinary tail workers
preserve deletion and cursor semantics under competition.

The decision does not promote semantic retrieval to the default. W12 qualifies
the projection-current gate and exact lexical fallback during concurrent
authority change; semantic quality remains covered by the separate W08 and W10
retrieval cases rather than this operational scale fixture.

## Protected Delivery

Head `9b9c0616e6595963dd3a196f5a16406542af4747` passed protected CI run
[`29407243728`](https://github.com/jstar0/Vermory/actions/runs/29407243728),
job `87325577789`, in `4m33s`. The workflow completed the PostgreSQL suite,
runtime and Reality race sets, vet, module-drift check, release build, OpenClaw
check/package, four-platform GoReleaser snapshot, artifact upload, and clean
diff gate.

Artifact `8339654995`
(`vermory-pr-snapshot-c45a6ae1cf5b12b06f076ef1ac452851dcd28518`) is
20,990,790 bytes with GitHub digest
`sha256:cbd94db4587906a745c892e60c3c1e2ed72c1332304a0a57c617d9d002159882`.
The independently downloaded transport ZIP had the same SHA-256. Its four
archive checksums passed, every Go archive contained exactly `vermory`,
`LICENSE`, `README.md`, and `README.zh-CN.md`, and every binary reported the
expected GOOS/GOARCH with `CGO_ENABLED=0`, `-trimpath=true`, and
`vcs.modified=false`. The OpenClaw `0.1.0` package contained 12 expected
entries. The downloaded Darwin arm64 binary executed `version` and
`retrieval-snapshot-rebuild --help`.

The artifact revision is GitHub's verified synthetic merge commit
`c45a6ae1cf5b12b06f076ef1ac452851dcd28518`; its second parent is the tested
head `9b9c0616e6595963dd3a196f5a16406542af4747`. At verification time Draft PR 1
was `OPEN`, `CLEAN`, `MERGEABLE`, and required `test=SUCCESS`. No tag or GitHub
Release existed.

## Non-Claims

- This is not a one-million-vector-row qualification.
- This is not HA, failover, replication, PITR, or cross-region evidence.
- This does not qualify unlimited projection-event retention.
- This does not test a dimensionality migration while backlog is active.
- This does not measure memory formation quality or benchmark superiority.
- This does not switch the lexical default to semantic retrieval.
- This does not complete sealed evaluation, signing, or final release gates.
