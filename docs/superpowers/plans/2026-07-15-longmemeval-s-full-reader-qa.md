# LongMemEval-S Full Reader QA Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Execute paired K10 reader QA and a separate real custom judge over all 500 pinned LongMemEval-S records using the exact W14 retrieval rankings, with atomic resume, failure attribution, deterministic metrics, and protected delivery evidence.

**Architecture:** Extend benchmark evidence with non-secret reader, judge, and frozen-retrieval provenance. Stream the 277 MB source, validate the 1.2 MB W14 ranking JSONL, reconstruct bounded semantic contexts, execute reader and judge phases through bounded worker pools, and persist one atomic checkpoint per record-condition pair. Aggregate only from checkpoints so resume is zero-call and report hashes are deterministic.

**Tech Stack:** Go 1.25.7+, PostgreSQL 18 integration gates, existing provider interface, isolated Grok CLI 0.2.101, Cobra, JSON/JSONL/Markdown, official cleaned LongMemEval-S JSON, GoReleaser 2.17.0, pnpm 11/OpenClaw.

## Global Constraints

- Source must be exactly `277383467` bytes with SHA-256 `d6f21ea9d60a0d56f34a05b609c79c88a451d2ae03597821ea3d5a9678c3a442`.
- Source summary must remain 500 records, 23,867 sessions, 246,750 turns, 470 scored records, 30 abstention records, and record-set SHA-256 `f038965c54b03632f86a59104dd77848b66e3f80c08d5fbabdd3984d16457811`.
- W14 retrieval input must have SHA-256 `a4caa0a6b2b30975bcab97791b19fa0e0a32d81c58128b00df4c47c8783227ad`, run ID `longmemeval-s-full-retrieval-20260715-v1`, and implementation revision `0f59bf58d9a107ce47f79ba86fd61a7c35c8324b`.
- W15 uses `evaluation_target=qa`, `execution_scope=full`, `claim_scope=qualified_dataset_full`, `selection_mode=all_records`, and K=10.
- Conditions are exactly `plain_token_overlap_k10` and `vermory_lexical_k10`.
- The same reader provider, model, prompt, K, timeout, attempts, and scheduling contract apply to both conditions.
- Formal reader is isolated `grok-cli/grok-composer-2.5-fast`; formal judge is isolated `grok-cli/grok-4.5` with scorer class `custom_model_judge`.
- Formal concurrency is four reader workers and four judge workers. Retries cannot exceed three attempts per task.
- A poor semantic answer is never retried. Only process/runtime/empty-output failures may consume later attempts.
- Reader context contains only selected official session date, role, and content plus the condition wrapper. It cannot inject reference answers or evaluator metadata outside those sessions.
- Every failed attempt and terminal failure remains in checkpoints and the failure ledger.
- The first formal run does not tune retrieval, change K, change the lexical default, or rank models.
- Upstream source, W14 raw JSONL, credentials, and temporary Grok login material remain outside Git.
- Do not use subagents unless a later task becomes genuinely independent.

---

### Task 1: Full QA Evidence Contract

**Files:**
- Create: `internal/benchmark/longmemeval_qa_manifest.go`
- Create: `internal/benchmark/longmemeval_qa_manifest_test.go`
- Modify: `internal/benchmark/manifest.go`
- Modify: `internal/benchmark/manifest_test.go`
- Create: `casebook/benchmarks/qualifications/longmemeval-s-cleaned-qa.json`
- Create: `casebook/benchmarks/executions/longmemeval-s-full-reader-qa.json`

**Interfaces:**
- Produces: `benchmark.ExecutionModelConfig`.
- Produces: `benchmark.RetrievalExecutionInput`.
- Extends: `benchmark.ExecutionManifest` with `Reader`, `Judge`, and `RetrievalInput`.
- Produces: `benchmark.ValidateLongMemEvalQAExecution(Qualification, ExecutionManifest) error`.

- [x] **Step 1: Write failing model-provenance and retrieval-input tests**

Add tests with these required shapes:

