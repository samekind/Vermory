# LongMemEval-S Full Retrieval Qualification Evidence

Date: 2026-07-15

Status: qualified public full-dataset retrieval execution; QA and sealed evaluation remain open

## Question

The existing original LongMemEval evidence ran six oracle records through a
real Grok reader. It established the execution path but could not answer
whether Vermory retrieves evidence reliably across the complete long-history
dataset. It also retained two different platform-quality problems:

- `0a995998` returned the wrong multi-session count;
- `6a1eabeb` returned the correct updated value while also mentioning an older
  conflicting value.

W14 separates retrieval from reader reasoning. It executes every record in the
official cleaned LongMemEval-S artifact and records whether the official answer
sessions are present in the production Vermory lexical ranking.

## Frozen Source

| Field | Value |
|---|---|
| Benchmark | `LongMemEval`, S cleaned variant |
| Upstream repository revision | `9e0b455f4ef0e2ab8f2e582289761153549043fc` |
| Dataset revision | `98d7416c24c778c2fee6e6f3006e7a073259d48f` |
| Artifact | `longmemeval_s_cleaned.json` |
| Size | `277,383,467 bytes` |
| SHA-256 | `d6f21ea9d60a0d56f34a05b609c79c88a451d2ae03597821ea3d5a9678c3a442` |
| Records | `500` |
| Scored / abstention | `470 / 30` |
| Sessions | `23,867` |
| Turns | `246,750` |
| Sorted record-ID SHA-256 | `f038965c54b03632f86a59104dd77848b66e3f80c08d5fbabdd3984d16457811` |
| Official retrieval metric script SHA-256 | `58b70c0b562ea57372a7774a554c347cd908e901b77ac0149fc90b097b6f1b8f` |

The source was downloaded from the pinned Hugging Face revision and verified
before PostgreSQL was opened.

## Source-Shape Corrections

The real 277 MB artifact invalidated two assumptions from the original small
fixture:

1. Thirteen records repeat one non-answer distractor session ID at two
   timestamps. The repeated content is identical, the dates differ, and no
   answer session is duplicated. W14 stores both positions as distinct governed
   memories using occurrence keys while retaining the raw ID for official
   metrics.
2. Twelve turns have an empty, unlabeled `content` field: nine user turns and
   three assistant turns. No session is entirely empty and no empty turn is
   answer-labeled. W14 retains these turns in source counts and omits their
   empty semantic content, matching the upstream flat retriever.

These are recorded source-contract corrections, not removed records.

## Execution

| Field | Value |
|---|---|
| Run ID | `longmemeval-s-full-retrieval-20260715-v1` |
| Implementation | `0f59bf58d9a107ce47f79ba86fd61a7c35c8324b` |
| Binary | CGO-free, `-trimpath`, Darwin arm64 |
| OS / CPU / memory | `Darwin 27.0.0 arm64` / Apple M4 Pro / 48 GiB |
| Go | `go1.26.5 darwin/arm64` |
| PostgreSQL / pgvector | `18.4` / `0.8.5` |
| Clean run | `156.30 s`, max RSS `46,088,192 bytes` |
| Resume proof | `5.03 s`, max RSS `39,272,448 bytes` |
| Database size | `824,350,399 bytes` |

Each official record received one isolated conversation continuity. Every
timestamped session was committed through `CommitGovernedObservation` as an
active `source_update`; the production `RetrievalCoordinator` then executed one
lexical query at K=12. K=5 and K=10 metrics use ranking prefixes. The comparison
condition is a deterministic token-overlap ranking over the same timestamped
session text.

The 46 MB maximum resident set is materially below the 277 MB source size and
supports the streaming-loader claim. It is not a general memory-usage profile.

## Aggregate Results

| Condition | K | Recall any | Recall all | nDCG | MRR |
|---|---:|---:|---:|---:|---:|
| `plain_token_overlap` | 5 | `0.9064` | `0.7234` | `0.7690` | `0.8063` |
| `plain_token_overlap` | 10 | `0.9489` | `0.8383` | `0.7983` | `0.8119` |
| `plain_token_overlap` | 12 | `0.9596` | `0.8660` | `0.8036` | `0.8128` |
| `vermory_lexical` | 5 | `0.8234` | `0.6000` | `0.6529` | `0.6871` |
| `vermory_lexical` | 10 | `0.9021` | `0.7340` | `0.6918` | `0.6974` |
| `vermory_lexical` | 12 | `0.9213` | `0.7638` | `0.7005` | `0.6991` |

W14 does not hide the regression: the current production lexical ranking is
worse than the simple token-overlap baseline on this English long-history
corpus. At K=10, Vermory loses `0.1043` RecallAll and `0.1065` nDCG. This does
not justify changing the product default inside W14; it creates a measured
retrieval hypothesis for a later frozen comparison.

## Type Results At K10

