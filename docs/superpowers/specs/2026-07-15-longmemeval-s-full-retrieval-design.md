# LongMemEval-S Full Retrieval Qualification Design

## Purpose

W14 qualifies Vermory's production retrieval path against every record in the
pinned cleaned LongMemEval-S artifact. It answers a narrower and more useful
question than the existing six-record oracle QA sample:

> When the complete timestamped conversation history is stored behind the
> correct conversation continuity, does Vermory retrieve the official evidence
> sessions without crossing continuity boundaries?

This stage intentionally separates retrieval from reader-model correctness.
The existing sample contains two unresolved QA failures, one multi-session
counting case and one knowledge-update case. W14 determines whether those
failures begin in retrieval or remain after all required evidence is present.

## Upstream Source

The source is the official cleaned LongMemEval release, not a translated case
or a locally authored fixture.

| Field | Frozen value |
|---|---|
| Repository | `https://github.com/xiaowu0162/LongMemEval` |
| Repository revision | `9e0b455f4ef0e2ab8f2e582289761153549043fc` |
| Dataset repository | `https://huggingface.co/datasets/xiaowu0162/longmemeval-cleaned` |
| Dataset revision | `98d7416c24c778c2fee6e6f3006e7a073259d48f` |
| Artifact | `longmemeval_s_cleaned.json` |
| Artifact size | `277383467` bytes |
| Artifact SHA-256 | `d6f21ea9d60a0d56f34a05b609c79c88a451d2ae03597821ea3d5a9678c3a442` |
| Records | `500` |
| Sessions | `23867` |
| Turns | `246750` |
| Sorted record-ID SHA-256 | `f038965c54b03632f86a59104dd77848b66e3f80c08d5fbabdd3984d16457811` |
| Official retrieval metric script | `src/evaluation/print_retrieval_metrics.py` |
| Metric script SHA-256 | `58b70c0b562ea57372a7774a554c347cd908e901b77ac0149fc90b097b6f1b8f` |

The record IDs are byte-for-byte identical to the qualified oracle artifact's
record-ID set. LongMemEval-S contains 38 to 62 sessions per record, averaging
47.734 sessions. The 30 abstention records remain in execution evidence but
are excluded from official retrieval aggregates, matching upstream behavior.

## Evidence Semantics

The existing `benchmark_wide` claim is too broad for a run that evaluates only
one target such as retrieval or QA. W14 adds two explicit dimensions:

- `evaluation_target`: `retrieval` or `qa`;
- `claim_scope`: `dataset_sample`, `qualified_dataset_full`, or
  `benchmark_wide`.

`qualified_dataset_full` means that every record in one pinned artifact was
executed for the named target. `benchmark_wide` remains reserved for an
execution that satisfies the complete upstream task, input variant, and scorer
contract. W14 uses:

```text
execution_scope = full
evaluation_target = retrieval
claim_scope = qualified_dataset_full
```

The full manifest stores `selection_mode=all_records` and the canonical sorted
record-ID digest. It does not copy 500 IDs into a hand-maintained selection
list and does not require a sample fixture.

## Runtime Shape

The run uses a dedicated PostgreSQL database and a stable tenant:

```text
tenant = benchmark:<run-id>
one conversation continuity per LongMemEval record
one governed active source memory per official history session
one idempotent operation ID per imported session
one production lexical retrieval request per record at K=12
```

Each source memory contains only timestamped semantic session text. Its memory
ID is mapped to the official session occurrence at import time; model-facing
or report content never needs to infer IDs from text. The production
`RetrievalCoordinator` performs the Vermory query. Direct SQL ranking or an
in-memory imitation cannot count as the Vermory condition.

The cleaned S artifact contains 13 records where one distractor session ID is
repeated at two timestamps. In every case the content is identical, the dates
differ, and the duplicated ID is not an answer session. This is valid upstream
data, not a loader error. Vermory stores each position as a distinct governed
memory using an occurrence key while retaining the raw official session ID for
upstream-compatible metrics. A repeated distractor may therefore occupy two
ranking positions exactly as it does in the official evaluator.

The artifact also contains 12 unlabeled turns with an empty `content` field:
nine user turns and three assistant turns. No complete session is empty and no
empty turn is answer-labeled. The streaming loader retains them in source turn
counts, while semantic session text omits their empty content exactly as the
upstream flat retriever does. Missing roles, empty answer-labeled turns, and
sessions with no non-empty content remain invalid.