```go
func TestValidateLongMemEvalQAExecutionAcceptsFrozenFullRun(t *testing.T) {
    qualification := validLongMemEvalQAQualification()
    manifest := validLongMemEvalQAExecution()
    if err := ValidateLongMemEvalQAExecution(qualification, manifest); err != nil {
        t.Fatal(err)
    }
}

func TestValidateLongMemEvalQAExecutionRejectsMissingReader(t *testing.T) {
    manifest := validLongMemEvalQAExecution()
    manifest.Reader = nil
    err := ValidateLongMemEvalQAExecution(validLongMemEvalQAQualification(), manifest)
    if err == nil || !strings.Contains(err.Error(), "reader") {
        t.Fatalf("expected reader rejection, got %v", err)
    }
}

func TestValidateLongMemEvalQAExecutionRejectsWrongK(t *testing.T) {
    manifest := validLongMemEvalQAExecution()
    manifest.RetrievalInput.K = 12
    err := ValidateLongMemEvalQAExecution(validLongMemEvalQAQualification(), manifest)
    if err == nil || !strings.Contains(err.Error(), "K=10") {
        t.Fatalf("expected K rejection, got %v", err)
    }
}
```

Also reject empty provider/model/interface, non-positive limits, invalid scorer
class, missing retrieval SHA/run/revision, a reader scorer class, an official
judge class whose model is not exactly `gpt-4o-2024-08-06`, condition drift,
and any JSON fields named `api_key`, `token`, `authorization`, or `secret`.

- [x] **Step 2: Verify RED**

```bash
go test ./internal/benchmark -run 'TestValidateLongMemEvalQAExecution|TestExecutionModel' -count=1
```

Expected: FAIL because the types and validator do not exist.

- [x] **Step 3: Implement typed non-secret provenance**

Add:

```go
type ExecutionModelConfig struct {
    Provider        string      `json:"provider"`
    Model           string      `json:"model"`
    Interface       string      `json:"interface"`
    ScorerClass     ScorerClass `json:"scorer_class,omitempty"`
    MaxOutputTokens int         `json:"max_output_tokens"`
    TimeoutSeconds  int         `json:"timeout_seconds"`
    Workers         int         `json:"workers"`
    MaxAttempts     int         `json:"max_attempts"`
}

type RetrievalExecutionInput struct {
    Path                   string `json:"path"`
    SHA256                 string `json:"sha256"`
    RunID                  string `json:"run_id"`
    ImplementationRevision string `json:"implementation_revision"`
    K                      int    `json:"k"`
}
```

`ValidateLongMemEvalQAExecution` calls `ValidateExecution`, requires the frozen
full-QA fields, validates both model configs, requires K=10 and the two exact
condition names, and keeps official/custom judge provenance distinct.

- [x] **Step 4: Freeze the QA qualification and execution manifests**

The QA qualification uses the same dataset bytes but pins:

```json
{
  "official_scorer": {
    "path": "src/evaluation/evaluate_qa.py",
    "revision": "9e0b455f4ef0e2ab8f2e582289761153549043fc",
    "sha256": "ecce9c4c79dc89d99534ac17b383a5cbb5b9f0c69ee98adaf0684742e3d95251",
    "class": "official_model_judge"
  }
}
```

The execution manifest freezes the Global Constraints values, reader
`grok-composer-2.5-fast`, judge `grok-4.5`, worker counts, timeouts, attempts,
K10, deterministic metrics, custom judge, and explicit non-claims.

- [x] **Step 5: Verify GREEN and commit**

```bash
go test ./internal/benchmark -run 'TestValidateLongMemEvalQAExecution|TestExecution' -count=1
jq empty casebook/benchmarks/qualifications/longmemeval-s-cleaned-qa.json
jq empty casebook/benchmarks/executions/longmemeval-s-full-reader-qa.json
git diff --check
```

Commit:

```bash
git add internal/benchmark casebook/benchmarks
git commit -m "feat: define full reader QA evidence"
```

### Task 2: Provider Usage And Stateless Grok Boundary

**Files:**
- Modify: `internal/provider/provider.go`
- Modify: `internal/provider/grok_cli.go`
- Modify: `internal/provider/grok_cli_test.go`
- Modify: `internal/provider/openai_compatible.go`
- Modify: `internal/provider/provider_test.go`

**Interfaces:**
- Produces: `provider.TokenUsage`.
- Extends: `provider.GenerateResponse` with `Usage *TokenUsage`.
- Changes: Grok invocation to `--max-turns 1` and a verified zero-tool
  allowlist/denylist boundary.

- [x] **Step 1: Write failing normalized-usage tests**

Test Grok raw JSON containing:

```json
{
  "usage": {
    "input_tokens": 2718,
    "cache_read_input_tokens": 7285,
    "output_tokens": 97,
    "reasoning_tokens": 0,
    "total_tokens": 10100
  }
}
```

and OpenAI-compatible raw JSON containing prompt/completion totals plus cached
and reasoning detail. Require exact normalized fields. Add an argument-capture
test requiring `--max-turns 1`, `--no-memory`, `--disable-web-search`,
`--no-plan`, `--no-subagents`, `--tools todo_write`, and
`--disallowed-tools todo_write,update_goal,search_tool,use_tool,CallMcpTool,Agent`.

- [x] **Step 2: Verify RED**

```bash
go test ./internal/provider -run 'TestGrokCLI.*Usage|TestGrokCLI.*Arguments|TestOpenAICompatible.*Usage' -count=1
```

Expected: FAIL because normalized usage is absent and Grok still allows three
turns.

- [x] **Step 3: Implement usage normalization and the one-turn boundary**

Add:

```go
type TokenUsage struct {
    InputTokens       int `json:"input_tokens"`
    CachedInputTokens int `json:"cached_input_tokens"`
    OutputTokens      int `json:"output_tokens"`
    ReasoningTokens   int `json:"reasoning_tokens"`
    TotalTokens       int `json:"total_tokens"`
}
```

Populate it only when provider usage is present. Do not infer missing values.
Keep raw artifacts unchanged. Change only Grok's stateless execution arguments;
do not add login material or environment values to request artifacts.

- [x] **Step 4: Run provider tests and commit**

```bash
go test ./internal/provider -count=1
go test -race ./internal/provider -count=1
git diff --check
```

Commit:

```bash
git add internal/provider
git commit -m "feat: record provider usage for QA runs"
```

### Task 3: W14 Ranking Playback And Context Construction

**Files:**
- Create: `internal/app/longmemeval_qa_types.go`
- Create: `internal/app/longmemeval_qa_playback.go`
- Create: `internal/app/longmemeval_qa_playback_test.go`

**Interfaces:**
- Produces: `app.LongMemEvalQATask`.
- Produces: `app.LoadLongMemEvalQARetrieval(path string, execution benchmark.ExecutionManifest) (map[string]LongMemEvalRetrievalRecordResult, error)`.
- Produces: `app.BuildLongMemEvalQATasks(record benchmark.LongMemEvalRecord, retrieval LongMemEvalRetrievalRecordResult, k int) ([]LongMemEvalQATask, error)`.

- [x] **Step 1: Write failing playback-validation tests**

Use a two-record fixture and JSONL. Prove exact SHA/run/revision/dataset and
record-set match; exactly one retrieval row per record; exact two conditions;
occurrence position/raw-ID mapping; and duplicate distractor IDs at different
positions. Reject unknown occurrence, wrong ID, duplicate record, missing
condition, and K below ten.

Require both tasks to contain the same question/system prompt and the first up
to ten selected sessions in W14 order, with the actual count equal to
`min(K, ranking length)`. Preserve the one-session W14 production lexical result
for record `0f05491a` without filler. Include no `has_answer`, IDs, scorer
metadata, or injected reference-answer field. Permit answer text naturally
present inside selected source sessions.

- [x] **Step 2: Verify RED**

```bash
go test ./internal/app -run 'TestLoadLongMemEvalQARetrieval|TestBuildLongMemEvalQATasks' -count=1
```

- [x] **Step 3: Implement streaming JSONL validation and bounded context**

Decode W14 JSONL line by line and retain only 500 small ranking records. Require
at least one result per condition, but do not require a retriever to fill K.
Map up to K occurrence positions directly to source sessions. Plain context uses
`Retrieved conversation memory:`. Vermory context uses
`runtime.BuildConversationContext(nil, memories, nil)` for byte-compatible
production wrapping. Hash the system prompt and final context with SHA-256.

Task order within each record is selected by SHA-256 parity of
`"longmemeval-w15-v1:" + record.QuestionID`.

- [x] **Step 4: Verify playback GREEN and commit**

