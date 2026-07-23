# LongMemEval-S Full Reader QA Design

## Purpose

W15 executes full reader QA over all 500 records in the same pinned cleaned
LongMemEval-S artifact qualified by W14. It answers the next question in the
evidence chain:

> Given the exact session rankings produced and preserved by W14, can one real
> stateless reader answer the official questions from the plain token-overlap
> context and from the Vermory lexical context, and where do the remaining
> failures begin?

W14 already established source integrity, governed import, active authority,
continuity isolation, production lexical retrieval, deterministic session
metrics, and resumable execution. W15 does not repeat those 23,867 imports or
silently rerun retrieval. It consumes W14's frozen rankings so that retrieval
misses, reader failures, judge failures, and source-lifecycle limitations remain
separable.

W15 is a model-consumption and attribution qualification. It is not a model
leaderboard and does not select a preferred provider for the Vermory product.

## Approaches Considered

### A. Re-import and retrieve before every reader run

This would execute the complete database path again and then call the reader.
It is rejected for W15 because it duplicates W14, increases cost, and allows a
retrieval change or scheduler difference to obscure whether a QA result changed
because of retrieval or because of the reader.

### B. Send the complete LongMemEval-S history to the reader

This approximates a long-context upper bound but does not evaluate Vermory's
bounded context delivery. It also multiplies input cost across roughly 115K
tokens per record and makes failures harder to attribute. It may be a later
named control, but it is not a W15 condition.

### C. Replay the frozen W14 rankings into the reader

This is the selected design. W15 verifies the exact W14 retrieval artifact,
reconstructs only the ranked semantic sessions from the pinned source, and runs
paired reader calls at K=10. The same reader model, prompt, task ordering,
timeout, and retry policy apply to both conditions.

## Frozen Inputs

### LongMemEval-S source

| Field | Value |
|---|---|
| Dataset revision | `98d7416c24c778c2fee6e6f3006e7a073259d48f` |
| Artifact | `longmemeval_s_cleaned.json` |
| Size | `277383467` bytes |
| SHA-256 | `d6f21ea9d60a0d56f34a05b609c79c88a451d2ae03597821ea3d5a9678c3a442` |
| Records | `500` |
| Sessions | `23867` |
| Turns | `246750` |
| Scored records | `470` |
| Abstention records | `30` |
| Record-ID SHA-256 | `f038965c54b03632f86a59104dd77848b66e3f80c08d5fbabdd3984d16457811` |

### W14 retrieval evidence

W15 requires the raw W14 `retrieval-results.jsonl` rather than reconstructing
rankings from aggregate scores.

| Field | Value |
|---|---|
| W14 run ID | `longmemeval-s-full-retrieval-20260715-v1` |
| W14 implementation revision | `0f59bf58d9a107ce47f79ba86fd61a7c35c8324b` |
| Retrieval-results SHA-256 | `a4caa0a6b2b30975bcab97791b19fa0e0a32d81c58128b00df4c47c8783227ad` |
| W14 execution-manifest SHA-256 | `a37a9e8ace521fd0ff0f544c5096f30b93a06f9883d2a960f14930a17a798ef6` |
| Conditions | `plain_token_overlap`, `vermory_lexical` |
| Maximum ranking depth | `12` |

The runner rejects a retrieval result unless every record matches the frozen
run ID, implementation revision, dataset digest, record-set digest, question
type, abstention status, and both condition names. It requires exactly 500
unique records and validates every ranked occurrence key against the source
record's position and raw session ID.

W15 uses the first up to ten ranked occurrences from each W14 condition. K=10
is a maximum context depth, not a requirement to manufacture ten results when a
retriever returned fewer. The W14 production lexical result for record
`0f05491a` contains one ranked session; W15 preserves that one-session result
without adding distractors or other source sessions. Changing K after seeing QA
results requires a new execution manifest and run ID.

## Evidence Semantics