| Question type | Count | Condition | Recall any | Recall all | nDCG | MRR |
|---|---:|---|---:|---:|---:|---:|
| knowledge-update | 72 | token overlap | `0.9861` | `0.9583` | `0.9048` | `0.9439` |
| knowledge-update | 72 | Vermory lexical | `0.9861` | `0.8889` | `0.8290` | `0.8604` |
| multi-session | 121 | token overlap | `0.9752` | `0.7438` | `0.7678` | `0.8659` |
| multi-session | 121 | Vermory lexical | `0.9421` | `0.5620` | `0.6318` | `0.7111` |
| single-session-assistant | 56 | token overlap | `0.9643` | `0.9643` | `0.8719` | `0.7899` |
| single-session-assistant | 56 | Vermory lexical | `0.7321` | `0.7321` | `0.6698` | `0.6201` |
| single-session-preference | 30 | token overlap | `0.6333` | `0.6333` | `0.4030` | `0.2801` |
| single-session-preference | 30 | Vermory lexical | `0.6333` | `0.6333` | `0.4401` | `0.3135` |
| single-session-user | 64 | token overlap | `0.9844` | `0.9844` | `0.9196` | `0.8492` |
| single-session-user | 64 | Vermory lexical | `0.9375` | `0.9375` | `0.8084` | `0.6740` |
| temporal-reasoning | 127 | token overlap | `0.9528` | `0.7795` | `0.7666` | `0.8021` |
| temporal-reasoning | 127 | Vermory lexical | `0.9370` | `0.7323` | `0.6817` | `0.7285` |

The largest complete-evidence deficit is multi-session retrieval. Preference
retrieval is weak for both lexical conditions, which is consistent with
questions that often require latent personal information rather than direct
word overlap.

## Failure Classification

At K=12, the 500 records classify as:

| Condition | All evidence | Partial evidence | No evidence | Abstention unscored |
|---|---:|---:|---:|---:|
| `plain_token_overlap` | `407` | `44` | `19` | `30` |
| `vermory_lexical` | `359` | `74` | `37` | `30` |

These are quality outcomes, not runtime failures. The runtime failure ledger is
empty.

## Retained Sample Attribution

### `0a995998`: multi-session counting

Vermory retrieved all three official answer sessions at ranks 1, 2, and 3:

```text
answer_afa9873b_3
answer_afa9873b_1
answer_afa9873b_2
```

RecallAll is `1.0` at K=5. The prior answer of `2` instead of `3` is therefore
a reader aggregation failure, not missing context.

### `6a1eabeb`: knowledge update

Vermory ranked the newer answer session `answer_a25d4a91_2` first and the older
answer session `answer_a25d4a91_1` sixth. RecallAll is `0.0` at K=5 and `1.0`
at K=10. The evidence is available, but imported sessions remain separate
source memories rather than an explicit supersession chain. W14 does not
rewrite that source lifecycle after seeing the result.

## Hard Gates

The final PostgreSQL state contains:

```text
conversation continuities: 500
observations: 23,867
distinct observation operation IDs: 23,867
governed memories: 23,867
active governed memories: 23,867
lexical projection rows: 23,867
runtime failures: 0
cross-continuity or unmapped retrieval results: 0
```

The same binary, run ID, database, and artifact root then completed with
`--resume` in 5.03 seconds. It created zero new observations or memories.
Normalized `scores.json`, `failure-ledger.json`, and
`retrieval-results.jsonl` retained SHA-256 values
`2fd109eb...81bb`, `74234e98...b90b`, and `a4caa0a6...27ad`.

The benchmark label was normalized from the source variant name
`LongMemEval-S` to benchmark `LongMemEval` before the final resume artifact;
the S variant remains explicit in the qualification path, source filename,
hashes, and evidence title. Retrieval scores and failure evidence were
byte-identical across that metadata correction.

## Evidence Files

- [normalized scores](snapshots/2026-07-15-longmemeval-s-full-retrieval-scores.json)
- [normalized failure ledger](snapshots/2026-07-15-longmemeval-s-full-retrieval-failures.json)
- [normalized execution manifest](snapshots/2026-07-15-longmemeval-s-full-retrieval-execution.json)

Raw run hashes are included in the normalized score snapshot. The upstream
277 MB dataset and the raw 1.2–2.0 MB per-record artifacts remain outside Git.
All committed W14 evidence was scanned for credential-shaped `sk-*` strings.

## Decision

- Keep lexical as the current product default; W14 is diagnosis, not a silent
  retrieval cutover.
- Treat LongMemEval-S multi-session and assistant-side evidence loss as a
  measured retrieval-quality problem.
- Treat `0a995998` as a downstream reader/aggregation problem.
- Treat `6a1eabeb` as both ranking pressure and a source-lifecycle expression
  problem.
- Use W14 failures to freeze the next retrieval comparison instead of tuning
  on the same 500 labels without a holdout.

## Non-Claims

- This is not a full LongMemEval QA score.
- No reader model or official GPT-4o answer judge was used.
- LongMemEval-M was not executed.
- Governed import of official sessions does not prove automatic memory
  formation from raw chat streams.
- This result does not select an embedding model or prove vector superiority.
- This public benchmark run is not withheld or externally sealed evaluation.
- W14 does not complete the overall Vermory platform goal.