The runner computes K=5 and K=10 metrics from the K=12 ranking prefix. It also
executes a deterministic token-overlap baseline over the same official
sessions. Both conditions receive the same question text and use the same
session-level granularity.

## Metrics

For every non-abstention record and condition, W14 records:

- `recall_any@5` and `recall_all@5`;
- `ndcg_any@5`;
- `recall_any@10` and `recall_all@10`;
- `ndcg_any@10`;
- `recall_any@12` and `recall_all@12` as a Vermory product-limit diagnostic;
- first relevant rank and reciprocal rank;
- retrieved official session occurrence keys and raw IDs in exact order;
- latency and failure category.

The DCG and recall definitions reproduce the pinned upstream evaluator. The
report aggregates overall results and the six official question types. The 30
abstention records are reported separately and do not enter the official
retrieval mean.

## Failure Attribution

Every answerable record is classified into one of these mutually exclusive
outcomes:

- `all_evidence_retrieved`;
- `partial_evidence_retrieved`;
- `no_evidence_retrieved`;
- `runtime_failure`.

Known six-record QA failures receive an explicit cross-reference showing their
retrieval outcome. W14 does not change retrieval behavior to make those cases
pass. Any improvement is a later hypothesis with a separately frozen case.

## Resume And Reproducibility

The 277 MB source is streamed record by record so the same loader can later
support LongMemEval-M without loading a multi-gigabyte artifact into memory.
Each completed record writes an atomic checkpoint containing:

- dataset SHA-256;
- record-ID digest;
- run ID and implementation revision;
- condition results and imported-memory count.

`--resume` accepts only checkpoints matching all frozen identifiers. Results
are sorted by official record ID before final aggregation, so worker scheduling
cannot change the report hash. Repeating a completed run against the same
database must replay idempotent imports rather than create duplicate memories.

## Hard Gates

W14 fails if any of the following occurs:

- source size, SHA-256, record count, session count, or record-ID digest differs;
- fewer or more than 500 records execute;
- the 470 non-abstention records do not all have metrics;
- a retrieved memory belongs to another record continuity;
- a non-active memory is returned;
- imported memory count differs from 23,867;
- an operation replay creates a duplicate governed memory;
- a runtime failure is omitted from the final report;
- final aggregates differ between a clean run and an idempotent resume;
- the report labels this run as full LongMemEval QA or an official QA score.

Quality metrics are reported, not converted into an arbitrary pass threshold.
Isolation, completeness, lifecycle state, source integrity, and reproducibility
are hard gates; recall is evidence used to drive the next implementation
hypothesis.

## Artifacts

The runner writes under `artifacts/benchmarks/<run-id>/`:

- `source.json` with pinned source and record-set evidence;
- `checkpoints/<record-id>.json` with atomic per-record results;
- `retrieval-results.jsonl` in upstream-compatible record order;
- `scores.json` with per-record and aggregate deterministic metrics;
- `failure-ledger.json` with every non-success category;
- `report.md` with scope, metrics, known-case attribution, and non-claims;
- `execution-manifest.json` validated after all artifacts exist.

Credentials are not needed for the lexical W14 run and must not appear in any
artifact. The 277 MB upstream dataset remains outside Git and is referenced by
its pinned digest.

## Acceptance

W14 is accepted only when:

- manifest and streaming-loader rejection tests pass;
- a disposable PostgreSQL integration test proves the complete production
  retrieval path, source-ID mapping, isolation, idempotent resume, and atomic
  checkpoint behavior;
- the real pinned 500-record artifact completes with all hard gates passing;
- the report retains every retrieval failure and separates it from reader QA;
- local PostgreSQL, race, vet, module, OpenClaw, and release gates pass;
- protected GitHub CI passes and its newly generated release artifact is
  independently verified;
- Draft PR 1 is updated while the overall Vermory platform goal remains active.

## Non-Claims

- W14 is not a full LongMemEval QA score.
- W14 does not use the official GPT-4o answer judge.
- W14 does not evaluate LongMemEval-M.
- W14 does not prove automatic memory formation from raw chats; it evaluates
  governed import and retrieval of official sessions.
- W14 does not select a superior model or embedding provider.
- W14 is public benchmark evidence, not withheld or externally sealed evidence.