```bash
go test ./internal/app -run 'TestLoadLongMemEvalQARetrieval|TestBuildLongMemEvalQATasks' -count=1
VERMORY_LONGMEMEVAL_S_DATASET=/tmp/vermory-longmemeval-98d7416/longmemeval_s_cleaned.json \
  VERMORY_LONGMEMEVAL_W14_RESULTS=/tmp/vermory-w14-0f59bf5-artifacts/benchmarks/longmemeval-s-full-retrieval-20260715-v1/retrieval-results.jsonl \
  go test ./internal/app -run TestLongMemEvalQARealPlaybackMetadata -count=1
git diff --check
```

Commit:

```bash
git add internal/app/longmemeval_qa_types.go internal/app/longmemeval_qa_playback.go internal/app/longmemeval_qa_playback_test.go
git commit -m "feat: replay frozen LongMemEval rankings"
```

### Task 4: Atomic Reader Worker And Resume

**Files:**
- Create: `internal/app/longmemeval_qa_checkpoint.go`
- Create: `internal/app/longmemeval_qa_checkpoint_test.go`
- Create: `internal/app/longmemeval_qa_reader.go`
- Create: `internal/app/longmemeval_qa_reader_test.go`

**Interfaces:**
- Produces: `app.LongMemEvalQAOptions`.
- Produces: `app.RunLongMemEvalQAReader(ctx context.Context, opts LongMemEvalQAOptions) (LongMemEvalQAReaderSummary, error)`.
- Produces: atomic `longmemeval-qa-checkpoint/v1` files.

- [x] **Step 1: Write failing checkpoint contract tests**

Define checkpoint validation around these identifiers:

```go
type LongMemEvalQACheckpoint struct {
    SchemaVersion           string
    RunID                   string
    ImplementationRevision string
    DatasetSHA256           string
    RecordSetSHA256         string
    RetrievalSHA256         string
    RetrievalRunID          string
    RetrievalRevision       string
    K                       int
    Reader                  benchmark.ExecutionModelConfig
    PromptSHA256            string
    ContextSHA256           string
    RecordID                string
    QuestionType            string
    Abstention              bool
    Condition               string
    RetrievalClassification string
    RankedOccurrenceKeys    []string
    RankedSessionIDs        []string
    ReaderStatus            string
    Response                string
    Attempts                []LongMemEvalQAAttempt
    Score                   *benchmark.DeterministicScore
    Judge                   *LongMemEvalQAJudgeState
}
```

Prove same-directory temp plus rename, no partial JSON after injected write
failure, strict mismatch rejection, and valid terminal checkpoints accepted.

- [x] **Step 2: Write failing worker-pool tests**

Use a blocking provider that records active calls. Require:

```text
100 tasks produce 100 checkpoints
maximum active calls never exceeds configured workers
poor completed answers are called once
planned runtime failure retries exactly max_attempts
all attempts remain ordered and bounded
resume makes zero provider calls
resume does not change checkpoint bytes
condition-order parity is deterministic
```

- [x] **Step 3: Verify RED**

```bash
go test ./internal/app -run 'TestLongMemEvalQACheckpoint|TestRunLongMemEvalQAReader' -count=1
```

- [x] **Step 4: Implement reader execution**

Stream the source through `ScanLongMemEval`. For each record, build its two
tasks and send them to a channel bounded by `2*workers`. A fixed worker pool
checks resume, executes attempts with `context.WithTimeout`, records normalized
usage, computes `benchmark.ScoreAnswer`, and writes the terminal checkpoint.

Retry delays are `1s`, then `2s`; tests inject a no-wait sleeper. Error text is
bounded to 1000 bytes. A completed non-empty response is terminal regardless of
score. Return an error only for source/input/checkpoint contract failures;
provider task failures remain evidence.

- [x] **Step 5: Run reader tests and commit**

```bash
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' go test -p 1 ./internal/app -count=1
go test -race ./internal/app -run 'TestRunLongMemEvalQAReader|TestLongMemEvalQACheckpoint' -count=1
git diff --check
```

Commit:

```bash
git add internal/app/longmemeval_qa_checkpoint.go internal/app/longmemeval_qa_checkpoint_test.go internal/app/longmemeval_qa_reader.go internal/app/longmemeval_qa_reader_test.go
git commit -m "feat: run resumable LongMemEval readers"
```

### Task 5: Upstream-Prompt Judge Worker

**Files:**
- Create: `internal/benchmark/longmemeval_judge.go`
- Create: `internal/benchmark/longmemeval_judge_test.go`
- Create: `internal/app/longmemeval_qa_judge.go`
- Create: `internal/app/longmemeval_qa_judge_test.go`

