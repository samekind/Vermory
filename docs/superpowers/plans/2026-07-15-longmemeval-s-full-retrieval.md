# LongMemEval-S Full Retrieval Qualification Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Execute all 500 records from the pinned cleaned LongMemEval-S artifact through Vermory's real PostgreSQL-authoritative lexical retrieval path and publish deterministic, failure-preserving session-recall evidence.

**Architecture:** Extend benchmark manifests so a full execution can make a target-specific `qualified_dataset_full` claim without pretending to be full QA. Add a streaming LongMemEval reader and upstream-compatible session metrics, then build a resumable runner that imports each official session as governed conversation memory, queries the production `RetrievalCoordinator`, maps returned memory IDs back to official session IDs, and writes atomic per-record evidence before final aggregation.

**Tech Stack:** Go 1.25.7, PostgreSQL 18/pgx, existing Vermory runtime and artifact store, Cobra, JSON/JSONL/Markdown, official cleaned LongMemEval-S JSON.

## Global Constraints

- Source artifact must be exactly `277383467` bytes with SHA-256 `d6f21ea9d60a0d56f34a05b609c79c88a451d2ae03597821ea3d5a9678c3a442`.
- Execute exactly 500 records, 23,867 sessions, and 246,750 turns; score exactly 470 non-abstention records.
- Canonical sorted record-ID SHA-256 must be `f038965c54b03632f86a59104dd77848b66e3f80c08d5fbabdd3984d16457811`.
- W14 uses `evaluation_target=retrieval`, `execution_scope=full`, and `claim_scope=qualified_dataset_full`.
- One official record maps to one isolated conversation continuity; cross-continuity retrieval is a zero-tolerance failure.
- Production evidence must call `runtime.RetrievalCoordinator`; direct SQL or an in-memory stand-in cannot count as Vermory retrieval.
- Lexical remains the product default. W14 does not introduce vector or HNSW changes.
- Every failure remains in checkpoints and the failure ledger; do not revise records or thresholds after seeing results.
- The upstream dataset remains outside Git. Commit only manifests, code, normalized evidence, and permitted metadata.
- Never print, persist, or commit credentials. W14 requires no provider key.
- Do not use subagents for this plan unless a later task becomes genuinely independent.

---

### Task 1: Target-Specific Full-Execution Contract

**Files:**
- Modify: `internal/benchmark/manifest.go`
- Modify: `internal/benchmark/manifest_test.go`
- Modify: `casebook/benchmarks/executions/longmemeval-oracle-sample.json`
- Create: `casebook/benchmarks/qualifications/longmemeval-s-cleaned.json`
- Create: `casebook/benchmarks/executions/longmemeval-s-full-retrieval.json`

**Interfaces:**
- Produces: `benchmark.EvaluationTarget` with `EvaluationTargetQA` and `EvaluationTargetRetrieval`.
- Produces: `benchmark.ClaimScopeQualifiedDatasetFull`.
- Produces: `benchmark.SelectionModeAllRecords`.
- Extends: `benchmark.ExecutionManifest` with `EvaluationTarget`, `SelectionMode`, and `RecordSetSHA256`.

- [x] **Step 1: Write failing full-execution validation tests**

Add tests equivalent to:

```go
func TestExecutionAcceptsTargetSpecificQualifiedDatasetFull(t *testing.T) {
    manifest := validExecution()
    manifest.EvaluationTarget = EvaluationTargetRetrieval
    manifest.ExecutionScope = ExecutionScopeFull
    manifest.ClaimScope = ClaimScopeQualifiedDatasetFull
    manifest.SelectionMode = SelectionModeAllRecords
    manifest.RecordSetSHA256 = strings.Repeat("d", 64)
    manifest.SamplingRule = ""
    manifest.FixturePath = ""
    manifest.FixtureSHA256 = ""
    manifest.SelectedRecordIDs = nil
    if err := ValidateExecution(validQualification(), manifest); err != nil {
        t.Fatal(err)
    }
}

func TestExecutionRejectsFullRunWithoutRecordSetDigest(t *testing.T) {
    manifest := validFullRetrievalExecution()
    manifest.RecordSetSHA256 = ""
    if err := ValidateExecution(validQualification(), manifest); err == nil ||
        !strings.Contains(err.Error(), "record_set_sha256") {
        t.Fatalf("expected record-set rejection, got %v", err)
    }
}

func TestExecutionRejectsBenchmarkWideRetrievalClaim(t *testing.T) {
    manifest := validFullRetrievalExecution()
    manifest.ClaimScope = ClaimScopeBenchmarkWide
    if err := ValidateExecution(validQualification(), manifest); err == nil ||
        !strings.Contains(err.Error(), "qualified_dataset_full") {
        t.Fatalf("expected overclaim rejection, got %v", err)
    }
}
```

