# Vector Degradation Attribution Evidence

Date: 2026-07-15

Status: W12 degradation attributed; scoped-HNSW hypothesis rejected

## Question

The original W12 run completed 1,000 scoped queries correctly, but only 138 of
550 requested vector calls remained effective vector results. The other 412
calls used the exact lexical fallback. The original payload did not store the
failure-code distribution, so it was not valid to infer the cause from the
aggregate count alone.

An initial follow-up hypothesis blamed HNSW recall under selective tenant and
continuity filters. W13 tested that explanation before changing production
retrieval.

## Execution

| Field | Value |
|---|---|
| Case | `W13-vector-degradation-attribution` |
| Case SHA-256 | `ae00edce919813287c21ff06f53a8d104b911254a4cb34c5648e57eeb0235125` |
| Attribution implementation | `29a3429` |
| Projection-current control implementation | `b1ca093` |
| OS / CPU / memory | `Darwin 27.0 arm64` / Apple M4 Pro / 48 GiB |
| Go | `go1.26.5 darwin/arm64` |
| PostgreSQL / pgvector | `18.4` / `0.8.5` |

Normalized evidence:
[W13 JSON](snapshots/2026-07-15-vector-degradation-attribution.json).

Primary raw runs:

- [instrumented W12 replay](snapshots/2026-07-15-server-qualification-scale-attribution.log), SHA-256 `88b876f933876d650522ba7f50cb1d3cbff20a0f16e6b4f41fba1cab1795e413`;
- [100k projection-current control](snapshots/2026-07-15-vector-current-100k-control.log), SHA-256 `7407b497ebfba629971bb21ff08705d9b6b45fb3796cf3c7b729f062a5f46810`.

All committed W13 logs were scanned for credential-shaped `sk-*` strings and
contained zero matches.

## Projection-Current Control

The control created 100,000 active governed facts and 100,000 current vectors
across the same ten tenants and 100 continuities used by W12. It issued all 550
queries in vector mode without concurrent deletes or new projection events.

| Metric | Result |
|---|---:|
| Requested vector queries | `550` |
| Effective vector results | `550` |
| Degraded results | `0` |
| Cross-scope leaks | `0` |
| P50 / P95 / P99 | `134 / 248 / 287 ms` |
| Vector snapshot | `352.484 s` |
| Database size | `1,728,173,759 bytes` |

This result falsifies the claim that the current 100,000-vector shape itself
causes the W12 fallback rate.

## Concurrent W12 Replay

The instrumented replay retained the complete W12 workload: 100,000 current
facts, 450,000 append-only revisions, one million initial projection events,
1,000 concurrent deletes, competing tail workers, 1,000 total queries, and the
two-request direct SiliconFlow post-scale probe.

The harness now aggregates the 550 vector audit rows by effective mode and
failure code and rejects any degraded category other than `projection_lag`.

| Metric | Result |
|---|---:|
| Requested vector queries | `550` |
| Effective vector results | `145` |
| `projection_lag` fallback | `405` |
| Other degraded results | `0` |
| Correct total task deliveries | `1,000` |
| Cross-scope leaks / final lag | `0 / 0` |
| P50 / P95 / P99 | `13 / 212 / 253 ms` |
| Real provider requests | `2` |

The effective/lag split differs slightly from the original `138/412` because
the exact interleaving of 50 query clients, ten deleters, and competing tail
workers is scheduler-dependent. The invariant is exact:

```text
145 effective vector + 405 projection_lag fallback = 550 requested vector
other degraded = 0
```

Vermory therefore behaved as designed. Once a delete event made a tenant's
projection non-current, the coordinator refused vector delivery and used the
exact lexical result until tail processing restored zero lag.

## Rejected HNSW Investigation

The investigation retained six attempts instead of deleting them:

| Attempt | Result |
|---|---|
| Medium simplified query | scope B-tree selected; not an HNSW reproduction |
| Medium after scope-index removal | sequential scan selected |
| Medium with seqscan disabled | primary-key bitmap scan selected |
| Medium forced HNSW | `200/200` effective, but fixture was non-representative |
| Full simplified-plan check | evaluator inspected the wrong SQL shape |
| Full production-plan check | production SQL selected the scope B-tree and exact sort |

Raw logs:

- [medium scope index](snapshots/2026-07-15-w13-rejected-medium-scope-index.log)
- [medium sequential scan](snapshots/2026-07-15-w13-rejected-medium-seq-scan.log)
- [medium bitmap scan](snapshots/2026-07-15-w13-rejected-medium-bitmap-scan.log)
- [medium forced HNSW control](snapshots/2026-07-15-w13-medium-forced-hnsw-control.log)
- [full simplified plan](snapshots/2026-07-15-w13-rejected-full-simplified-plan.log)
- [full production plan](snapshots/2026-07-15-w13-rejected-full-production-plan.log)

These attempts prevented a plausible but incorrect `SET LOCAL
hnsw.iterative_scan` change from entering production.

## Decision

- Keep the current production vector query unchanged.
- Keep the projection-current gate and exact lexical fallback unchanged.
- Keep lexical as the product default.
- Include failure-code distributions in scale evidence instead of reporting only aggregate degradation.
- Interpret W12's controlled fallbacks as concurrent projection-lag behavior, not server-scale ANN recall failure.

## Protected Delivery

Implementation head `b918584588343d9fc029b96459f52826ce031667` passed protected
CI run [`29412393366`](https://github.com/jstar0/Vermory/actions/runs/29412393366),
job `87342311436`. The required `test` job completed every PostgreSQL, race,
vet, module, binary, OpenClaw, release-snapshot, package, and clean-diff step
successfully.

Artifact `8341752144`
(`vermory-pr-snapshot-a409d5c8abfed5f9baec7bfe932f737606af9b55`) is
20,992,109 bytes with GitHub digest
`sha256:e2ff956335d5752e26d44400447851be73a677041d381ca90652bac8899f3c46`.
The independently downloaded transport ZIP had the same SHA-256. All four
archive checksums passed and every archive contained exactly `vermory`,
`LICENSE`, `README.md`, and `README.zh-CN.md`.

All four binaries reported the expected GOOS/GOARCH, `CGO_ENABLED=0`,
`-trimpath=true`, and `vcs.modified=false`. The downloaded Darwin arm64 binary
executed `version` and `retrieval-snapshot-rebuild --help`. The OpenClaw `0.1.0`
package contained exactly 12 entries. GitHub's verified synthetic merge commit
`a409d5c8abfed5f9baec7bfe932f737606af9b55` has implementation head `b918584`
as its second parent.

Draft PR 1 remained `OPEN`, `CLEAN`, `MERGEABLE`, and required
`test=SUCCESS`. The repository had no tag or GitHub Release. The overall
Vermory platform goal remains active after W13.

## Non-Claims

- This does not establish general semantic quality or benchmark superiority.
- This does not rank pgvector indexes or embedding models.
- This does not qualify one million vectors, HA, PITR, or cross-region use.
- This does not eliminate future query-planner or ANN recall monitoring.