**Interfaces:**
- Produces: `benchmark.LongMemEvalJudgePrompt(record LongMemEvalRecord, response string) (string, error)`.
- Produces: `benchmark.ParseLongMemEvalJudgeLabel(output string) (bool, error)`.
- Produces: `app.RunLongMemEvalQAJudge(ctx context.Context, opts LongMemEvalQAOptions) (LongMemEvalQAJudgeSummary, error)`.

- [x] **Step 1: Freeze exact upstream prompt branches in tests**

Add golden expected strings for:

```text
single-session-user
single-session-assistant
multi-session
temporal-reasoning
knowledge-update
single-session-preference
any *_abs record
```

The wording must match pinned `evaluate_qa.py` SHA-256
`ecce9c4c79dc89d99534ac17b383a5cbb5b9f0c69ee98adaf0684742e3d95251`.
Test strict parsing of `yes`, `Yes.`, `NO`, invalid explanations, both labels,
empty output, and neither label.

- [x] **Step 2: Verify judge RED**

```bash
go test ./internal/benchmark -run 'TestLongMemEvalJudgePrompt|TestParseLongMemEvalJudgeLabel' -count=1
go test ./internal/app -run TestRunLongMemEvalQAJudge -count=1
```

- [x] **Step 3: Implement prompt and strict label parsing**

Port only the prompt templates and task dispatch from the pinned Python source.
Do not port its OpenAI client or substring-`yes` parser. Normalize terminal
punctuation and require one unambiguous label.

- [x] **Step 4: Implement resumable judge workers**

Load every reader checkpoint, skip terminal reader failures with explicit judge
state `not_run_reader_failed`, and queue completed responses. The judge request
contains only question, reference answer/rubric, response, and upstream prompt.
It never receives retrieval context or condition metadata.

Use the reader's bounded-attempt and atomic-update contract. A valid label is
terminal. Exhausted or invalid output becomes `judge_failed` or
`judge_invalid` and remains in evidence. Resume makes zero judge calls for
terminal judge states.

- [x] **Step 5: Verify judge GREEN and commit**

```bash
go test ./internal/benchmark -run 'TestLongMemEvalJudgePrompt|TestParseLongMemEvalJudgeLabel' -count=1
go test ./internal/app -run TestRunLongMemEvalQAJudge -count=1
go test -race ./internal/app -run TestRunLongMemEvalQAJudge -count=1
git diff --check
```

Commit:

```bash
git add internal/benchmark/longmemeval_judge.go internal/benchmark/longmemeval_judge_test.go internal/app/longmemeval_qa_judge.go internal/app/longmemeval_qa_judge_test.go
git commit -m "feat: judge LongMemEval reader responses"
```

### Task 6: Deterministic Aggregation And Attribution

**Files:**
- Create: `internal/app/longmemeval_qa_report.go`
- Create: `internal/app/longmemeval_qa_report_test.go`
- Modify: `internal/app/benchmark_coverage.go`
- Modify: `internal/app/benchmark_coverage_test.go`

**Interfaces:**
- Produces: `app.FinalizeLongMemEvalQA(opts LongMemEvalQAOptions) (LongMemEvalQAReport, error)`.
- Produces: sorted reader/judge JSONL, scores, failure ledger, report, and final execution manifest.

- [x] **Step 1: Write failing aggregate tests**

Use a fixture containing all terminal states. Assert condition totals,
completion/failure counts, deterministic means, overall and task-averaged judge
accuracy, six type aggregates, abstention accuracy, the paired outcome table,
usage totals, latency percentiles, K10 retrieval-class groups, and primary
failure attribution. `source_lifecycle_candidate` remains an auxiliary flag.

Require report bytes to remain identical across shuffled checkpoint load order.
Finalization fails unless all 1,000 checkpoints exist.

- [x] **Step 2: Verify RED**

```bash
go test ./internal/app -run 'TestFinalizeLongMemEvalQA|TestAggregateLongMemEvalQA' -count=1
```

- [x] **Step 3: Implement finalization**

Sort checkpoints by record ID then condition and write artifacts atomically.
The failure ledger includes every reader/judge terminal failure and every
judged incorrect answer with retrieval attribution. Aggregate artifacts exclude
full context and reference answers.

