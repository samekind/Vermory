# LongMemEval-S Domestic Vector Reader QA Rejected Run v1

Date: 2026-07-20 Asia/Shanghai

Status: rejected after the direct provider exhausted the account balance; no
paired answer-quality result is qualified

## Scope

W30 compares the frozen `vermory_lexical_k10` and
`vermory_vector_k10` contexts with one direct domestic reader and one separate
custom judge. The first formal reader execution started only after the
exact-head DeepSeek and Qwen probes passed. This record preserves the resulting
provider-account failure instead of resuming or relabeling the partial run as a
successful comparison.

## Frozen Runtime

| Field | Value |
|---|---|
| Run ID | `longmemeval-s-full-domestic-vector-reader-qa-20260720-v1` |
| Implementation revision | `7a7f7f258195a061f6d28a38318bb99628e36126` |
| Source archive SHA-256 | `f1fd889b85cf5a44bba53c81277c433c1a1d82124d59ac7e170d2db4f6f47800` |
| Binary SHA-256 | `30eeec7adbd0a4ad6755d85d5ec14e7c2aa359d1a0392ccb3e5f7b1858929db0` |
| Execution manifest SHA-256 | `d25256533de3e2fbbdc5773f280059b873ee527a5f2ad3bd360dd50df5ac3dcf` |
| Keychain wrapper SHA-256 | `7f913fe8bda5cd03e15b3e29c3684e74de2b18d66f21f9486f9c0441eefcbc98` |
| Reader LaunchAgent plist SHA-256 | `1683101cddc6aaa82616e81ef51819cf80f7465b3dcb2038e35c0bfed04c89f4` |
| Reader | direct SiliconFlow `deepseek-ai/DeepSeek-V4-Flash` |
| Custom judge | direct SiliconFlow `Qwen/Qwen3-30B-A3B-Instruct-2507` |

The formal process used the macOS login Keychain wrapper. The API key did not
enter the plist, process arguments, checkpoints, raw responses, logs, or Git.
The request route was `https://api.siliconflow.cn/v1`; it did not pass through
NewAPI.

## Qualified Preflight

The exact-head one-shot LaunchAgent probe completed both frozen models before
the dataset execution:

| Gate | Result |
|---|---|
| LaunchAgent runs | `1` |
| Exit code | `0` |
| Stderr | empty |
| DeepSeek response | exact model identity, `finish_reason=stop`, content `OK` |
| Qwen response | exact model identity, `finish_reason=stop`, content `OK` |
| Probe report SHA-256 | `13e07a25467574801a6bc87d20aa28ec5bfabdc68b8fda01bcc2e4dca5ea913b` |
| LaunchAgent status SHA-256 | `0594f0ed980ba5bb8784d3fb972279f32f892aceb12727f7e0441f09987b4d0d` |

This established model-route compatibility at preflight time. It did not
guarantee sufficient provider credit for the 1,000-task reader execution.

## Observed Reader Execution

The reader began the frozen 1,000-task workload. Once terminal failures made
the zero-failure hard gate impossible, the LaunchAgent was booted out instead
of spending further calls. In-flight tasks finished writing their terminal
checkpoints during shutdown.

| Condition | Completed | Failed | Checkpoints written |
|---|---:|---:|---:|
| `vermory_lexical_k10` | `16` | `22` | `38` |
| `vermory_vector_k10` | `16` | `23` | `39` |
| Total | `32` | `45` | `77` |

All 45 failed tasks exhausted exactly three attempts. Every one of the 135
failed attempts returned the same direct-provider response:

```text
403 Forbidden
code: 30001
message: Sorry, your account balance is insufficient
```

The final attempt of the first terminal failure started at
`2026-07-20T05:03:07.307102Z`. The last successful response had started at
`2026-07-20T05:02:57.899829Z`, before that attempt. No timeout, rate limit,
model-busy response, parser failure, wrong model identity, tool call, or
Vermory lifecycle failure was observed in the retained checkpoints.

All 32 completed tasks used the exact reader model, returned
`finish_reason=stop`, and preserved raw-response hashes. Their accounted usage
was:

| Usage | Tokens |
|---|---:|
| Input | `884,248` |
| Output | `13,850` |
| Reasoning subset | `13,689` |
| Total | `898,098` |

The custom judge was not started. Deterministic per-task fields remain in the
local checkpoints for diagnosis, but the partial tasks contribute no aggregate
score, paired outcome, or answer-quality claim.

## Integrity And Shutdown

| Check | Result |
|---|---|
| Checkpoint files | `77` |
| Raw reader responses | `32` |
| Checkpoint tree SHA-256 | `d91e4f39d151554fe3ba6625c5da322d63a8a8e42fe24bf73f53773c68457081` |
| Raw-response tree SHA-256 | `0139bd753866cc1eb5d0e3d702c227a7505777b154f1f04f0c22f4493fa07944` |
| Reader stdout / stderr | both empty, SHA-256 `e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855` |
| Credential-shape scan | `0` matching files |
| Matching process after shutdown | `0` |
| LaunchAgent loaded after shutdown | no |

The retained local artifact root is immutable for qualification purposes. It
must not be resumed into a successful run or used as the base for a judge run.

## Decision

- Reject v1 because the frozen hard gate requires `1,000/1,000` reader tasks
  with zero terminal failures.
- Classify the interruption as `provider_account_balance_exhausted`, not as a
  retrieval-quality, model-quality, parser, concurrency, or retry-policy result.
- Do not lower concurrency or add retry backoff in response to this failure;
  neither addresses insufficient account balance.
- Preserve the HTTP status as a typed provider error so a non-retryable 403
  does not consume all outer attempts. Freeze a one-terminal-failure execution
  budget for later W30 runs so an already-disqualified run stops scheduling
  new work automatically. This bounds waste but does not cure the account
  state or change v1.
- After the direct SiliconFlow account has enough credit, start a fresh run ID,
  artifact root, exact source snapshot, binary identity, and LaunchAgent label.
- Preserve W29 separately. A successful W30 run would not replace the Grok
  execution required by W29.

## Non-Claims

- The 32 completed tasks do not show whether lexical or vector context is more
  useful because only a non-random prefix completed and no judge ran.
- This is not a DeepSeek or SiliconFlow quality score.
- It does not invalidate the qualified W28 retrieval result.
- It does not activate the candidate vector profile or change the production
  default.
- It does not complete the overall Vermory platform qualification.
