# LongMemEval-S Full Vector Retrieval Qualification Evidence

Date: 2026-07-20

Status: qualified public full-dataset retrieval execution; candidate profile remains inactive

## Question

W14 established that Vermory's lexical retrieval underperformed a same-text
token-overlap baseline on the complete cleaned LongMemEval-S dataset. W28 asks
the next bounded question:

> When the same 23,867 governed session memories are projected through the
> registered production retrieval worker and queried through the production
> vector coordinator, what evidence-session ranking is returned, and does the
> path complete without degradation, lifecycle leakage, continuity leakage, or
> unaccounted provider work?

This is a retrieval qualification. It is not a full LongMemEval answer score,
an embedding-provider ranking, or a production-default switch.

## Frozen Source And Runtime

| Field | Value |
|---|---|
| Dataset | LongMemEval-S cleaned |
| Dataset revision | `98d7416c24c778c2fee6e6f3006e7a073259d48f` |
| Dataset SHA-256 | `d6f21ea9d60a0d56f34a05b609c79c88a451d2ae03597821ea3d5a9678c3a442` |
| Records / scored / abstention | `500 / 470 / 30` |
| Sessions / turns | `23,867 / 246,750` |
| Sorted record-ID SHA-256 | `f038965c54b03632f86a59104dd77848b66e3f80c08d5fbabdd3984d16457811` |
| Run ID | `longmemeval-s-full-vector-retrieval-20260719-v7` |
| Implementation | `837149e2c4ded381d749abc3b5a43513f24b40b3` |
| PostgreSQL / pgvector | `18.3 / 0.8.5` |
| Host | macOS `26.5.1`, Apple M4 arm64, 16 GiB |

The exact binary SHA-256 was
`e09b0b4389e1ea00e5b9bea40ed816b309e1ef5254d8c785aaeb21d19af6ad53`.
The frozen vector-profile and runner hashes were
`031dc0a1829a7ec9571fcd2a1a43bdb642ef121f8fd4d27645cacf026122f82c`
and `1c0b7ba13bd50d20850a451e88bf24e3e549458d169b564773eee3e122158706`.
The run used direct SiliconFlow `BAAI/bge-m3`; it did not use NewAPI.

## Long-Input Contract

The active `siliconflow-bge-m3-1024-v1` profile was not modified. W28 used the
registered candidate `siliconflow-bge-m3-1024-chunked-mean-v2`:

| Property | Frozen value |
|---|---|
| Input policy | `utf8-byte-chunks-v1` |
| Maximum physical chunk | `7,500` UTF-8 bytes |
| Overlap | `500` bytes |
| Pooling | float64 arithmetic mean, then L2 normalization |
| Worker batch | one governed memory |
| Provider batch | at most 16 physical chunks |

No source byte is truncated. Physical chunks are transient request material,
not separately persisted memories. One governed memory still produces one
vector document with the complete source-content hash. The same policy applies
to query embeddings. Invalid UTF-8, wrong response cardinality or dimension,
and zero-norm pooled vectors fail closed.

## Retained Rejected Runs

W28 did not overwrite failures until a clean score appeared. Runs v1-v6 remain
explicitly rejected and contribute no score:

| Run | Retained reason | Durable boundary | Scored queries |
|---|---|---:|---:|
| v1 | supplied revision did not equal Git HEAD | pre-qualification | `0` |
| v2 | launch method restarted and overwrote the first failure log | `1,032` vectors | `0` |
| v3 | three-attempt transient-failure envelope exhausted | `1,280` vectors | `0` |
| v4 | five-attempt linear retry envelope exhausted | `2,560` vectors | `0` |
| v5 | bounded same-process projection recovery exhausted | `2,560` vectors | `0` |
| v6 | one 43,406-byte session was deterministically rejected with provider code `20015` | `2,592` vectors | `0` |

The v6 diagnostic replayed the 16-item physical operation individually: 15
items returned HTTP 200 and the one oversized item returned the same HTTP 400.
This ruled out another retry-only change and led to the governed chunking
policy. The normalized rejected-run ledger is part of the committed evidence.

## Formal Execution

The one-shot user LaunchAgent ran exactly once. It imported 500 isolated
conversation continuities and 23,867 active governed source memories, drained
the PostgreSQL projection outbox, then executed all three conditions for every
record:

1. `plain_token_overlap`
2. `vermory_lexical`
3. `vermory_vector`

The complete process took `7,457.74s`; vector projection took `7,210.157s`.
Maximum RSS was `55,099,392` bytes and the final database size was
`1,211,463,359` bytes. After completion the LaunchAgent was booted out and its
plist removed; no matching process or database session remained.

## Aggregate Retrieval

| Condition | K | Recall any | Recall all | nDCG | MRR |
|---|---:|---:|---:|---:|---:|
| token overlap | 5 | `0.9064` | `0.7234` | `0.7690` | `0.8063` |
| token overlap | 10 | `0.9489` | `0.8383` | `0.7983` | `0.8119` |
| token overlap | 12 | `0.9596` | `0.8660` | `0.8036` | `0.8128` |
| Vermory lexical | 5 | `0.8234` | `0.6000` | `0.6529` | `0.6871` |
| Vermory lexical | 10 | `0.9021` | `0.7340` | `0.6918` | `0.6974` |
| Vermory lexical | 12 | `0.9213` | `0.7638` | `0.7005` | `0.6991` |
| Vermory vector | 5 | `0.9638` | `0.8553` | `0.8901` | `0.8978` |
| Vermory vector | 10 | `0.9830` | `0.9404` | `0.9069` | `0.9006` |
| Vermory vector | 12 | `0.9872` | `0.9553` | `0.9100` | `0.9010` |