Also prove that the existing oracle sample remains valid after adding
`evaluation_target=qa`.

- [x] **Step 2: Verify RED**

Run:

```bash
go test ./internal/benchmark -run 'TestExecution' -count=1
```

Expected: FAIL because the new enum values and manifest fields do not exist.

- [x] **Step 3: Implement the contract**

Add these exact public values:

```go
type EvaluationTarget string
const (
    EvaluationTargetQA        EvaluationTarget = "qa"
    EvaluationTargetRetrieval EvaluationTarget = "retrieval"
)

const ClaimScopeQualifiedDatasetFull ClaimScope = "qualified_dataset_full"

type SelectionMode string
const SelectionModeAllRecords SelectionMode = "all_records"
```

For `sample`, retain the frozen fixture and selected-ID rules. For `full`,
require `selection_mode=all_records`, a lowercase SHA-256
`record_set_sha256`, no sample fixture, and
`claim_scope=qualified_dataset_full`. Reject `benchmark_wide` until a later
schema proves the complete upstream task and scorer contract.

- [x] **Step 4: Freeze the official manifests**

The S qualification must contain:

```json
{
  "dataset": {
    "path": "longmemeval_s_cleaned.json",
    "revision": "98d7416c24c778c2fee6e6f3006e7a073259d48f",
    "sha256": "d6f21ea9d60a0d56f34a05b609c79c88a451d2ae03597821ea3d5a9678c3a442",
    "size_bytes": 277383467,
    "record_count": 500
  },
  "official_scorer": {
    "path": "src/evaluation/print_retrieval_metrics.py",
    "sha256": "58b70c0b562ea57372a7774a554c347cd908e901b77ac0149fc90b097b6f1b8f",
    "class": "deterministic"
  }
}
```

The full execution must freeze both conditions, all-record selection, the
record-set digest, upstream-compatible deterministic scorers, and explicit
non-claims from the design.

- [x] **Step 5: Verify GREEN and commit**

Run:

```bash
go test ./internal/benchmark -run 'TestExecution|TestQualification' -count=1
jq empty casebook/benchmarks/qualifications/longmemeval-s-cleaned.json
jq empty casebook/benchmarks/executions/longmemeval-s-full-retrieval.json
git diff --check
```

Commit:

```bash
git add internal/benchmark/manifest.go internal/benchmark/manifest_test.go casebook/benchmarks
git commit -m "feat: define full retrieval benchmark evidence"
```

### Task 2: Streaming Source Verification And Official Metrics

**Files:**
- Create: `internal/benchmark/longmemeval_stream.go`
- Create: `internal/benchmark/longmemeval_stream_test.go`
- Create: `internal/benchmark/retrieval_metrics.go`
- Create: `internal/benchmark/retrieval_metrics_test.go`
- Modify: `internal/benchmark/longmemeval.go`

**Interfaces:**
- Produces: `ScanLongMemEval(path string, visit func(LongMemEvalRecord) error) (LongMemEvalSummary, error)`.
- Produces: `LongMemEvalSummary{RecordCount, SessionCount, TurnCount int; RecordSetSHA256 string}`.
- Produces: `EvaluateSessionRetrieval(ranked, relevant []string, k int) SessionRetrievalMetric`.
- Produces: `AggregateSessionRetrieval(results []SessionRetrievalResult) RetrievalAggregate`.

- [x] **Step 1: Write failing streaming-loader tests**

Cover a valid JSON array, duplicate IDs, malformed parallel arrays, trailing
JSON, visitor failure propagation, canonical digest independence from source
record order, and exact record/session/turn counts. Include a test that feeds
the same records in opposite order and requires the same sorted-ID digest.
Also prove that unlabeled empty turns are retained in the turn count but omitted
from semantic text, while an all-empty session or empty answer-labeled turn is
rejected.

- [x] **Step 2: Verify streaming RED**

Run:

```bash
go test ./internal/benchmark -run 'TestScanLongMemEval' -count=1
```

Expected: FAIL because `ScanLongMemEval` does not exist.

- [x] **Step 3: Implement streaming JSON-array decoding**

Use `json.Decoder`, require the opening `[` and closing `]`, validate every
record with the existing `record.validate`, reject duplicates, and retain only
the record IDs needed for the final sorted newline-delimited SHA-256. Do not
read the whole 277 MB file with `os.ReadFile`.