Validate the final execution manifest through
`benchmark.ValidateLongMemEvalQAExecution` after artifact URIs are present.
Benchmark coverage accepts the full QA execution without requiring the sample
fixture or raw W14 JSONL inside Git.

- [x] **Step 4: Verify deterministic hashes and commit**

```bash
go test ./internal/app -run 'TestFinalizeLongMemEvalQA|TestAggregateLongMemEvalQA|TestBenchmarkCoverage' -count=1
git diff --check
```

Commit:

```bash
git add internal/app/longmemeval_qa_report.go internal/app/longmemeval_qa_report_test.go internal/app/benchmark_coverage.go internal/app/benchmark_coverage_test.go
git commit -m "feat: report full LongMemEval reader QA"
```

### Task 7: CLI And End-To-End Miniature

**Files:**
- Create: `cmd/vermory/benchmark_longmemeval_qa.go`
- Modify: `cmd/vermory/main.go`
- Modify: `cmd/vermory/main_test.go`
- Create: `cmd/vermory/benchmark_longmemeval_qa_test.go`

**Interfaces:**
- Produces: `vermory benchmark-longmemeval-qa`.
- Supports phases: `reader`, `judge`, `all`, `finalize`.

- [x] **Step 1: Write failing CLI tests**

Require flags:

```text
--source-dataset
--retrieval-results
--qualification
--execution
--artifact-root
--run-id
--implementation-revision
--phase
--reader-command
--reader-base-url
--reader-api-key-env
--judge-command
--judge-base-url
--judge-api-key-env
--resume
```

Reject unknown phases and positional arguments. Provider/model/worker/timeout/K
come from the execution manifest and are not mutable CLI flags.

- [x] **Step 2: Verify RED**

```bash
go test ./cmd/vermory -run 'TestBenchmarkLongMemEvalQA|TestRootCommand' -count=1
```

- [x] **Step 3: Implement CLI and a two-record all-phase integration test**

The command loads the manifest, builds reader and judge providers from its
provider names plus runtime endpoint/command flags, executes requested phases,
and prints one bounded summary line without answers or credentials.

The integration test uses two provider overrides, two conditions, concurrency,
reader failure, judge invalid output, resume, and finalization. It proves the
second run makes zero provider calls and final hashes do not change.

- [x] **Step 4: Run focused and full tests, then commit**

```bash
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' go test -p 1 -count=1 ./cmd/vermory ./internal/app ./internal/benchmark ./internal/provider
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' go test -race -p 1 -count=1 ./cmd/vermory ./internal/app ./internal/benchmark ./internal/provider
git diff --check
```

Commit:

```bash
git add cmd/vermory
git commit -m "feat: add full LongMemEval QA command"
```

### Task 8: Formal 500-Record Grok Execution

**Files:**
- Create after execution: `docs/evidence/2026-07-15-longmemeval-s-full-reader-qa.md`
- Create after execution: `docs/evidence/snapshots/2026-07-15-longmemeval-s-full-reader-qa-scores.json`
- Create after execution: `docs/evidence/snapshots/2026-07-15-longmemeval-s-full-reader-qa-failures.json`
- Create after execution: `docs/evidence/snapshots/2026-07-15-longmemeval-s-full-reader-qa-execution.json`

**Interfaces:**
- Consumes: isolated current Grok login material outside Git, pinned source, pinned W14 raw retrieval JSONL, and the W15 release binary.
- Produces: full reader and custom-judge runtime artifacts plus normalized committed evidence.

- [x] **Step 1: Build the exact release binary**

```bash
full_head_sha="$(git rev-parse HEAD)"
short_sha="$(git rev-parse --short=7 HEAD)"
CGO_ENABLED=0 go build -trimpath \
  -ldflags "-X vermory/internal/brand.Revision=$full_head_sha" \
  -o "/tmp/vermory-w15-$short_sha" ./cmd/vermory
```

Record `go version -m`, binary SHA-256, full implementation revision, OS,
architecture, Go version, Grok version, and model list.

- [x] **Step 2: Create and verify isolated Grok state**

Create mode-`0700` temporary HOME/GROK_HOME, copy only current `auth.json` and
`agent_id` with mode `0600`, and create an operator-owned wrapper outside Git.
Run `grok inspect --json` through the wrapper and require empty project
instructions, plugins, skills, MCP servers, and hooks. Preserve only the
redaction-safe inspection summary. Run formal phases through one-shot
LaunchAgent plists with `RunAtLoad=true`, `KeepAlive=false`, an explicit
`PATH=/opt/homebrew/bin:/usr/bin:/bin:/usr/sbin:/sbin`, independent exit
markers, and post-run proof that `runs=1`. Do not use foreground PTYs, `nohup`,
`screen`, or `launchctl submit` as the formal execution transport.

