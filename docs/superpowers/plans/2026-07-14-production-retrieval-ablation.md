# Production Retrieval Ablation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Measure the current production lexical runtime, direct PostgreSQL/pgvector retrieval, and deterministic hybrid RRF on one lifecycle-aware, multi-scope corpus without changing Vermory's default retrieval path.

**Architecture:** A versioned W08 corpus is materialized through the real runtime authority APIs into a dedicated PostgreSQL database and mirrored into the existing disposable native pgvector backend with the same governed memory IDs. A pure ablation package executes and scores lexical, vector, and hybrid conditions, rechecks vector eligibility against PostgreSQL authority, verifies degradation and rebuild behavior, and writes deterministic JSON/Markdown evidence.

**Tech Stack:** Go 1.26, PostgreSQL 18 with PostgreSQL 16+ SQL compatibility, pgvector, pgx v5, Cobra, existing `internal/runtime` and `internal/memorybackend` packages, direct SiliconFlow OpenAI-compatible embeddings.

## Global Constraints

- PostgreSQL governed memory and lifecycle remain authoritative; the vector index is disposable.
- Do not modify `runtime.Store.SearchActiveMemory` or switch the product default in W08.
- The real embedding profile is `https://api.siliconflow.cn/v1`, `BAAI/bge-m3`, 1024 dimensions.
- The API key is read only from a named environment variable and never written to artifacts, arguments, or Git.
- Mac mini NewAPI, Gemini CLI, mem0, MemOS, Supermemory, Redis, and an LLM reranker are not used.
- Database tests using `VERMORY_TEST_DATABASE_URL` run with `-p 1`.
- Fixed hard gates are zero cross-tenant, cross-continuity, superseded, deleted, and proposed result violations.
- Quality and latency winning thresholds are measured but not frozen in this slice.
- Public corpus results are public evidence, not sealed evidence.

---

### Task 1: Freeze The W08 Corpus And Loader

**Files:**
- Create: `runtime/cases/W08-production-retrieval-ablation/corpus.json`
- Create: `internal/retrievalablation/corpus.go`
- Create: `internal/retrievalablation/corpus_test.go`

**Interfaces:**
- Consumes: existing `casebook/cases/101-*` through `109-*`, `201-*` through `205-*`, and `301-*` through `305-*` provenance paths.
- Produces: `LoadCorpus(path string) (Corpus, error)`, `ValidateCorpus(root string, corpus Corpus) error`, and `CorpusSHA256(corpus Corpus) (string, error)`.

- [x] **Step 1: Define the frozen corpus types and canonical lifecycle vocabulary.**

```go
type Corpus struct {
    Version string        `json:"version"`
    Name    string        `json:"name"`
    Scopes  []Scope       `json:"scopes"`
    Records []Record      `json:"records"`
    Queries []Query       `json:"queries"`
}

type Scope struct {
    ID       string `json:"id"`
    TenantID string `json:"tenant_id"`
    Line     string `json:"line"` // workspace or conversation
    Anchor   string `json:"anchor"`
}

type Record struct {
    ID             string `json:"id"`
    ScopeID        string `json:"scope_id"`
    MemoryKey      string `json:"memory_key,omitempty"`
    Content        string `json:"content"`
    Lifecycle      string `json:"lifecycle"` // active, proposed, superseded, deleted
    ReplacementID  string `json:"replacement_id,omitempty"`
    ProvenanceCase string `json:"provenance_case"`
}

type Query struct {
    ID                 string   `json:"id"`
    ScopeID            string   `json:"scope_id"`
    Text               string   `json:"text"`
    Limit              int      `json:"limit"`
    RelevantRecordIDs  []string `json:"relevant_record_ids"`
    ForbiddenRecordIDs []string `json:"forbidden_record_ids"`
    Cohorts            []string `json:"cohorts"`
}
```

- [x] **Step 2: Write loader tests that reject duplicate IDs, unknown scopes, unsupported lines/lifecycles, missing provenance files, empty query sets, limits outside 1-12, overlapping relevant/forbidden IDs, non-active relevant records, and forbidden IDs that do not exist.**

Run:

```bash
go test -count=1 ./internal/retrievalablation -run 'Corpus'
```

Expected: FAIL because the package and corpus do not exist.

- [x] **Step 3: Implement strict JSON loading, unknown-field rejection, canonical sorting for hashing, provenance containment under `casebook/cases`, and corpus validation.**

The loader must reject trailing JSON and produce a lowercase 64-character SHA-256 over canonical JSON with sorted scopes, records, queries, and every ID list.

- [x] **Step 4: Write `corpus.json` with at least four tenants, eight scopes, 48 active records, four proposed records, four superseded records, four deleted records, and 24 queries across every cohort frozen in the design.**