- [x] **Step 4: Write failing metric tests from upstream definitions**

Use cases that prove:

```text
relevant=[a,c], ranked=[a,b,c]
K=2 -> recall_any=1, recall_all=0
K=3 -> recall_any=1, recall_all=1
```

Also test repeated distractor IDs, no relevant IDs, first relevant rank, MRR,
and NDCG using the upstream DCG discount where rank 1 is undiscounted and later
ranks divide by `log2(rank)`. The official cleaned S artifact contains 13
records with one repeated non-answer session ID, so repeated ranked IDs must
occupy distinct positions rather than being rejected or collapsed.

- [x] **Step 5: Implement and verify metrics**

Reject empty IDs but preserve repeated ranked IDs as distinct corpus
occurrences. Deduplicate the relevant-ID set as the upstream evaluator does.
Keep abstention exclusion outside the metric function so the report still
retains those records.

Run:

```bash
go test ./internal/benchmark -run 'TestScanLongMemEval|TestEvaluateSessionRetrieval|TestAggregateSessionRetrieval' -count=1
```

- [x] **Step 6: Verify the real source metadata and commit**

Run a focused test gated by:

```text
VERMORY_LONGMEMEVAL_S_DATASET=/tmp/vermory-longmemeval-98d7416/longmemeval_s_cleaned.json
```

Require `500`, `23867`, `246750`, and the frozen record-set digest. Then commit:

```bash
git add internal/benchmark
git commit -m "feat: stream and score LongMemEval retrieval"
```

### Task 3: Resumable Production Retrieval Runner

**Files:**
- Create: `internal/app/longmemeval_retrieval.go`
- Create: `internal/app/longmemeval_retrieval_test.go`
- Create: `cmd/vermory/benchmark_longmemeval_retrieval.go`
- Modify: `cmd/vermory/main.go`
- Modify: `cmd/vermory/main_test.go`

**Interfaces:**
- Produces: `app.RunLongMemEvalRetrieval(ctx context.Context, opts LongMemEvalRetrievalOptions) (LongMemEvalRetrievalReport, error)`.
- Produces: CLI command `vermory benchmark-longmemeval-retrieval`.

- [x] **Step 1: Write failing PostgreSQL integration tests**

Use a two-record fixture with independent answer and distractor sessions. The
test must prove:

- one continuity per record;
- one active governed memory per session;
- returned memory IDs map to official session IDs from commit receipts;
- `RetrievalCoordinator` is invoked in lexical mode with limit 12;
- no retrieved ID belongs to the other continuity;
- a second `--resume` run creates zero extra memories;
- a corrupted or mismatched checkpoint is rejected rather than skipped;
- per-record checkpoint files exist before final aggregate files;
- failures appear in `failure-ledger.json`.

- [x] **Step 2: Verify runner RED**

Run:

```bash
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' \
go test -p 1 ./internal/app ./cmd/vermory -run 'TestLongMemEvalRetrieval|TestBenchmarkLongMemEvalRetrieval' -count=1
```

Expected: FAIL because the runner and command do not exist.

- [x] **Step 3: Verify complete source shape and authority**

Extend the full execution manifest with positive `expected_session_count` and
`expected_turn_count` values. Validate the streamed summary against them before
opening the runtime. Use the existing tenant/continuity-scoped
`Store.ListGovernedMemories` result to prove every returned memory is active and
belongs to the current record; do not add a broader memory-inspection API.

- [x] **Step 4: Implement record execution**

For each streamed record:

```text
resolve/create benchmark conversation continuity
-> commit every timestamped session as source_update
-> retain memory_id -> official session occurrence mapping
-> run token-overlap baseline at K=12
-> run production RetrievalCoordinator lexical query at K=12
-> verify returned authority rows are active and in the record continuity
-> compute K=5/K=10/K=12 metrics
-> atomically write the record checkpoint
```

Use stable operation IDs containing run ID, question ID, session position, and
raw session ID so repeated upstream IDs remain distinct and idempotent.
Checkpoint validation must compare run ID, implementation revision, dataset
digest, record-set digest, question ID, condition names, and imported count.

- [x] **Step 5: Implement deterministic finalization**

Sort checkpoints by official question ID, require 500 total and 470 scored,
aggregate overall and by question type, write upstream-compatible JSONL,
scores, failures, report, and final manifest, then validate the final manifest.
Classify each answerable condition result exactly as
`all_evidence_retrieved`, `partial_evidence_retrieved`,
`no_evidence_retrieved`, or `runtime_failure`.

