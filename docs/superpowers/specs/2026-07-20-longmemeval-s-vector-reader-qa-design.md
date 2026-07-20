# LongMemEval-S Vector Reader QA Design

## Purpose

W28 proved that the registered candidate vector path improves deterministic
session retrieval on the complete public LongMemEval-S corpus. W29 asks the
downstream question that retrieval metrics cannot answer:

> With the same real reader and custom judge, does the frozen vector K=10
> context improve final answer quality over the frozen Vermory lexical K=10
> context?

W29 is a reader-utility comparison. It does not rerun retrieval, tune K, alter
governed memories, rank models, or activate a retrieval profile.

## Frozen Inputs

| Field | Value |
|---|---|
| Dataset | LongMemEval-S cleaned |
| Dataset SHA-256 | `d6f21ea9d60a0d56f34a05b609c79c88a451d2ae03597821ea3d5a9678c3a442` |
| Record-set SHA-256 | `f038965c54b03632f86a59104dd77848b66e3f80c08d5fbabdd3984d16457811` |
| Records / scored / abstention | `500 / 470 / 30` |
| Sessions / turns | `23,867 / 246,750` |
| Retrieval run | `longmemeval-s-full-vector-retrieval-20260719-v7` |
| Retrieval implementation | `837149e2c4ded381d749abc3b5a43513f24b40b3` |
| Retrieval JSONL SHA-256 | `1d2f0b50f618cdd94654c70afcfccf1ef9bc6471bc3921ff6de327a4ede453b6` |
| K | `10` |

The execution manifest is
`casebook/benchmarks/executions/longmemeval-s-full-vector-reader-qa.json`.
Formal runs supply a unique run ID and the exact implementation revision at
runtime; the final generated execution manifest records both.

## Conditions

W29 has exactly two contemporaneous conditions:

1. `vermory_lexical_k10`
2. `vermory_vector_k10`

The input JSONL must contain the original W28 sequence
`plain_token_overlap`, `vermory_lexical`, `vermory_vector`. The runner validates
all three but constructs tasks only from lexical and vector. Every vector input
record must be completed, effective `vector`, non-degraded, and free of a
failure code.

Both selected conditions use the same governed conversation-context wrapper.
They differ only in the frozen session ranking. Task order is deterministically
balanced per record so provider timing cannot consistently favor one condition.

W15 token-overlap results remain a historical reference. They are not part of
the W29 contemporaneous paired table and must not be described as if rerun.

## Reader And Judge

W29 preserves the qualified W15 model contract:

| Role | Provider | Model | Workers | Timeout | Attempts |
|---|---|---|---:|---:|---:|
| Reader | isolated Grok CLI | `grok-composer-2.5-fast` | 4 | 180s | 3 |
| Custom judge | isolated Grok CLI | `grok-4.5` | 4 | 120s | 3 |

Each model call is one turn with memory, web search, plans, subagents, MCP, and
all effective tools disabled. The wrapper must advertise a non-empty sentinel
allowlist and deny the sentinel plus always-on tool paths, matching the
qualified W15 isolation rule. Raw responses and authentication state remain
outside Git.

The custom judge uses the pinned upstream prompt branches but is not the
official `gpt-4o-2024-08-06` evaluator. Its result is reported as custom-judge
accuracy only.

## Execution Phases

1. Verify the dataset, retrieval JSONL, execution manifest, exact source
   revision, binary, provider wrappers, and model probes.
2. Build exactly 1,000 reader tasks from 500 records and two conditions.
3. Run the reader under a one-shot `KeepAlive=false` user LaunchAgent.
4. Run exactly one custom-judge task for every completed reader task under a
   separate one-shot LaunchAgent.
5. Finalize deterministic scores, custom-judge scores, paired outcomes,
   retrieval attribution, token accounting, latency, failures, and artifacts.
6. Prove resume makes zero provider calls and preserves checkpoint and
   normalized artifact hashes.

Foreground PTY execution and `launchctl submit` are not formal transports. A
formal run uses a versioned binary, isolated artifact root, unique run ID, and
LaunchAgent label. Failed runs are retained and never resumed into a success.

## Hard Gates

W29 is qualified only when:

1. The exact 500-record dataset and exact W28 retrieval JSONL pass their hashes.
2. Every record produces one lexical and one vector reader task.
3. Reader completed tasks are `1,000/1,000`; terminal reader failures are zero.
4. Judge completed tasks are `1,000/1,000`; terminal failures, invalid labels,
   and not-run tasks are zero.
5. All raw artifacts are non-empty, hash-matched, one-turn outputs with no tool
   call field.
6. Checkpoints retain the exact prompt, context, ranking, dataset, retrieval,
   run, implementation, model, and condition fingerprints.
7. The paired table contains exactly 500 eligible records and zero incomplete
   records with generic lexical/vector labels.
8. Retrieval classification and final-answer failures remain attributable per
   condition; no failed or difficult record is removed.
9. Resume invokes no provider and preserves every checkpoint and normalized
   result hash.
10. No credential, raw authentication state, source answer, or full model
    payload enters committed evidence.
11. The vector retrieval profile remains candidate and the product default is
    unchanged.
12. Local tests, PostgreSQL-backed CI, race, vet, repository policy, release
    manifest, and protected signing pass on the final exact head.

Reader quality is measured, not a hard-coded success threshold. Vector may
win, tie, or lose; any of those is a valid result if the execution gates pass.

## Failure Rules

- A terminal provider failure, malformed output, context mismatch, tool call,
  wrong retrieval hash, or task-count mismatch rejects the formal run.
- Retries are bounded by the frozen model configs and remain fully counted.
- A rejected run contributes no aggregate score and gets a new run ID,
  artifact root, binary identity, and LaunchAgent label on the next attempt.
- Grok unavailability is retained as a failed compatibility result; W29 is not
  silently replaced with Cursor, Codex, another model, or a mock provider.

## Decision Rule

- If vector improves contemporaneous custom-judge task success while all hard
  gates pass, the result supports further candidate-promotion evaluation but
  does not itself switch defaults.
- If retrieval improves but answer quality does not, W29 attributes the gap to
  reader aggregation, unresolved source lifecycle, or context presentation
  before proposing another retrieval algorithm.
- If vector loses, the negative result is retained and the candidate remains
  inactive.

## Non-Claims

- W29 is not official LongMemEval GPT-4o accuracy.
- W29 does not evaluate LongMemEval-M.
- W29 does not rank language models or embedding providers.
- W29 does not evaluate automatic memory formation from raw chats.
- W29 does not infer source supersession from benchmark labels.
- W29 is public benchmark evidence, not sealed or externally held evaluation.
- W29 does not complete the overall Vermory platform goal.