- [x] **Step 3: Run reader phase with bounded concurrency**

Use formal run ID:

```text
longmemeval-s-full-reader-qa-grok-20260715-v3
```

Run against the pinned source and W14 JSONL. Capture wall time, max RSS, raw log
SHA-256, checkpoint counts, attempt/failure counts, and provider usage totals.
Do not stop the complete run for individual provider failures.

The discarded `v1` prequalification run used an empty Grok tool allowlist and
was stopped after an observed `update_goal` path proved that boundary
ineffective. The discarded `v2` run corrected that boundary, but two host PTY
interruptions and a `launchctl submit` resume with a system-only `PATH` produced
188 terminal `env: node: No such file or directory` failures; the submit job
also used inferred keepalive semantics. Preserve both failed runs outside the
formal `v3` artifact root. Before starting `v3`, require one-shot LaunchAgent
debug probes for both formal models to report `runs=1`, exit zero,
`tool_count=0`, no tool call, one turn, and `EndTurn`.

- [x] **Step 4: Run custom judge and finalize**

Use the same isolated wrapper with `grok-4.5`. Require one terminal judge state
for every completed reader response, then finalize all artifacts. Preserve all
invalid labels and exhausted attempts.

- [x] **Step 5: Prove resume**

Run `--phase all --resume` against the same artifact root. Require:

```text
zero new reader provider calls
zero new judge provider calls
zero changed checkpoint bytes
unchanged normalized reader-results hash
unchanged normalized judge-results hash
unchanged scores hash
unchanged failure-ledger hash
```

- [x] **Step 6: Normalize evidence without hiding failures**

Commit aggregate scores, complete failure categories, provider/model identity,
usage totals, source/W14 hashes, known six-record attribution, exact commands
with secrets omitted, and explicit non-claims. Do not commit full source
sessions, copied auth, raw prompt contexts, or provider session files.

- [x] **Step 7: Commit formal evidence**

```bash
git add docs/evidence casebook/benchmarks docs/evaluation-matrix.md README.md README.zh-CN.md
git commit -m "docs: qualify full LongMemEval reader QA"
```

### Task 9: Protected Delivery

**Files:**
- Modify: `docs/superpowers/plans/2026-07-15-longmemeval-s-full-reader-qa.md`
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
go run github.com/goreleaser/goreleaser/v2@v2.17.0 check --config .goreleaser.yaml
go run github.com/goreleaser/goreleaser/v2@v2.17.0 release --snapshot --clean --skip=publish --config .goreleaser.yaml
git diff --check
```

Verify four archive checksums/layouts/build metadata, OpenClaw 12-entry package,
Darwin arm64 `version`, and `benchmark-longmemeval-qa --help`.

- [x] **Step 2: Push the evidence head and update Draft PR**

Mark the local-gate item from fresh output, commit that state, push
`agent/grok-cli-runtime`, and append the full QA result and non-claims to Draft
PR 1 without changing it from Draft. Leave remote acceptance unchecked.

- [x] **Step 3: Verify the evidence-head CI and artifact independently**

Download the new transport ZIP through GitHub's artifact API, match its digest
and byte count, verify all four archives and metadata, execute Darwin arm64
`version` and `benchmark-longmemeval-qa --help`, verify the signed synthetic
merge second parent, and require `OPEN / Draft / CLEAN / MERGEABLE /
test=SUCCESS`, zero tags, and zero Releases.

- [x] **Step 4: Close the checklist on a final protected head**

Mark the remaining W15 items, commit `docs: close full LongMemEval reader QA`,
push again, and require that final checklist head to pass a second protected CI
and independent artifact verification. Append only final immutable run, job,
artifact, digest, merge, and PR-state identifiers to the PR body so no third
documentation commit is created.

- [x] **Step 5: Keep the platform goal active**

W15 completion advances public full reader QA only. The overall goal remains
active for a genuinely external sealed evaluator, withheld cases, long-duration
retention, active-backlog dimensional migration, HA/failover/PITR, signing, and
final release acceptance.