- [x] **Step 6: Register the CLI**

Expose exactly these flags:

```text
--database-url
--source-dataset
--qualification
--execution
--artifact-root
--run-id
--implementation-revision
--resume
```

The command output must state target, scope, records, scored records, and report
URI without printing individual dataset content.

- [x] **Step 7: Verify GREEN and commit**

Run the focused PostgreSQL tests twice, once normally and once with `-race`,
then:

```bash
git add internal/app/longmemeval_retrieval.go internal/app/longmemeval_retrieval_test.go internal/benchmark/manifest.go internal/benchmark/manifest_test.go casebook/benchmarks/executions/longmemeval-s-full-retrieval.json cmd/vermory
git commit -m "feat: run full governed LongMemEval retrieval"
```

### Task 4: Real 500-Record Execution And Failure Attribution

**Files:**
- Create: `docs/evidence/2026-07-15-longmemeval-s-full-retrieval.md`
- Create: `docs/evidence/snapshots/2026-07-15-longmemeval-s-full-retrieval-scores.json`
- Create: `docs/evidence/snapshots/2026-07-15-longmemeval-s-full-retrieval-failures.json`
- Modify: `docs/evaluation-matrix.md`
- Modify: `README.md`
- Modify: `README.zh-CN.md`
- Modify: `docs/superpowers/specs/2026-07-11-vermory-hypothesis-register.md`

**Interfaces:**
- Consumes: the pinned source at `/tmp/vermory-longmemeval-98d7416/longmemeval_s_cleaned.json`.
- Produces: one full deterministic public benchmark execution and explicit attribution for the two known sample QA failures.

- [x] **Step 1: Build the exact implementation binary**

Record `git rev-parse HEAD`, build with `-trimpath`, and create a dedicated
database named for the W14 run. Do not reuse `vermory_test` or any service
database.

- [x] **Step 2: Execute the clean full run**

Run `benchmark-longmemeval-retrieval` without `--resume`. Preserve stdout,
stderr, elapsed time, database size, record count, memory count, and artifact
hashes. If execution fails, retain the attempt log and classify the defect
before changing code or rerunning.

- [x] **Step 3: Execute the idempotent resume proof**

Run the same binary, run ID, database, and artifact root with `--resume`.
Require zero new governed memories and a byte-identical normalized score and
failure digest.

- [x] **Step 4: Inspect failure attribution**

Report overall and per-question-type metrics for both conditions. For retained
sample failures `0a995998` (multi-session counting) and `6a1eabeb`
(knowledge-update conflict), record the exact retrieved official session IDs
and whether all evidence was present. Do not alter ranking or cases in W14.

- [x] **Step 5: Verify hard gates and commit evidence**

Query PostgreSQL for 500 continuities, 23,867 active memories, zero non-active
returned IDs, zero cross-continuity IDs, and zero duplicate operation effects.
Scan all evidence for credential-shaped strings. Commit only normalized
snapshots and hashes, not the 277 MB source or raw conversation content.

### Task 5: Protected Delivery

**Files:**
- Modify: `docs/superpowers/plans/2026-07-15-longmemeval-s-full-retrieval.md`
- Modify: Draft PR 1 body

- [x] **Step 1: Run all local gates**

```bash
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' go test -p 1 -count=1 ./...
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' go test -race -p 1 -count=1 ./internal/benchmark ./internal/app ./internal/runtime ./cmd/vermory ./internal/provider ./internal/reality
go vet ./...
go mod tidy
git diff --exit-code -- go.mod go.sum
go build -trimpath ./cmd/vermory
pnpm -C integrations/openclaw check
pnpm -C integrations/openclaw pack --dry-run
git diff --check
```

- [x] **Step 2: Complete checklist, commit, and push**

Require no unchecked W14 items, commit the final evidence/checklist, push
`agent/grok-cli-runtime`, and update Draft PR 1 without changing it from Draft.

- [x] **Step 3: Verify protected CI and the new artifact independently**

Download the new transport ZIP through the GitHub API, match the GitHub digest,
verify all archive checksums and exact entries, inspect all four binary build
metadata, execute Darwin arm64 `version` and
`benchmark-longmemeval-retrieval --help`, verify the synthetic merge second
parent, and require `OPEN / Draft / CLEAN / MERGEABLE / test=SUCCESS`.

- [x] **Step 4: Keep the platform goal active**

W14 completion advances retrieval evidence only. Do not mark the overall goal
complete. The next stage is full 500-record reader QA and judge execution,
followed by a genuinely external sealed evaluator rather than a readable local
directory.