At K=10, vector improved RecallAll by `0.2064`, nDCG by `0.2151`, and MRR by
`0.2032` over Vermory lexical. It also improved RecallAll by `0.1021`, nDCG by
`0.1086`, and MRR by `0.0887` over token overlap. These are measured public
dataset results, not promotion thresholds.

## Question Types At K10

| Question type | Count | Token RecallAll | Lexical RecallAll | Vector RecallAll |
|---|---:|---:|---:|---:|
| knowledge-update | 72 | `0.9583` | `0.8889` | `0.9861` |
| multi-session | 121 | `0.7438` | `0.5620` | `0.9421` |
| single-session-assistant | 56 | `0.9643` | `0.7321` | `1.0000` |
| single-session-preference | 30 | `0.6333` | `0.6333` | `0.9333` |
| single-session-user | 64 | `0.9844` | `0.9375` | `0.9688` |
| temporal-reasoning | 127 | `0.7795` | `0.7323` | `0.8740` |

Vector materially closes the previously measured multi-session and
assistant-side retrieval deficits. It is not uniformly best in every cohort:
token overlap remains slightly higher for single-session-user RecallAll.

## Classification At K12

| Condition | All evidence | Partial evidence | No evidence | Abstention unscored |
|---|---:|---:|---:|---:|
| token overlap | `407` | `44` | `19` | `30` |
| Vermory lexical | `359` | `74` | `37` | `30` |
| Vermory vector | `449` | `15` | `6` | `30` |

The 21 vector partial/no-evidence records remain listed in the normalized score
snapshot. They are future diagnosis inputs, not removed outliers.

## Provider Accounting And Latency

| Check | Result |
|---|---:|
| Logical embedding operations | `24,367` |
| Logical items, actual / expected | `24,367 / 24,367` |
| Physical provider items, actual / expected | `46,657 / 46,657` |
| Provider attempts | `25,455` |
| Failed attempts followed by retry | `1,088` |
| Terminal embedding failures | `0` |
| Recovered / unrecovered worker failures | `0 / 0` |
| Effective / degraded vector queries | `500 / 0` |

The 1,088 failed attempts were operation-local transient failures; every one
ultimately succeeded inside the frozen retry envelope, so no projection-level
recovery was entered. They still represent real provider and duration cost.

| Condition | Mean | p50 | p95 | Max |
|---|---:|---:|---:|---:|
| token overlap | `16.88ms` | `13ms` | `32ms` | `118ms` |
| Vermory lexical | `56.37ms` | `54ms` | `72ms` | `104ms` |
| Vermory vector | `199.28ms` | `178ms` | `274ms` | `4,048ms` |

One-memory durable commit granularity prevented prefix loss but projected only
about `3.31` logical memories per second in this run. This is accepted as
qualification evidence, not as an operationally optimal default.

## Hard Gates

The generated report and independent PostgreSQL checks agreed on:

```text
conversation continuities: 500
observations / distinct observation operation IDs: 23867 / 23867
active governed memories: 23867
lexical projection rows: 23867
candidate vector rows: 23867
projection: idle, lag 0, rebuild_required false
retrieval runs / distinct operation IDs: 500 / 500
effective vector / degraded vector: 500 / 0
delivered memory IDs: 6000
scope or lifecycle violations: 0
vector-to-authority content-hash violations: 0
runtime failures: 0
terminal provider failures: 0
```

The candidate profile remained `candidate` with no activation timestamp. The
existing `siliconflow-bge-m3-1024-v1` profile remained the active profile.

## Retained Attribution

- `0a995998`: vector placed all three answer sessions at ranks 1-3. The earlier
  wrong multi-session count remains a downstream reader aggregation failure.
- `6a1eabeb`: vector placed the newer answer session first and the older one
  third, reaching RecallAll `1.0` at K=5. The benchmark import still represents
  them as separate source memories rather than inventing a supersession edge.

## Evidence Files

- [normalized scores](snapshots/2026-07-20-longmemeval-s-full-vector-retrieval-scores.json)
- [normalized failure ledger](snapshots/2026-07-20-longmemeval-s-full-vector-retrieval-failures.json)
- [normalized execution manifest](snapshots/2026-07-20-longmemeval-s-full-vector-retrieval-execution.json)
- [rejected-run ledger](snapshots/2026-07-20-longmemeval-s-full-vector-retrieval-rejected-runs.json)
- [512-entry raw artifact hash manifest](snapshots/2026-07-20-longmemeval-s-full-vector-retrieval-raw-artifacts.sha256)

The raw report, scores, retrieval JSONL, 500 checkpoints, logs, source summary,
and run-exit file remain on the Mac mini. Their complete manifest SHA-256 is
`1c5e24232a62fb95faa4b790c54b38d550a96d0e165977e4bba9589c695dc9dc`.
The 277 MB upstream dataset and 2.9 MB record-level report remain outside Git.

## Decision

- W28 qualifies the candidate vector path on the complete public
  LongMemEval-S retrieval dataset.
- The quality result is strong enough to justify a separately identified
  reader QA replay using the frozen vector K=10 rankings.
- The public result does not activate the candidate or switch the product
  default. Profile promotion still requires calibrated operational thresholds
  and evidence beyond this public corpus.
- Projection throughput and provider retry cost remain explicit optimization
  targets; they are not hidden by the quality gain.

## Non-Claims

- This is not a full LongMemEval QA score and uses no LLM judge.
- LongMemEval-M was not executed.
- Governed session import is not automatic formation from raw chats.
- This does not rank embedding providers or language models.
- This is public benchmark evidence, not withheld or externally sealed data.
- W28 does not complete the overall Vermory platform goal.