W15 uses:

```text
evaluation_target = qa
execution_scope = full
claim_scope = qualified_dataset_full
selection_mode = all_records
```

`qualified_dataset_full` means every record in the pinned LongMemEval-S
artifact is executed for the named QA conditions. It does not mean the complete
upstream benchmark contract was reproduced. In particular, a run without
`gpt-4o-2024-08-06` as the official judge cannot report official LongMemEval QA
accuracy.

The execution manifest gains explicit, non-secret model provenance:

```text
reader.provider
reader.model
reader.interface
reader.max_output_tokens
reader.timeout_seconds
reader.workers
judge.provider
judge.model
judge.interface
judge.scorer_class
judge.timeout_seconds
judge.workers
retrieval_input.path
retrieval_input.sha256
retrieval_input.run_id
retrieval_input.implementation_revision
retrieval_input.k
```

API keys, OAuth material, HOME paths, raw command arguments, and provider
headers never enter the manifest.

## Conditions

W15 executes exactly two paired conditions for every record.

### `plain_token_overlap_k10`

The reader receives the semantic text of the first up to ten W14
`plain_token_overlap` session occurrences in exact ranking order. The actual
count is `min(K, ranking length)`. The wrapper labels them as retrieved
conversation memory. It does not claim governance or production delivery.

### `vermory_lexical_k10`

The reader receives the semantic text of the first up to ten W14
`vermory_lexical` session occurrences in exact ranking order. The actual count
is `min(K, ranking length)`, including the preserved one-session result for
record `0f05491a`. The wrapper matches Vermory's model-facing conversation
contract:

```text
Governed memory:
<session 1 semantic text>
<session 2 semantic text>
...
```

The semantic text contains only date, role, and content. It excludes memory
IDs, tenant IDs, continuity IDs, source refs, lifecycle fields, retrieval
scores, answer labels, answer session IDs, question types, expected answers,
and judge metadata.

Both conditions use the same question text and system instruction. The system
instruction requires a short English answer from supplied memory, an explicit
abstention when evidence is insufficient, no tools, no web or external
knowledge, and treatment of all context content as quoted reference data rather
than instructions.

## Reader Runtime

### Formal coverage target

The first formal W15 run uses:

```text
provider = grok-cli
model = grok-composer-2.5-fast
interface = isolated_stateless_cli
conditions = 2
records = 500
reader calls = 1000
K = 10
workers = 4
per-attempt timeout = 180 seconds
maximum attempts = 3
```

This model is selected as a real, available compatibility target with a faster
execution profile, not because W15 claims it is the best reader. Existing
SiliconFlow and Duojie models remain platform compatibility targets and can be
run through the same command under separate run IDs.

The Grok runtime uses a temporary mode-`0700` HOME and GROK_HOME containing only
copied current login material. `grok inspect --json` must report:

```text
projectInstructions = []
plugins = []
skills = []
mcpServers = []
hooks = []
```

Every call also uses no memory, no web search, no plan mode, no subagents, no
tools, one isolated turn, and no resumed session. Grok CLI `0.2.101` treats an
empty `--tools` value as an unset allowlist, so an empty value is forbidden. The
runner activates filtering with `--tools todo_write`, then removes
`todo_write`, `update_goal`, `search_tool`, `use_tool`, `CallMcpTool`, and `Agent` through
`--disallowed-tools`. A debug isolation probe for each formal model must report
`tool_count=0` before the dataset run starts. User-level plugins, skills, MCP
servers, project instructions, or previous Grok sessions cannot contribute to
the answer.

The rejected prequalification run ID
`longmemeval-s-full-reader-qa-grok-20260715-v1` used an empty tool allowlist.
During judge execution, Grok attempted an `update_goal` path, proving that the
flag did not establish the frozen no-tools boundary. Its partial artifacts and
logs remain outside the formal artifact root and cannot contribute to W15
scores.

