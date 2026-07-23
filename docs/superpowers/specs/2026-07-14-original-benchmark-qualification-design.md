# Original Benchmark Qualification Design

## Purpose

Vermory already has translated local cases that exercise benchmark-inspired
capabilities. Those cases are useful regressions, but they are not executions
of an upstream benchmark. This design adds a separate evidence path for
official datasets so that source provenance, execution scope, scorer
provenance, and claim scope cannot be blurred into the existing coverage
count.

The first qualified source is the cleaned LongMemEval oracle dataset. The
first execution is intentionally a pinned sample that evaluates governed
import, retrieval, context delivery, and model consumption. It does not claim
automatic memory-formation quality or a full LongMemEval score.

## Evidence Model

Benchmark evidence is described on independent axes instead of one ordinal
"coverage" level.

### Source class

- `official_dataset`: bytes released by the benchmark owner and verified by a
  pinned revision and SHA-256 digest.
- `translated_proxy`: a Vermory-authored task that translates an upstream
  capability without using upstream records.
- `inspired_case`: a local case influenced by an upstream method.
- `design_mapping`: a capability mapping with no executable evidence.
- `unsupported`: a benchmark that is not currently relevant or runnable.

### Execution scope

- `none`: no upstream data execution exists.
- `sample`: a deterministic subset of upstream record IDs was executed.
- `full`: every record in the qualified dataset artifact was executed.

### Claim scope

- `mapping_only`: capability mapping only.
- `dataset_sample`: claims apply only to the named record IDs and conditions.
- `benchmark_wide`: claims apply to the complete qualified dataset artifact.

A `sample` execution may never use `benchmark_wide`. A full execution must
name the dataset record count and prove that the selected count equals it.

### Scorer class

- `deterministic`: code produces the same result from the same reference and
  response without a model call.
- `official_model_judge`: the upstream scorer invokes the benchmark owner's
  named judge model.
- `custom_model_judge`: a non-upstream model judge used only as auxiliary
  analysis.
- `none`: no scorer was run.

Hard factual claims require a deterministic metric. A model judge can be
reported beside it, but cannot be the only evidence for correctness,
isolation, deletion, or stale-context gates.

## Qualification Manifest

Each official source has a committed qualification manifest containing:

- benchmark name and source class;
- official repository URL and pinned revision;
- SPDX-compatible license identifier;
- dataset URL, path, revision, byte size, SHA-256, and record count;
- official scorer URL, path, revision, SHA-256, and scorer class;
- known license, contamination, annotation, and judge risks;
- a statement about whether a small fixture may be redistributed.

Qualification verifies provenance and availability. It does not imply that
Vermory executed the source.

## Execution Manifest

Each original-data run has a separate committed execution manifest containing:

- the qualification manifest path and matching dataset SHA-256;
- execution scope and claim scope;
- deterministic sampling rule and exact record IDs;
- run ID, implementation revision, provider/client configuration, and
  condition names;
- deterministic scorer names and optional auxiliary scorers;
- artifact paths, exclusions, failures, and explicit non-claims.

An execution manifest is valid only after its referenced report exists and
contains one result for every `(record_id, condition)` pair.

## First LongMemEval Slice

### Qualified source

- Repository: `https://github.com/xiaowu0162/LongMemEval`
- Repository revision: `9e0b455f4ef0e2ab8f2e582289761153549043fc`
- Dataset: `xiaowu0162/longmemeval-cleaned`
- Dataset revision: `98d7416c24c778c2fee6e6f3006e7a073259d48f`
- Dataset artifact: `longmemeval_oracle.json`
- Dataset SHA-256: `821a2034d219ab45846873dd14c14f12cfe7776e73527a483f9dac095d38620c`
- Dataset size: `15388478` bytes
- Dataset records: `500`
- License: `MIT`

The official evaluator uses GPT-4o as a yes/no answer judge. The first slice
therefore records that scorer as official provenance but uses deterministic
normalized exact match, token F1, answer-token recall, and abstention phrase
detection as primary sample metrics.

### Sampling rule

The first slice selects a fixed set of factual and abstention records from the
official oracle artifact. Record IDs are committed before provider execution.
Preference-only questions are excluded because the upstream rubric requires a
subjective judge. The report lists this exclusion and does not generalize to
the preference category.

### Conditions

- `no_context`: the reader receives only the question.
- `full_oracle_history`: the reader receives all official oracle sessions for
  the selected record.
- `plain_lexical_retrieval`: a deterministic token-overlap retriever selects
  the top sessions without Vermory governance or lifecycle semantics.
- `vermory_packet`: the same official sessions are imported as active,
  source-governed memories in an isolated conversation continuity; Vermory's
  production retrieval and context delivery produce the packet consumed by
  the same reader provider.

Every condition uses the same provider, model, question text, and output
contract. Provider compatibility is reported, not used to rank models.

### Runtime boundary

Each record receives its own tenant-scoped conversation anchor. The runner
uses a dedicated PostgreSQL database and a stable run ID. Source sessions are
stored as governed `source_update` observations, which makes the lifecycle
state explicit and reproducible. This path tests import, authority,
projection, retrieval, delivery, and model consumption. It does not test
automatic draft extraction or user confirmation quality.

Model-facing context contains only semantic session content and timestamps.
It does not include memory IDs, tenant IDs, continuity IDs, scorer labels, or
provenance/debug metadata.

## Validation Rules

The qualification loader rejects:

- unknown source, execution, claim, or scorer classes;
- an official source without repository revision, license, dataset revision,
  byte size, digest, record count, or scorer provenance;
- a digest that is not lowercase SHA-256;
- a sample execution without a sampling rule and record IDs;
- a sample execution that claims `benchmark_wide`;
- a full execution whose selected count differs from the dataset record count;
- a hard factual execution with no deterministic scorer;
- an execution whose dataset digest differs from its qualification manifest;
- an original-data execution merged into translated-proxy aggregate counts.

## Artifacts

The runner writes under `artifacts/benchmarks/<run-id>/`:

- `source.json`: verified source and selection metadata;
- `requests/<record>/<condition>.json`: semantic input and condition metadata;
- `responses/<record>/<condition>.json`: provider output and model name;
- `scores.json`: per-record deterministic metrics and condition aggregates;
- `report.md`: human-readable scope, results, failures, and non-claims;
- `execution-manifest.json`: machine-readable execution evidence.

Raw provider credentials are never written. Raw upstream data remains outside
the repository; committed fixtures contain only the selected official records
permitted by the MIT license and carry their source revision and hashes.

## Acceptance

This slice is accepted when:

- source and execution validation tests prove every rejection rule red first
  and green after implementation;
- the official oracle artifact hash matches the qualification manifest;
- the selected fixture is derived from that artifact and its record IDs match
  the frozen selection;
- all four conditions run through one real provider for every selected record,
  or failures are retained and classified per condition;
- the Vermory condition uses PostgreSQL, governed active memories, production
  retrieval, and recorded delivery rather than an in-memory substitute;
- report and scores distinguish deterministic metrics from any model judge;
- the report states `dataset_sample`, never a full LongMemEval score;
- existing translated benchmark reports continue to pass and remain separate.

