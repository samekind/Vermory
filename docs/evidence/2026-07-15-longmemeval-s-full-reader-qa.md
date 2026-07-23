# LongMemEval-S Full Reader QA Evidence

Date: 2026-07-15 through 2026-07-16 Asia/Shanghai

Status: qualified public full-dataset reader QA execution with a custom Grok
judge; external sealed evaluation remains open

## Question

W14 established retrieval fidelity over every record in the pinned cleaned
LongMemEval-S artifact. W15 asks the downstream question: when a real isolated
reader consumes those frozen rankings, do the retrieved sessions support the
correct answer?

This run replays the same W14 K=10 rankings under two conditions:

- `plain_token_overlap_k10`;
- `vermory_lexical_k10`.

It does not rerun retrieval, tune K, change the lexical product default, or use
the QA labels to rewrite source lifecycle state.

## Frozen Inputs

| Field | Value |
|---|---|
| Benchmark | `LongMemEval`, S cleaned variant |
| Dataset revision | `98d7416c24c778c2fee6e6f3006e7a073259d48f` |
| Dataset bytes | `277,383,467` |
| Dataset SHA-256 | `d6f21ea9d60a0d56f34a05b609c79c88a451d2ae03597821ea3d5a9678c3a442` |
| Records / scored / abstention | `500 / 470 / 30` |
| Sessions / turns | `23,867 / 246,750` |
| Record-set SHA-256 | `f038965c54b03632f86a59104dd77848b66e3f80c08d5fbabdd3984d16457811` |
| W14 retrieval run | `longmemeval-s-full-retrieval-20260715-v1` |
| W14 retrieval revision | `0f59bf58d9a107ce47f79ba86fd61a7c35c8324b` |
| W14 retrieval JSONL SHA-256 | `a4caa0a6b2b30975bcab97791b19fa0e0a32d81c58128b00df4c47c8783227ad` |
| Upstream QA scorer revision | `9e0b455f4ef0e2ab8f2e582289761153549043fc` |
| Upstream QA scorer SHA-256 | `ecce9c4c79dc89d99534ac17b383a5cbb5b9f0c69ee98adaf0684742e3d95251` |

The 500 source records produce 1,000 reader tasks: one task per condition for
each official record. The existing W14 record with one Vermory result remains a
one-result ranking; the runner does not pad it with invented distractors.

## Rejected Runs

### v1: the empty tool allowlist was not an allowlist

`longmemeval-s-full-reader-qa-grok-20260715-v1` used `--tools ""`. Grok CLI
`0.2.101` treated that value as unset. During judge execution, record
`c14c00dd` attempted the `update_goal` path and reached `max turns reached`.
The run was stopped immediately. Its 1,000 reader responses and partial judge
state cannot contribute to formal scores.

The rejected v1 artifacts remain at
`/tmp/vermory-w15-ca778f0-rejected-v1-artifacts`. The retained reader and judge
log hashes are `fe8ee6a3...201fb` and `e4272d99...d0aec`.

### v2: the execution transport was not stable

`longmemeval-s-full-reader-qa-grok-20260715-v2` corrected the tool boundary.
Two foreground reader processes were then interrupted by the host PTY
lifecycle, first after 626 checkpoints and then after 745. A subsequent
`launchctl submit` resume inherited the system-only launchd `PATH`; Grok's
`#!/usr/bin/env node` entrypoint could not find the Homebrew Node binary. The
resume produced 188 terminal failures, each retaining three attempts with
`env: node: No such file or directory`.

The submitted job also demonstrated inferred keepalive behavior, so
`launchctl submit` is not an acceptable one-shot benchmark transport. The run
was stopped with 745 completed tasks, 188 failed tasks, and 67 tasks not run.
All v2 artifacts remain at
`/tmp/vermory-w15-ad283d8-rejected-v2-artifacts` and cannot contribute to v3.

These failures are retained because they established two formal boundaries:
the provider must advertise zero tools, and the outer benchmark phase must
execute exactly once under a transport with an explicit runtime environment.

## Qualified v3 Execution

| Field | Value |
|---|---|
| Run ID | `longmemeval-s-full-reader-qa-grok-20260715-v3` |
| Implementation revision | `d10a53c307d86cb5005ab63b2b73aa9bace2f346` |
| Binary SHA-256 | `2645feb4d1d58644b5761eacdd2bc40e14ae1f6c97b77f6b29f6786a92f0ee3e` |
| Binary | CGO-free, `-trimpath`, Darwin arm64 |
| Go | `go1.26.5` |
| Grok CLI | `0.2.101 (5bc4b5dfadcf)` |
| Reader | `grok-composer-2.5-fast`, four workers |
| Judge | `grok-4.5`, four workers, custom upstream-prompt judge |

