# LongMemEval Original-Dataset Sample Evidence

Date: 2026-07-14

Final implementation revision: `de20f2e8f991414dea359b3ee04065665898f725`

## Scope

This evidence executes six frozen records from the official cleaned
LongMemEval oracle dataset under four comparable reader conditions:

- no context;
- full oracle history;
- deterministic plain lexical retrieval;
- a Vermory packet produced from active source-governed memories through the
  production PostgreSQL conversation retrieval and delivery path.

The execution scope is `sample` and the claim scope is `dataset_sample`. It is
not a full LongMemEval score. It does not use LongMemEval-S or LongMemEval-M
filler sessions, does not include preference questions, and does not measure
automatic draft extraction or user-confirmation quality.

## Official Source Qualification

| Item | Value |
|---|---|
| Upstream repository | `https://github.com/xiaowu0162/LongMemEval` |
| Repository revision | `9e0b455f4ef0e2ab8f2e582289761153549043fc` |
| Dataset repository | `xiaowu0162/longmemeval-cleaned` |
| Dataset revision | `98d7416c24c778c2fee6e6f3006e7a073259d48f` |
| Artifact | `longmemeval_oracle.json` |
| Artifact bytes | `15388478` |
| Artifact records | `500` |
| Artifact SHA-256 | `821a2034d219ab45846873dd14c14f12cfe7776e73527a483f9dac095d38620c` |
| License | MIT |
| Official scorer | `src/evaluation/evaluate_qa.py` at the repository revision above |
| Official scorer SHA-256 | `ecce9c4c79dc89d99534ac17b383a5cbb5b9f0c69ee98adaf0684742e3d95251` |

The Hugging Face LFS object ID and the oracle file extracted from the upstream
Google Drive release were identical. The frozen six-record fixture has
SHA-256:

```text
bfd60ccc2f577a1f4e0596797a5143c4b6cf1169d18ace4b349c7ef87142a3f2
```

Canonical JSON generated again from the 500-record artifact for the six
selected IDs matched the committed fixture canonical hash. The selected IDs
were frozen before provider execution:

```text
6a1eabeb
0a995998
7161e7e2
e47becba
gpt4_2655b836
0862e8bf_abs
```

## Runtime

```text
Vermory revision: de20f2e8f991414dea359b3ee04065665898f725
Release binary SHA-256: 2ba87989f385b7ae1965109fc291c9dc6213f1346d023847523da6d357d5fac9
Grok CLI: 0.2.99
Reader model: grok-4.5
PostgreSQL: local dedicated database, schema 9
Run ID: longmemeval-original-sample-grok-20260714-attempt-6
```

The Grok reader ran with:

```text
--verbatim
--no-memory
--disable-web-search
--no-plan
--no-subagents
--max-turns 3
--permission-mode dontAsk
--output-format json
```

The benchmark output contract required English, the shortest sufficient
answer, and reuse of source wording or numbers where possible. This prevents
local language preferences from invalidating the deterministic English token
metrics. The request artifacts contain semantic context only. A scan found no
`tenant_id`, `continuity_id`, `memory_id`, `source_ref`, or `has_answer` fields.

## Conditions

### `no_context`

The reader received the question only.

### `full_oracle_history`

The reader received all official oracle sessions for the record, including
their timestamps and turns, in source order.

### `plain_lexical_retrieval`

A deterministic token-overlap retriever selected up to five sessions. Common
English function words were excluded from the overlap score. There was no
Vermory continuity, lifecycle, or delivery state in this condition.

### `vermory_packet`

Each record received an isolated conversation continuity. Every oracle session
was committed as an active `source_update` memory. The production conversation
service retrieved active memory, recorded a delivery, returned its semantic
context packet, and persisted the completed reader turn.

This import path deliberately does not use the benchmark answer or
`has_answer` labels to choose, update, or supersede memories.

## Deterministic Results

The metrics below are local deterministic sample metrics. They are not the
official LongMemEval GPT-4o judge accuracy.

| Condition | Completed | Failed | Exact | Mean token F1 | Mean answer recall | Abstention detected |
|---|---:|---:|---:|---:|---:|---:|
| `no_context` | 6 | 0 | 0/6 | 0.0263 | 0.0385 | 1/1 |
| `full_oracle_history` | 6 | 0 | 2/6 | 0.5497 | 0.5727 | 1/1 |
| `plain_lexical_retrieval` | 6 | 0 | 2/6 | 0.4954 | 0.5154 | 1/1 |
| `vermory_packet` | 6 | 0 | 2/6 | 0.5201 | 0.5214 | 1/1 |