Every record must cite an existing case directory and every query must include at least one relevant and one forbidden record. The corpus must include exact paths, flags, error codes, model names, dates, durations, numbers, Chinese paraphrases, English paraphrases, mixed-language requests, multi-fact queries, same-scope distractors, cross-continuity distractors, and cross-tenant distractors.

- [x] **Step 5: Run corpus tests and a JSON syntax check, then commit.**

```bash
jq empty runtime/cases/W08-production-retrieval-ablation/corpus.json
go test -count=1 ./internal/retrievalablation -run 'Corpus'
git add runtime/cases/W08-production-retrieval-ablation/corpus.json internal/retrievalablation/corpus.go internal/retrievalablation/corpus_test.go
git commit -m "test: freeze production retrieval ablation corpus"
```

### Task 2: Deterministic Fusion And Metrics

**Files:**
- Create: `internal/retrievalablation/types.go`
- Create: `internal/retrievalablation/fusion.go`
- Create: `internal/retrievalablation/fusion_test.go`
- Create: `internal/retrievalablation/metrics.go`
- Create: `internal/retrievalablation/metrics_test.go`

**Interfaces:**
- Consumes: Task 1 `Query` values and ranked results from later search adapters.
- Produces: `FuseRRF`, `ScoreQuery`, `AggregateMetrics`, `ConditionReport`, `QueryReport`, and stable condition names.

- [x] **Step 1: Define ranked result and report types.**

```go
const (
    ConditionLexical = "lexical_runtime"
    ConditionVector  = "vector_pg"
    ConditionHybrid  = "hybrid_rrf"
    RRFK             = 60
)

type RankedResult struct {
    MemoryID     string  `json:"memory_id"`
    RecordID     string  `json:"record_id"`
    Content      string  `json:"content,omitempty"`
    Score        float64 `json:"score"`
    LexicalRank  int     `json:"lexical_rank,omitempty"`
    VectorRank   int     `json:"vector_rank,omitempty"`
    Exact        bool    `json:"exact"`
    Eligible     bool    `json:"eligible"`
}

type QueryMetrics struct {
    HitAt1             float64 `json:"hit_at_1"`
    RecallAtK          float64 `json:"recall_at_k"`
    MRR                float64 `json:"mrr"`
    NDCGAtK            float64 `json:"ndcg_at_k"`
    ForbiddenCount     int     `json:"forbidden_count"`
    IneligibleCount    int     `json:"ineligible_count"`
}
```

- [x] **Step 2: Write failing fusion tests for deduplication, exact-guard precedence, the exact `1/(60+rank)` formula, missing ranks, deterministic ties, limit truncation, and lexical-only degradation preserving IDs and order.**

- [x] **Step 3: Implement `FuseRRF(query string, limit int, lexical, vector []RankedResult) []RankedResult` without provider calls, mutable global weights, or content-based special cases beyond the frozen exact substring guard.**

- [x] **Step 4: Write failing metric tests for hit@1, recall@k, MRR, binary nDCG, multi-relevant queries, forbidden results, ineligible results, zero results, per-cohort aggregation, and deterministic percentile latency.**

- [x] **Step 5: Implement scoring and aggregation with no LLM judge.**

`ScoreQuery` compares stable record IDs. `AggregateMetrics` computes macro averages over queries and exact integer violation totals. Empty relevant sets are invalid corpus input rather than a special score.

- [x] **Step 6: Run pure package tests and commit.**

```bash
go test -count=1 ./internal/retrievalablation -run 'Fusion|Metric'
git add internal/retrievalablation/types.go internal/retrievalablation/fusion.go internal/retrievalablation/fusion_test.go internal/retrievalablation/metrics.go internal/retrievalablation/metrics_test.go
git commit -m "feat: score deterministic retrieval ablations"
```

### Task 3: PostgreSQL Authority Runner

**Files:**
- Create: `internal/retrievalablation/runner.go`
- Create: `internal/retrievalablation/runner_test.go`
- Create: `internal/retrievalablation/seed.go`
- Create: `internal/retrievalablation/seed_test.go`
- Modify: `internal/memorybackend/native.go`
- Modify: `internal/memorybackend/native_test.go`

**Interfaces:**
- Consumes: `runtime.Store`, `memorybackend.Backend`, Task 1 corpus, and Task 2 fusion/metrics.
- Produces: `Run(ctx context.Context, options Options) (Report, error)`, `SeedCorpus`, `RunCondition`, and rebuild/degradation evidence.

- [x] **Step 1: Define the runner dependency boundary.**