The isolated wrapper used mode-`0700` HOME/GROK_HOME directories, mode-`0600`
`auth.json` and `agent_id`, and no project instructions, hooks, skills,
plugins, MCP servers, remote settings, or enabled Cursor/Claude/Codex
compatibility discovery. The redaction-safe inspection summary has SHA-256
`a2dd1ea6...eb6f`.

Every provider call uses:

```text
--verbatim
--no-memory
--disable-web-search
--no-plan
--no-subagents
--max-turns 1
--tools todo_write
--disallowed-tools todo_write,update_goal,search_tool,use_tool,CallMcpTool,Agent
--permission-mode dontAsk
--output-format json
```

The non-empty sentinel activates Grok's allowlist behavior, and the denylist
then removes the sentinel and the always-on tool paths observed across the two
formal agents.

## One-Shot Transport Qualification

Reader, judge, and both model probes ran under LaunchAgent plists with:

```text
RunAtLoad = true
KeepAlive = false
PATH = /opt/homebrew/bin:/usr/bin:/bin:/usr/sbin:/sbin
```

Both v3 probes completed with `runs=1`, exit code zero, `text=yes`, one turn,
`EndTurn`, `tool_count=0`, and `response.has_tool_call=false`.

| Artifact | SHA-256 |
|---|---|
| Composer probe debug | `03e01956...9cd9` |
| Judge probe debug | `c50e5b61...9f6b` |
| Composer probe output | `49c9f4a2...5ee` |
| Judge probe output | `1d279cc3...778` |
| Reader LaunchAgent plist | `02963a37...e35` |
| Judge LaunchAgent plist | `df980069...377` |

The plists and debug logs remain outside Git. Only the redaction-safe facts and
hashes are committed.

## Reader Runtime

| Metric | Value |
|---|---:|
| Tasks / completed / failed | `1,000 / 1,000 / 0` |
| Provider calls | `1,000` |
| One-attempt tasks | `1,000` |
| Plain / Vermory tasks | `500 / 500` |
| Wall time | `2,262.99 s` |
| Maximum RSS | `281,362,432 bytes` |
| Input / cached input tokens | `28,890,723 / 1,860,679` |
| Output / reasoning tokens | `548,072 / 0` |
| Total tokens | `31,299,474` |

All 1,000 raw reader artifacts are non-empty
`grok-composer-2.5-fast` results with `EndTurn` and `num_turns=1`. Structured
JSON paths contain no tool, goal, search, MCP, or agent call field. Every raw
SHA-256 and byte count matches its checkpoint. The reader log SHA-256 is
`12b5e86a...4831`.

## Judge Runtime

| Metric | Value |
|---|---:|
| Tasks / judged / terminal failures / terminal invalid | `1,000 / 1,000 / 0 / 0` |
| Provider calls | `1,002` |
| One-attempt / two-attempt tasks | `998 / 2` |
| Wall time | `2,858.47 s` |
| Maximum RSS | `293,502,976 bytes` |
| Usage-bearing attempts | `1,001` |
| Input / cached input tokens | `344,765 / 1,089,536` |
| Output / reasoning tokens | `161,172 / 160,169` |
| Total tokens | `1,595,473` |

The two retained first-attempt failures are:

- one `context deadline exceeded` after the fixed 120-second timeout;
- one strict parser rejection of `**yes**`.

Both tasks completed on the second attempt. The Markdown-wrapped label was not
silently accepted. All 1,001 raw judge artifacts are one-turn
`grok-4.5-build` `EndTurn` results, expose no tool-call field, and match their
recorded SHA-256 and byte counts. The judge log SHA-256 is
`d0ed0fbf...c7f`.

## Aggregate QA Results

| Condition | Correct | Accuracy | Exact | Mean token F1 | Mean answer recall | Abstention |
|---|---:|---:|---:|---:|---:|---:|
| `plain_token_overlap_k10` | `379 / 500` | `0.7580` | `252` | `0.5906` | `0.7060` | `29 / 30` |
| `vermory_lexical_k10` | `341 / 500` | `0.6820` | `226` | `0.5328` | `0.6557` | `29 / 30` |

Paired outcomes over all 500 records:

| Both correct | Plain only | Vermory only | Neither | Incomplete |
|---:|---:|---:|---:|---:|
| `310` | `69` | `31` | `90` | `0` |