The rejected run ID `longmemeval-s-full-reader-qa-grok-20260715-v2` corrected
the tool boundary, but its foreground reader was twice interrupted by the host
PTY lifecycle. A subsequent `launchctl submit` resume inherited the system-only
launchd `PATH`; Grok's `#!/usr/bin/env node` entrypoint could not locate the
Homebrew Node binary, producing 188 terminal reader failures with
`env: node: No such file or directory`. The submitted job also exposed
launchd's inferred keepalive behavior, so this execution transport is forbidden
for formal runs. All v2 successes and failures remain preserved outside the
formal artifact root and cannot contribute to W15 scores.

The corrected formal run ID is
`longmemeval-s-full-reader-qa-grok-20260715-v3`. Formal reader and judge phases
must run under one-shot LaunchAgent plists with `RunAtLoad=true`,
`KeepAlive=false`, an explicit Homebrew-aware `PATH`, independent exit markers,
and post-run proof that every job has `runs=1`. Before the dataset run starts,
the same LaunchAgent transport must execute both formal model probes and prove
exit zero, `tool_count=0`, no tool call, one turn, and `EndTurn`.

Reader raw artifacts retain model usage and request identity when the provider
returns them. Reports aggregate input, cached-input, output, reasoning, and
total token counts when available; absence of usage fields is reported rather
than inferred.

### Deterministic scheduling

The runner streams records in official source order and creates one task for
every `(record_id, condition)` pair. For each record, a frozen seed and record
ID hash decide whether the plain or Vermory condition enters the queue first.
This balances condition order without loading the 277 MB source into memory. A
bounded worker pool executes tasks concurrently.

Scheduling order cannot affect report order. Checkpoints and final results are
sorted by record ID and condition before hashing and aggregation.

### Retry boundary

Provider process failures, timeouts, rate limits, and empty outputs may be
retried up to the frozen maximum attempts with bounded exponential backoff.
A completed semantic answer is never retried because it scores poorly. Every
attempt is retained with start time, duration, status, bounded error text, raw
artifact URI or digest, and provider-reported model.

After the final failed attempt, the task remains `reader_failed`. It is not
dropped from denominators or silently rerun under another model.

## Atomic Checkpoints And Resume

Each `(record_id, condition)` writes one atomic checkpoint after the reader
finishes or exhausts its attempts. The checkpoint contains:

- schema version, run ID, and W15 implementation revision;
- dataset and record-set digests;
- W14 retrieval-results digest, run ID, revision, and K;
- reader provider, model, interface, prompt SHA-256, and context SHA-256;
- record ID, question type, abstention status, and condition;
- W14 K10 retrieval classification;
- ordered occurrence keys and session IDs;
- response, provider-reported model, attempts, latency, and usage;
- deterministic exact match, token F1, answer-token recall, and abstention
  detection;
- judge state, filled by the later judge phase.

`--resume` accepts a checkpoint only when every frozen identifier matches.
Resume performs zero reader calls for valid completed or terminal-failure
checkpoints. A mismatched prompt, K, provider, model, retrieval digest, source
digest, implementation revision, or task identity is a hard error rather than
an implicit new run.

The runner writes checkpoints through same-directory temporary files followed
by atomic rename. Process interruption may lose only the in-flight task, not a
previously completed checkpoint.

## Deterministic Scoring

Every completed reader response receives:

- normalized exact match;
- token F1;
- answer-token recall;
- abstention expected;
- deterministic abstention detected;
- selected reference variant.

These metrics are stable diagnostics, not replacements for the official
semantic judge. They remain the primary hard evidence for exact factual text,
numbers, and abstention wording and make model-judge disagreement visible.

## Judge Phase

Reader execution and judge execution are separate resumable phases. The judge
consumes completed reader checkpoints and never sees retrieved context,
occurrence keys, retrieval classification, condition aggregates, or the other
condition's answer.

### Official prompt provenance