```go
type LexicalSearcher interface {
    SearchActiveMemory(ctx context.Context, tenantID, continuityID, query string, limit int) ([]runtime.Memory, error)
    ListGovernedMemories(ctx context.Context, tenantID, continuityID string) ([]runtime.GovernedMemory, error)
}

type Options struct {
    DatabaseURL          string
    CorpusPath           string
    RunID                string
    EmbeddingBaseURL     string
    EmbeddingAPIKey      string
    EmbeddingModel       string
    EmbeddingDimensions  int
    ImplementationRevision string
}
```

- [x] **Step 2: Write failing database tests that materialize active, proposed, superseded, deleted, cross-continuity, and cross-tenant records exclusively through runtime APIs and require a stable corpus-record-to-memory-ID map.**

Run:

```bash
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' go test -p 1 -count=1 ./internal/retrievalablation -run 'Seed|Authority'
```

Expected: FAIL because seeding and the runner are absent.

- [x] **Step 3: Implement corpus seeding with `ConfirmWorkspaceBinding`, `ResolveOrCreateConversation`, `CommitGovernedObservation`, supersession, and `DeleteMemory`.**

Use deterministic operation IDs derived from run ID plus record ID. Active
vector records use the returned governed memory IDs. Proposed records are never
inserted into the ANN projection; superseded and deleted records exercise a
put/delete transition and must be absent from the final index. Never insert
directly into authoritative runtime tables or `memory_search_documents`.

- [x] **Step 4: Make native backend scope rebuild deterministic and expose no authority mutation.**

If existing `RebuildScope` ordering is unstable, sort records by ID before embedding and insertion. Do not add lifecycle or authority semantics beyond the existing `Record.Status` filter.

- [x] **Step 5: Write failing runner tests for all three conditions, vector eligibility recheck, exact lexical fallback on vector error, completed-query preservation after another query fails, and projection reset/rebuild result equivalence.**

- [x] **Step 6: Implement the runner.**

For each query:

1. resolve logical scope to real tenant and continuity IDs;
2. call production lexical search;
3. retain the production lexical maximum of 12 candidates and call vector
   search with `max(20, limit*4)` candidates capped at 100;
4. map memory IDs back to corpus record IDs;
5. recheck every vector result against current active governed memories;
6. score lexical, eligible vector, and hybrid outputs;
7. persist query evidence in the in-memory report even if a later query fails.

- [x] **Step 7: Add a forced vector-outage backend and require hybrid results to equal lexical results byte-for-byte in ID order with `degraded_to_lexical=true`.**

- [x] **Step 8: Run focused serial database tests and native backend tests, then commit.**

```bash
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' go test -p 1 -count=1 ./internal/retrievalablation ./internal/memorybackend
git add internal/retrievalablation/runner.go internal/retrievalablation/runner_test.go internal/retrievalablation/seed.go internal/retrievalablation/seed_test.go internal/memorybackend/native.go internal/memorybackend/native_test.go
git commit -m "feat: run governed retrieval ablations"
```

### Task 4: CLI And Deterministic Reports

**Files:**
- Create: `internal/retrievalablation/report.go`
- Create: `internal/retrievalablation/report_test.go`
- Create: `cmd/vermory/retrieval_ablation.go`
- Create: `cmd/vermory/retrieval_ablation_test.go`
- Modify: `cmd/vermory/main.go`

**Interfaces:**
- Consumes: Task 3 `Run` and `Report`.
- Produces: `vermory retrieval-ablation`, `report.json`, and `report.md`.

- [x] **Step 1: Write failing report tests that require deterministic JSON and Markdown, canonical condition/cohort ordering, corpus hash, implementation revision, schema version, authority fingerprint, embedding profile, engine version, every query trace, hard-gate status, failures, non-claims, exact artifact replay, and conflicting replay rejection without reseeding deleted content.**

- [x] **Step 2: Implement `WriteReport(outputDir string, report Report) (ArtifactPaths, bool, error)` using atomic temporary files followed by rename.**

The report must omit API keys, request headers, database credentials, private environment values, and raw private corpus text. The Markdown renderer must derive only from the JSON report value.

- [x] **Step 3: Write failing Cobra tests for every required flag, missing API-key environment variable, invalid corpus, stable success output, provider failure, and absence of secrets in stdout/stderr.**

- [x] **Step 4: Add `newRetrievalAblationCommand` and register it in `newRootCommand`.**

The command reads the API key with `os.Getenv(options.EmbeddingAPIKeyEnv)`, passes the value only in memory, and prints one line containing run ID, query count, hard-gate status, qualification status, and report path.

- [x] **Step 5: Run command/package tests and commit.**

```bash
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' go test -p 1 -count=1 ./internal/retrievalablation ./cmd/vermory
git add internal/retrievalablation/report.go internal/retrievalablation/report_test.go cmd/vermory/retrieval_ablation.go cmd/vermory/retrieval_ablation_test.go cmd/vermory/main.go
git commit -m "feat: expose production retrieval ablation"
```