The result retains the regression. On this public English long-history corpus,
the current Vermory lexical ranking does not outperform the same-text token
overlap baseline. W15 does not change the production default, tune on these
labels, or re-run the benchmark for a cleaner result.

## Results By Question Type

| Question type | Records | Plain accuracy | Vermory accuracy |
|---|---:|---:|---:|
| `knowledge-update` | `78` | `0.8974` | `0.8077` |
| `multi-session` | `133` | `0.6767` | `0.5789` |
| `single-session-assistant` | `56` | `0.9643` | `0.7500` |
| `single-session-preference` | `30` | `0.4667` | `0.5333` |
| `single-session-user` | `70` | `0.9714` | `0.9000` |
| `temporal-reasoning` | `133` | `0.6241` | `0.6015` |

Preference is the only question type where Vermory wins this comparison.
Multi-session and assistant-side questions retain the largest deficits.

## Retrieval Attribution

| Condition | All evidence | Partial evidence | No evidence | Abstention |
|---|---:|---:|---:|---:|
| Plain token overlap | `394` | `52` | `24` | `30` |
| Vermory lexical | `345` | `79` | `46` | `30` |

The 280 failure-ledger entries classify as:

| Primary category | Plain | Vermory |
|---|---:|---:|
| `retrieval_no_evidence` | `24` | `45` |
| `retrieval_partial_evidence` | `41` | `60` |
| `reader_or_aggregation_failure` | `55` | `53` |
| `abstention_failure` | `1` | `1` |

There are zero reader runtime, terminal judge, or aggregation runtime failures.
Sixteen entries are marked as auxiliary source-lifecycle candidates: five
plain and eleven Vermory. The benchmark labels do not mutate governed source
state.

## Deterministic Finalization And Resume

Finalization produced 500 records, 1,000 normalized reader results, 1,000
normalized judge results, 280 failure entries, and completed in 4.53 seconds.

| Artifact | SHA-256 |
|---|---|
| `reader-results.jsonl` | `10273922...318b` |
| `judge-results.jsonl` | `e8cb0154...5ff` |
| `scores.json` | `b727c1ab...b89` |
| `failure-ledger.json` | `09589e99...d46` |
| `execution-manifest.json` | `9f810fbf...32d` |
| `report.json` | `2613cef5...555` |
| `report.md` | `798e814b...ab8` |

The initial `--phase all --resume` reported 1,000 reader checkpoints resumed
and 1,000 judge checkpoints resumed. It made no provider calls and retained all
1,000 checkpoint bytes plus the four normalized hashes above.

A second proof replaced both provider commands with a sentinel executable that
would append a counter and exit 99 on any call. The resume still succeeded, no
counter file was created, and the checkpoint and normalized hash lists remained
byte-identical. The sentinel SHA-256 is `892b2ca4...55b`; the normal and sentinel
resume logs have hashes `bc699a8e...891` and `3db6b348...b3b`.

## Evidence Files

- [normalized scores](snapshots/2026-07-15-longmemeval-s-full-reader-qa-scores.json)
- [normalized failure ledger](snapshots/2026-07-15-longmemeval-s-full-reader-qa-failures.json)
- [normalized execution manifest](snapshots/2026-07-15-longmemeval-s-full-reader-qa-execution.json)

The 277 MB upstream source, W14 raw retrieval JSONL, checkpoints, raw model
responses, authentication material, provider session state, LaunchAgent
plists, and debug logs remain outside Git.

## Decision

- Accept v3 as the first qualified full LongMemEval-S reader QA execution.
- Keep the current lexical product default unchanged; W15 is diagnosis, not a
  silent retrieval cutover.
- Treat multi-session, assistant-side, and no/partial-evidence results as
  measured retrieval work, not a model-selection exercise.
- Retain the timeout and strict-label retry as execution evidence.
- Use these public failures to freeze a later retrieval improvement against a
  separate holdout or external sealed evaluator instead of tuning and
  re-scoring the same 500 labels.

## Non-Claims

- This is not official GPT-4o LongMemEval judge accuracy.
- The tested Grok models are compatibility targets, not a model ranking or a
  product default selection.
- LongMemEval-M was not executed.
- Frozen session import and QA do not prove automatic memory formation from raw
  chat streams.
- The benchmark does not create source supersession chains from labels.
- The run does not select an embedding model, promote semantic retrieval, or
  change K.
- This public execution is not withheld or externally sealed evidence.
- W15 does not complete the overall Vermory platform goal.
