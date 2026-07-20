# LongMemEval-S Domestic Direct Reader QA Design

## Purpose

W30 measures whether W28 vector-ranked governed context improves final answer
quality over the contemporaneous lexical ranking when consumed through a
direct domestic-model route. It is independent from W29: it does not replace
the unavailable Grok execution and does not rank language models.

The paired question is:

> With one frozen reader, one frozen custom judge, and the same 500 records,
> does `vermory_vector_k10` provide more useful answer context than
> `vermory_lexical_k10`?

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

W30 replays the exact W28 JSONL. It does not rerun retrieval, tune K, change
memory lifecycle state, or activate the candidate vector profile.

## Conditions

W30 contains exactly:

1. `vermory_lexical_k10`
2. `vermory_vector_k10`

Both conditions use the same governed conversation wrapper and differ only in
the frozen ranked sessions. Every vector input must be completed, effective
vector, non-degraded, and free of a failure code before any provider call.

## Reader And Judge

| Role | Route | Model | Workers | Timeout | Attempts |
|---|---|---|---:|---:|---:|
| Reader | direct SiliconFlow | `deepseek-ai/DeepSeek-V4-Flash` | 4 | 180s | 3 |
| Custom judge | direct SiliconFlow | `Qwen/Qwen3-30B-A3B-Instruct-2507` | 4 | 120s | 3 |

Both model IDs are non-Pro routes. The reader and judge were selected as
distinct available compatibility targets, not as winners. The request goes
directly to `https://api.siliconflow.cn/v1`; Mac mini NewAPI is forbidden.

The API key is read at process start from the macOS login Keychain by the
repository wrapper. It cannot appear in the execution manifest, plist,
command line, checkpoint, report, or committed evidence.

## Qualified Probe

Before freezing the dataset run, one GUI LaunchAgent executed both model
probes through the exact-head Vermory binary and Keychain wrapper:

- label `org.vermory.w30.probe-siliconflow.ffcdcfe`;
- `runs=1`, exit `0`, empty stderr;
- both models returned exactly `OK`;
- probe report SHA-256
  `ae3fae6d86edde0bf539ace706a2d8f8c6cefd38dd27c80ec439ff0e7b5885aa`;
- LaunchAgent status SHA-256
  `6f0fcf994c3ec7acecfdb6c1c2879f4329ab0dcaae26ea1e6e54396facc77f94`.

The earlier SSH probes failed before credential access and remain rejected.
They contribute no provider-success claim.

## Execution

1. Verify exact dataset, retrieval JSONL, execution manifest, source revision,
   binary, Keychain wrapper, endpoint, and probe hashes.
2. Build exactly 1,000 balanced reader tasks.
3. Run the reader under a one-shot LaunchAgent with `KeepAlive=false`.
4. Run one custom-judge task for every reader task under a separate one-shot
   LaunchAgent.
5. Finalize deterministic scores, custom-judge scores, paired outcomes,
   retrieval attribution, usage, latency, failures, and model identity.
6. Prove normal resume and a sentinel-provider resume make zero provider calls
   while preserving checkpoint and normalized artifact hashes.

Every rejected run gets a new run ID, artifact root, source snapshot, binary,
and LaunchAgent label. Foreground PTY execution and `launchctl submit` are not
formal transports.

## Hard Gates

W30 qualifies only when:

1. All frozen source and retrieval hashes match.
2. Reader tasks complete `1,000/1,000` with zero terminal failures.
3. Judge tasks complete `1,000/1,000` with zero terminal failures, invalid
   labels, or not-run tasks.
4. The paired table has 500 eligible records and zero incomplete records.
5. Raw response model identities match the frozen reader and judge IDs.
6. No response contains a tool call and no request advertises tools.
7. Checkpoints preserve prompt, context, ranking, dataset, retrieval, run,
   implementation, model, and condition fingerprints.
8. Resume performs zero provider calls and preserves exact checkpoint and
   normalized artifact hashes.
9. Credentials, source answers, full contexts, and raw authentication state do
   not enter committed evidence.
10. The vector profile remains candidate and the production default remains
    unchanged.
11. Exact-head local tests, PostgreSQL CI, race, vet, repository policy,
    release manifest, and signing pass.

Answer quality is measured rather than predetermined. Vector may win, tie, or
lose without invalidating the run when all execution gates pass.

## Non-Claims

- W30 is not official GPT-4o LongMemEval accuracy.
- W30 does not complete or substitute for W29.
- W30 does not rank DeepSeek, Qwen, Grok, Cursor, Codex, or providers.
- W30 does not evaluate LongMemEval-M or automatic memory formation.
- W30 does not activate a retrieval profile or change the product default.
- W30 is public benchmark evidence, not sealed external evaluation.
