# Original Benchmark Qualification Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Qualify an official benchmark source, execute a pinned LongMemEval oracle sample through comparable baselines and Vermory's production conversation path, and publish evidence that cannot be mistaken for a full benchmark score.

**Architecture:** Keep the existing translated benchmark map unchanged as a capability registry. Add a focused `internal/benchmark` package for qualification/execution manifests, LongMemEval loading, deterministic selection/scoring, and a runner orchestrated from `internal/app`. The real `vermory_packet` condition uses the existing PostgreSQL runtime, governed source updates, production retrieval, delivery recording, and the same provider used by all baseline conditions.

**Tech Stack:** Go 1.24, PostgreSQL/pgx, existing Vermory provider interface, Cobra CLI, JSON/Markdown artifacts, upstream LongMemEval JSON.

## Global Constraints

- Use the official dataset bytes pinned by SHA-256; do not silently substitute LongMemEval-V2.
- Keep original-data results separate from translated proxies and design mappings.
- A sampled run must use `claim_scope=dataset_sample` and must not publish a benchmark-wide score.
- Hard factual metrics require deterministic scoring; model judges are auxiliary only.
- Model-facing packets contain semantic content only, never engineering metadata.
- Use a dedicated PostgreSQL database for the real run and remove no pre-existing database or service.
- Preserve provider failures as evidence; do not rerun them away without retaining the failed attempt.
- Never print, persist, or commit credentials.
- Follow test-first red/green cycles for production behavior.

---

### Task 1: Qualification And Execution Contracts

**Files:**
- Create: `internal/benchmark/manifest.go`
- Create: `internal/benchmark/manifest_test.go`
- Create: `casebook/benchmarks/qualifications/longmemeval-cleaned-oracle.json`
- Modify: `internal/app/benchmark_coverage.go`
- Modify: `internal/app/benchmark_coverage_test.go`

**Interfaces:**
- Produces: `benchmark.LoadQualification(path string) (Qualification, error)`
- Produces: `benchmark.ValidateExecution(qualification Qualification, execution ExecutionManifest) error`
- Produces: independent official-source and translated-proxy counts in the coverage artifact.

- [x] **Step 1: Write failing manifest validation tests**

Add table-driven tests that reject missing source revision/license/dataset hash/scorer provenance, invalid SHA-256, sampled benchmark-wide claims, full runs with incomplete counts, factual runs without deterministic scorers, and dataset-digest mismatch.

- [x] **Step 2: Verify RED**

Run:

```bash
go test ./internal/benchmark ./internal/app -run 'Test(Qualification|Execution|BenchmarkCoverage)' -count=1
```

Expected: FAIL because the manifest package and separate evidence counters do not exist.

- [x] **Step 3: Implement minimal contracts**

Implement typed enums, JSON loaders, normalization, validation, and coverage reporting that never adds original executions to translated proxy counts.

- [x] **Step 4: Verify GREEN**

Run the same focused test command and require exit 0.

- [x] **Step 5: Commit**

```bash
git add internal/benchmark internal/app/benchmark_coverage.go internal/app/benchmark_coverage_test.go casebook/benchmarks/qualifications/longmemeval-cleaned-oracle.json docs/superpowers/specs/2026-07-14-original-benchmark-qualification-design.md docs/superpowers/plans/2026-07-14-original-benchmark-qualification.md
git commit -m "feat: qualify official benchmark evidence"
```

### Task 2: LongMemEval Loader, Frozen Sample, And Deterministic Scoring

**Files:**
- Create: `internal/benchmark/longmemeval.go`
- Create: `internal/benchmark/longmemeval_test.go`
- Create: `casebook/benchmarks/fixtures/longmemeval-oracle-sample.json`
- Create: `casebook/benchmarks/executions/longmemeval-oracle-sample.json`

**Interfaces:**
- Produces: `benchmark.LoadLongMemEval(path string) ([]LongMemEvalRecord, error)`
- Produces: `benchmark.SelectRecords(records []LongMemEvalRecord, ids []string) ([]LongMemEvalRecord, error)`
- Produces: `benchmark.RetrieveSessions(record LongMemEvalRecord, limit int) []LongMemEvalSession`
- Produces: `benchmark.ScoreAnswer(record LongMemEvalRecord, response string) DeterministicScore`

- [x] **Step 1: Write failing loader and scorer tests**

Cover duplicate/missing IDs, malformed parallel session arrays, stable frozen selection order, lexical retrieval determinism, normalized exact match, token F1, answer-token recall, and abstention detection.

- [x] **Step 2: Verify RED**

Run:

```bash
go test ./internal/benchmark -run 'TestLongMemEval|TestSelect|TestRetrieve|TestScore' -count=1
```

Expected: FAIL because the loader and scorer do not exist.

- [x] **Step 3: Implement minimal loader and scorer**

Use Go standard-library JSON parsing and Unicode-aware token normalization. Keep scores numeric and retain raw responses; do not convert the custom deterministic metrics into an official LongMemEval accuracy.

- [x] **Step 4: Derive and verify the fixture**

Extract only the frozen record IDs from the verified `821a2034...` oracle artifact. Verify fixture IDs, source digest metadata, and a committed fixture SHA-256 recorded in the execution manifest.

- [x] **Step 5: Verify GREEN and commit**

Run the focused tests, then:

```bash
git add internal/benchmark casebook/benchmarks/fixtures casebook/benchmarks/executions
git commit -m "feat: freeze LongMemEval sample scoring"
```

### Task 3: Comparable Baseline And Vermory Runner

**Files:**
- Create: `internal/app/longmemeval_benchmark.go`
- Create: `internal/app/longmemeval_benchmark_test.go`
- Modify: `cmd/vermory/main.go`
- Modify: `cmd/vermory/main_test.go`

**Interfaces:**
- Produces: `app.RunLongMemEvalSample(ctx context.Context, opts LongMemEvalOptions) (LongMemEvalReport, error)`
- Produces: CLI command `vermory benchmark-longmemeval`.

- [x] **Step 1: Write failing orchestration tests**

Use a real test PostgreSQL database and a deterministic provider double to prove four conditions per record, isolated conversation continuities, source-governed active memories, recorded deliveries, semantic-only packets, stable artifacts, and retained provider failures.

- [x] **Step 2: Verify RED**

Run:

```bash
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' go test -p 1 ./internal/app ./cmd/vermory -run 'TestLongMemEval|TestBenchmarkLongMemEval' -count=1
```

Expected: FAIL because the runner and command do not exist.

- [x] **Step 3: Implement four conditions**

Build `no_context`, `full_oracle_history`, and `plain_lexical_retrieval` directly through the shared provider interface. For `vermory_packet`, resolve a record-specific conversation, commit each official oracle session as an active `source_update`, and call the production conversation service so retrieval, delivery, and answer persistence use PostgreSQL.

- [x] **Step 4: Implement artifacts and execution validation**

Write source metadata, semantic requests, provider responses, deterministic scores, Markdown report, and execution manifest. Validate the final execution manifest before returning success.

- [x] **Step 5: Verify GREEN and commit**

Run focused tests and the CLI help test, then:

```bash
git add internal/app/longmemeval_benchmark.go internal/app/longmemeval_benchmark_test.go cmd/vermory/main.go cmd/vermory/main_test.go
git commit -m "feat: run governed LongMemEval sample"
```

### Task 4: Real Grok Execution, Evidence, And Release Integration

**Files:**
- Create: `docs/evidence/2026-07-14-longmemeval-original-sample.md`
- Create: `docs/evidence/snapshots/2026-07-14-longmemeval-original-sample-scores.json`
- Modify: `docs/evaluation-matrix.md`
- Modify: `README.md`
- Modify: Draft PR 1 body

**Interfaces:**
- Consumes: `vermory benchmark-longmemeval` and the frozen official sample.
- Produces: reproducible real-provider evidence and an explicit remaining-boundary list.

- [x] **Step 1: Build a release binary and prepare a dedicated database**

Use a new dedicated database. Apply embedded migrations through the release binary and keep the benchmark database isolated from existing services.

- [x] **Step 2: Execute the real Grok slice**

Run every frozen record under all four conditions with the locally authenticated Grok CLI. Preserve all failed attempts and retry records without deleting the original failure artifacts.

- [x] **Step 3: Verify database and artifact invariants**

Query counts for continuities, active source memories, deliveries, completed/failed turns, and cross-record isolation. Verify execution-manifest and snapshot hashes.

- [x] **Step 4: Run release verification**

```bash
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' go test -p 1 -count=1 ./...
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' go test -race -p 1 -count=1 ./internal/benchmark ./internal/app ./internal/runtime ./internal/provider ./cmd/vermory
go vet ./...
go mod tidy
git diff --exit-code -- go.mod go.sum
go build -trimpath -o /tmp/vermory-benchmark-release ./cmd/vermory
git diff --check
```

- [x] **Step 5: Document, commit, push, and update Draft PR**

Record the exact source revisions, hashes, record IDs, conditions, deterministic metrics, failures, database evidence, and non-claims. Keep PR 1 in Draft state and retain the overall platform goal as active.