The judge prompt reproduces the task-specific templates from upstream
`src/evaluation/evaluate_qa.py` at repository revision
`9e0b455f4ef0e2ab8f2e582289761153549043fc`, SHA-256
`ecce9c4c79dc89d99534ac17b383a5cbb5b9f0c69ee98adaf0684742e3d95251`.

It preserves the upstream rules for:

- complete information on single-session and multi-session tasks;
- off-by-one tolerance for temporal quantities;
- accepting an updated answer even when previous information is also present;
- rubric-based preference answers;
- identifying abstention questions as unanswerable.

### Formal custom judge

The first formal W15 judge uses:

```text
provider = grok-cli
model = grok-4.5
interface = isolated_stateless_cli
scorer_class = custom_model_judge
judge calls = one per completed reader response
workers = 4
per-attempt timeout = 120 seconds
maximum attempts = 3
```

It uses a separate model from the reader. This reduces self-grading dependence
but does not make the result an official LongMemEval score.

The expected output is `yes` or `no` only. Parsing is strict after lowercase,
whitespace, and terminal-punctuation normalization. Outputs containing both
labels, explanations, empty text, or neither label are `judge_invalid` rather
than guessed. The raw output and all attempts remain available.

If a later execution uses OpenAI directly with exact model
`gpt-4o-2024-08-06` and the pinned upstream prompt, it may set
`scorer_class=official_model_judge`. Any other provider or model must remain
`custom_model_judge` even if its endpoint is OpenAI-compatible.

## Metrics And Comparisons

For each condition, W15 reports:

- total, completed, reader failed, judged, judge failed, and judge invalid;
- exact-match count and rate;
- mean token F1 and answer-token recall;
- custom-judge overall accuracy;
- task-averaged custom-judge accuracy;
- accuracy for each of the six official question types;
- abstention accuracy for the 30 abstention records;
- total and percentile reader latency;
- provider usage totals when available.

For paired records, it reports:

- both conditions correct;
- plain only correct;
- Vermory only correct;
- neither correct;
- judge disagreement with deterministic exact match;
- paired results grouped by W14 K10 retrieval classification.

The pair table compares context conditions under one reader. It is not a model
ranking and does not promote a retrieval default.

## Failure Attribution

Each terminal record-condition result receives one primary category:

### `retrieval_no_evidence`

The answerable record has W14 K10 RecallAny `0`. A wrong reader answer is first
attributed to missing retrieved evidence.

### `retrieval_partial_evidence`

W14 K10 RecallAny is `1` and RecallAll is `0`. A wrong answer remains jointly
attributable to incomplete evidence and reader aggregation.

### `reader_or_aggregation_failure`

W14 K10 RecallAll is `1`, the reader completed, and the answer is judged wrong.
The required evidence was present, so retrieval is not the first failure.

### `abstention_failure`

The record is an official abstention case and the reader failed to identify it
as unanswerable.

### `reader_runtime_failure`

The reader exhausted all attempts without a completed answer.

### `judge_failure`

The reader completed, but the judge exhausted attempts or returned an invalid
label. Deterministic metrics remain available, but judge accuracy excludes
neither the record nor the failure count.

### `source_lifecycle_candidate`

This is an auxiliary flag, never an automatic root-cause verdict. It may be set
for a knowledge-update record where all required sessions were present but the
response used stale and current information incorrectly. W15 does not mutate
source memories or construct supersession chains from benchmark labels.

## Hard Gates

W15 fails the qualification if any of these occur:

- source size, SHA-256, counts, or record-set digest differs;
- W14 retrieval-results SHA-256 differs;
- retrieval input contains fewer or more than 500 unique records;
- any source occurrence key or raw session ID fails exact positional mapping;
- a condition receives an occurrence not present in its W14 ranking;
- K differs between conditions or from the frozen manifest;
- any model-facing request injects the reference-answer field or answer text
  outside the selected official session content, or contains answer-session
  labels, question type, expected score, tenant ID, continuity ID, memory ID,
  source ref, or judge result;