Exact match is intentionally strict. The assistant-rotation and GPS answers
were semantically correct but included surrounding words, so token F1 and
answer recall carry more information than exact match for those records.

## Record-Level Findings

| Record | Capability | Result |
|---|---|---|
| `6a1eabeb` | knowledge update | All three context conditions answered `25:50`; no-context abstained. The Vermory answer also mentioned the conflicting `27:12`, showing that source sessions were retrieved but not automatically represented as an explicit supersession chain. |
| `0a995998` | multi-session counting | Reference answer is `3`. Full history and Vermory answered `2`; plain retrieval answered `1`. This is a retained task failure, not counted as success. |
| `7161e7e2` | assistant-side memory | All three context conditions recalled `8 am - 4 pm (Day Shift)`; no-context abstained. |
| `e47becba` | user fact | All three context conditions returned `Business Administration`; no-context abstained. |
| `gpt4_2655b836` | temporal reasoning | All three context conditions identified the GPS system issue after the first service; no-context abstained. |
| `0862e8bf_abs` | abstention | Every condition stated that the hamster name could not be determined; all four were detected by the deterministic abstention rule. |

The multi-session failure shows that context availability alone does not solve
task interpretation and aggregation. The knowledge-update record shows that a
correct final answer can still coexist with unresolved conflicting source
memories. These remain platform-quality work, not benchmark-harness defects.

## PostgreSQL Evidence

The final run independently returned:

```json
{
  "continuities": 6,
  "active_memories": 11,
  "deliveries": 6,
  "completed_turns": 6,
  "failed_turns": 0,
  "in_progress_turns": 0,
  "cross_continuity": 0
}
```

The active-memory count equals the total oracle-session count for the selected
records. Cross-continuity inspection searched every delivery for memory
content owned by another selected record and returned zero.

## Preserved Attempts

The ignored runtime artifact root retains all attempts:

1. `attempt-1` stopped after 7 condition artifacts because the runner passed a
   41 KB provider error directly into the 512-byte failure ledger boundary.
   The database retained one completed and one in-progress turn.
2. `attempt-2` retained 19 completed and 5 failed condition results. Grok's
   one-turn bound caused `max turns reached` on long prompts. The runner fix
   retained the full provider error in artifacts and stored only a bounded
   UTF-8-safe failure message in PostgreSQL.
3. `attempt-3` retained 23 completed and 1 failed condition result after a
   two-turn bound. A direct replay proved the remaining Vermory prompt needed
   three model calls.
4. `attempt-4` completed 24/24 with a three-turn bound, but local language
   preference caused Chinese output against English deterministic references.
5. `attempt-5` completed 24/24 after `--verbatim` and an English output
   contract. It exposed that the abstention detector missed the passive phrase
   `cannot be determined`.
6. `attempt-6` reran the complete sample after the scorer fix and is the final
   evidence referenced by the repository.

These attempts are not averaged together and are not hidden. They distinguish
provider-loop, runner-ledger, isolation, output-contract, scorer, and platform
quality failures.

## Committed Snapshots

- [scores snapshot](snapshots/2026-07-14-longmemeval-original-sample-scores.json)
- [execution manifest snapshot](snapshots/2026-07-14-longmemeval-original-sample-execution.json)

```text
normalized scores snapshot SHA-256:
57e134edeb4d90acf7ec024c58da251bfc50c2c0045493bd46f8d66f4ebeb75b

raw runtime scores SHA-256:
2be12512e3520cc38e0d425fcab07d95026711b8d41443b9f39e439cf82614d0

normalized execution snapshot SHA-256:
4f332ec093fec00a21d77fc558e39292497ee529a2cc828292a029ae34cea076

raw runtime execution manifest SHA-256:
83916ae2b556981c4b38ee03e239b412af436454616da60a698225b27c403e10
```

Committed snapshots normalize generated artifact URIs to repository-relative
paths. The normalized execution snapshot now also states
`evaluation_target=qa` explicitly for the target-specific evidence contract;
the ignored raw runtime artifacts remain byte-for-byte unchanged.

Generated request/response artifacts remain under
`artifacts/benchmarks/longmemeval-original-sample-grok-20260714-attempt-6/`
and are intentionally ignored. The dedicated PostgreSQL database was removed
after counts and hashes were captured.

## Evidence Boundary

- This is a real official-dataset sample and a real Grok reader execution.
- It proves the sample source, selection, four condition paths, governed
  PostgreSQL import, retrieval, delivery, model consumption, artifact capture,
  and isolation checks executed end to end.
- It does not prove a full LongMemEval score, full-haystack retrieval,
  preference memory, automatic formation quality, model superiority, scale,
  sealed generalization, or release readiness.