### Task 5: Real SiliconFlow Qualification

**Files:**
- Create: `docs/evidence/2026-07-14-production-retrieval-ablation.md`
- Create: `docs/evidence/snapshots/2026-07-14-production-retrieval-ablation-report.json`
- Modify: `docs/evaluation-matrix.md`
- Modify: `docs/superpowers/specs/2026-07-11-vermory-hypothesis-register.md`
- Modify: `README.md`

**Interfaces:**
- Consumes: isolated release binary, dedicated PostgreSQL database, direct SiliconFlow embeddings, and W08 corpus.
- Produces: one public measured retrieval result, preserved failures, and an H-009 decision state that does not change the runtime default.

- [x] **Step 1: Build an isolated release binary, create a dedicated database, migrate it, and record PostgreSQL, schema, pgvector, binary revision, corpus hash, and corpus counts.**

- [x] **Step 2: Verify direct `BAAI/bge-m3` embedding compatibility with one bounded probe, recording only HTTP status, model, vector dimensions, duration, and response artifact SHA-256.**

Do not log the authorization header or key. Preserve provider timeout/rate-limit failures if they occur.

- [x] **Step 3: Execute the full W08 run once with a stable run ID and require all lifecycle/scope hard gates to pass.**

- [x] **Step 4: Force vector failure and require every hybrid query to degrade to the exact lexical IDs/order without authority changes.**

- [x] **Step 5: Delete the vector projection, rebuild it from the governed corpus, rerun the vector and hybrid conditions, and require result-ID equivalence plus an unchanged authority fingerprint.**

- [x] **Step 6: Review the measured cohort deltas without tuning the corpus or formula, then set H-009 to `testing` with `measured`, `hard_gate_failed`, or `ready_for_threshold_review` exactly as supported by the report.**

- [x] **Step 7: Commit a normalized report snapshot and evidence document containing exact commands, versions, hashes, counts, metrics, per-cohort deltas, failures, degradation, rebuild equivalence, and explicit non-claims.**

```bash
git add docs/evidence/2026-07-14-production-retrieval-ablation.md docs/evidence/snapshots/2026-07-14-production-retrieval-ablation-report.json docs/evaluation-matrix.md docs/superpowers/specs/2026-07-11-vermory-hypothesis-register.md README.md
git commit -m "docs: record production retrieval ablation evidence"
```

### Task 6: Full Verification And Delivery

**Files:**
- Modify: `docs/superpowers/plans/2026-07-14-production-retrieval-ablation.md`
- Modify: Draft PR 1 body.

**Interfaces:**
- Consumes: all W08 implementation and evidence.
- Produces: green local and protected-CI gates, independently verified release artifact, clean commits, and an updated Draft PR while the overall Vermory goal remains active.

- [x] **Step 1: Run the complete serial PostgreSQL suite, selected runtime/new-package race suite, reality race, vet, tidy, module diff, and diff check.**

```bash
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' go test -p 1 -count=1 ./...
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' go test -race -p 1 -count=1 ./internal/authn ./internal/runtime ./internal/webchat ./internal/identitycli ./internal/operatorcli ./internal/mcpserver ./internal/provider ./internal/retrievalablation ./cmd/vermory
go test -race -count=1 ./internal/reality
go vet ./...
go mod tidy
git diff --exit-code -- go.mod go.sum
git diff --check
```

- [x] **Step 2: Run Actionlint, GoReleaser check and four-platform snapshot/checksums, downloaded host archive execution, OpenClaw tests/typecheck/build/package, schema replay, RLS/runtime-role checks, and native backup/restore including the unchanged runtime authority fingerprint.**

- [x] **Step 3: Scan all tracked and public evidence files for credential-shaped values and require zero matches. Remove every dedicated database, temporary runtime role, API transcript containing headers, isolated HOME, and downloaded local artifact after normalized evidence is committed.**

- [x] **Step 4: Mark the plan checklist from fresh evidence, push `agent/grok-cli-runtime`, wait for protected CI, download the final artifact, verify GitHub digest, all four archive checksums/layouts, OpenClaw package, and darwin/arm64 execution.**

- [x] **Step 5: Update Draft PR 1 with the W08 result, preserved limitations, final run/job/artifact/digest, and confirmation that it remains Draft, `CLEAN`, `MERGEABLE`, and required `test=SUCCESS`.**

- [x] **Step 6: Keep the overall Vermory goal active. W08 completion does not complete production hybrid integration, source authority ranking, scale/fault qualification, sealed evaluation, signing, or final release acceptance.**