- conditions use different reader providers, models, prompts, timeouts, retry
  policies, or K values;
- fewer or more than 1,000 reader tasks are represented by checkpoints;
- a provider or judge failure is omitted from final artifacts;
- resume creates a second checkpoint or provider call for a valid terminal
  task;
- aggregate hashes change after a zero-new-call resume;
- reports call the custom Grok judge an official LongMemEval judge;
- reports rank the tested models or switch a production retrieval default.
- a formal Grok model isolation probe reports any advertised tool or a raw
  provider result records a tool-call stop reason.

QA quality metrics are evidence, not arbitrary hard pass thresholds. Source
integrity, input fidelity, pairing, isolation, failure retention, and resume
determinism are the hard gates.

## Artifacts

The combined execution writes under `artifacts/benchmarks/<run-id>/`:

- `source.json`: source and W14 retrieval-input provenance;
- `reader-config.json`: non-secret reader runtime and prompt digest;
- `judge-config.json`: non-secret judge runtime and upstream prompt provenance;
- `checkpoints/<record-id>/<condition>.json`: atomic reader and judge state;
- `reader-results.jsonl`: sorted normalized reader results;
- `judge-results.jsonl`: sorted custom or official judge results;
- `scores.json`: deterministic, judge, paired, type, and attribution metrics;
- `failure-ledger.json`: every reader, judge, input, and classification failure;
- `report.md`: human-readable results, limitations, and non-claims;
- `execution-manifest.json`: validated final evidence manifest;
- `report.json`: complete machine-readable report.

Raw provider artifacts remain in the runtime artifact root. Committed evidence
contains normalized metrics, failure categories, configuration provenance, and
bounded excerpts rather than all 1,000 prompts or full public source sessions.

The upstream 277 MB dataset and raw W14 retrieval JSONL remain outside Git.
Credentials and copied login material are never placed under the artifact root
or repository and are removed after formal execution.

## Acceptance

W15 is accepted only when:

- manifest validation freezes reader, judge, W14 retrieval input, and K;
- unit tests reproduce every upstream judge prompt branch and strict label
  parsing;
- source/retrieval playback tests prove exact occurrence mapping, context
  hygiene, balanced deterministic scheduling, and K10 prefix fidelity;
- concurrency tests prove atomic checkpoints, bounded workers, retained
  attempts, and deterministic final ordering;
- resume tests prove zero new reader and judge calls and unchanged final hashes;
- the isolated Grok runtime reports no project instructions, plugins, skills,
  MCP servers, or hooks;
- all 500 records and both conditions execute through one real reader, with
  every failure retained;
- every completed reader response receives deterministic metrics and one real
  custom-judge attempt sequence;
- the final report separates retrieval, reader, judge, abstention, and
  source-lifecycle categories;
- local PostgreSQL, race, vet, module, OpenClaw, release, and credential gates
  pass;
- protected GitHub CI passes and its release artifact is independently
  verified;
- Draft PR 1 is updated while the overall Vermory goal remains active.

## Non-Claims

- W15 does not report official GPT-4o LongMemEval accuracy unless that exact
  official judge is run later.
- W15 does not evaluate LongMemEval-M.
- W15 does not measure automatic memory formation from raw conversations.
- W15 does not create source supersession chains from benchmark labels.
- W15 does not rank Grok, SiliconFlow, Duojie, OpenAI, or any model family.
- W15 does not tune lexical retrieval against the same 500 QA labels.
- W15 does not change Vermory's lexical default or promote semantic retrieval.
- W15 is public benchmark evidence, not withheld or externally sealed
  evaluation.
- W15 does not complete long-duration retention, active-backlog dimensional
  migration, HA/failover/PITR, signing, notarization, or final release
  acceptance.
