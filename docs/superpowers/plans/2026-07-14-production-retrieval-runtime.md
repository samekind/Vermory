# Production Retrieval Runtime Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Connect one governed active-only `BAAI/bge-m3` PostgreSQL/pgvector profile to the real workspace MCP and conversation Web Chat runtimes with durable projection events, explicit lexical/shadow/vector modes, exact lexical fallback, and auditable failure behavior while keeping lexical as the default.

**Architecture:** Migration 14 appends tenant-scoped governed-memory lifecycle events and stores per-profile cursors, active-only vector documents, and retrieval audit rows. A fixed-tenant worker rechecks current PostgreSQL authority before every vector mutation. A shared runtime coordinator serves workspace and linked-conversation retrieval, records non-sensitive evidence, and falls back byte-for-byte to the existing lexical path whenever the projection or provider is unavailable.

**Tech Stack:** Go 1.26, PostgreSQL 18, pgvector 0.8.5, pgx v5, Goose, Cobra, MCP Go SDK, existing runtime RLS/authentication, direct SiliconFlow OpenAI-compatible embeddings, logged-in Grok CLI.

## Global Constraints

- PostgreSQL governed memory is the only authority.
- `memory_search_documents` and lexical retrieval remain the default.
- The only accepted W09 profile is `siliconflow-bge-m3-1024-v1`, direct base URL `https://api.siliconflow.cn/v1`, model `BAAI/bge-m3`, dimensions `1024`.
- No authority transaction may call the embedding provider.
- Provider or vector failure must preserve authority and return the exact lexical IDs and order.
- Proposed, superseded, deleted, redacted, Global Defaults, cross-tenant, and unauthorized cross-continuity rows must never enter delivered vector context.
- W09 adds no RRF, reranker, default switch, profile migration, privileged all-tenant worker, or scale claim.
- API keys are read only from a named environment variable and never written to files, arguments, audit rows, artifacts, logs, or Git.
- Mac mini NewAPI and Gemini CLI are not used.
- Database tests using `VERMORY_TEST_DATABASE_URL` run with `-p 1`.

---

### Task 1: Migration 14, Trigger, RLS, And Runtime Role

**Files:**
- Create: `internal/store/postgres/migrations/00014_production_retrieval_runtime.sql`
- Create: `internal/runtime/retrieval_migration_test.go`
- Modify: `internal/runtime/postgres_store.go`
- Modify: `internal/runtime/rls_migration_test.go`
- Modify: `internal/runtime/tenant_pool_test.go`
- Modify: `internal/authn/role.go`
- Modify: `internal/identitycli/command_test.go`

**Interfaces:**
- Consumes: existing `governed_memories`, tenant-aware keys, `vermory.tenant_id`, `GrantRuntimeRole`, `ValidateRuntimeRole`, migration 13.
- Produces: `memory_projection_events`, `memory_projection_cursors`, `memory_vector_documents`, `memory_retrieval_runs`, trigger `enqueue_memory_projection_event`, schema version 14, and restricted-role access to the new served tables.

- [x] **Step 1: Write migration tests that require the four tables, exact checks, tenant-aware foreign keys, RLS policies, HNSW cosine index, governed-memory trigger, and one seed event per existing governed memory.**

The tests must migrate a schema-13 fixture containing active, proposed,
superseded, deleted, and Global Defaults rows, apply migration 14, and assert:

```text
active fact            -> desired_state active
all other memory       -> desired_state absent
event contains no content column
vector embedding type  -> vector(1024)
```

Run:

```bash
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' \
  go test -p 1 -count=1 ./internal/runtime -run 'RetrievalMigration|Migration14'
```

Expected: FAIL because migration 14 and its tables do not exist.

- [x] **Step 2: Add runtime-role and RLS tests for no-context, selected-tenant, other-tenant, cross-tenant foreign-key, ownership, required privilege, and forbidden legacy-table behavior.**

The restricted role must have served-table privileges but must not own tables,
bypass RLS, or read `vermory_auth.api_tokens` or Phase 1 legacy tables.

- [x] **Step 3: Implement migration 14.**

Use these canonical status/profile values:

```sql
CHECK (desired_state IN ('active', 'absent'))
CHECK (status IN ('idle', 'running', 'failed'))
CHECK (requested_mode IN ('lexical', 'shadow', 'vector'))
CHECK (effective_mode IN ('lexical', 'shadow', 'vector'))
CHECK (profile_id = 'siliconflow-bge-m3-1024-v1')
```

The trigger must append an event only when the projected state may have changed.
The migration seed insert must order by governed-memory creation and ID so tests
can reproduce the event stream.

- [x] **Step 4: Add all four tables to test reset, runtime-role grant, runtime-role validation, RLS matrix, and database command acceptance.**

`ResetForTest` truncates child projection/audit tables before authority tables.
`ValidateRuntimeRole` requires the same CRUD boundary as the other served
runtime tables and still rejects table owners and inherited forbidden access.

- [x] **Step 5: Run migration, RLS, role, command, and full serial database tests, then commit.**

```bash
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' \
  go test -p 1 -count=1 ./internal/runtime ./internal/authn ./internal/identitycli
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' \
  go test -p 1 -count=1 ./...
git add internal/store/postgres/migrations/00014_production_retrieval_runtime.sql \
  internal/runtime/retrieval_migration_test.go internal/runtime/postgres_store.go \
  internal/runtime/rls_migration_test.go internal/runtime/tenant_pool_test.go \
  internal/authn/role.go internal/identitycli/command_test.go
git commit -m "feat: add production retrieval projection schema"
```

### Task 2: Durable Projection Store And Fixed-Tenant Worker

**Files:**
- Create: `internal/runtime/retrieval_types.go`
- Create: `internal/runtime/retrieval_store.go`
- Create: `internal/runtime/retrieval_store_test.go`
- Create: `internal/runtime/retrieval_worker.go`
- Create: `internal/runtime/retrieval_worker_test.go`
- Modify: `internal/memorybackend/embedding.go`
- Modify: `internal/memorybackend/native_test.go`

**Interfaces:**
- Consumes: Task 1 event/cursor/vector tables and existing OpenAI-compatible embedding HTTP implementation.
- Produces: `RetrievalProfile`, `ProjectionStatus`, `ProjectionEvent`, `ProjectionWorker`, `NewProjectionWorker`, `RunOnce`, `Run`, `Store.RetrievalProjectionStatus`, `Store.ResetVectorProjection`, and exported `memorybackend.NewOpenAIEmbedder` through a small `Embedder` interface.

- [x] **Step 1: Define and validate the frozen profile and worker options.**

```go
const ProductionRetrievalProfileID = "siliconflow-bge-m3-1024-v1"

type RetrievalProfile struct {
    ID         string
    BaseURL    string
    Model      string
    Dimensions int
}

type Embedder interface {
    Embed(context.Context, string) ([]float32, error)
}

type ProjectionWorkerOptions struct {
    TenantID    string
    Profile     RetrievalProfile
    BatchSize   int
    PollInterval time.Duration
}

func NewProjectionWorker(store *Store, embedder Embedder, options ProjectionWorkerOptions) (*ProjectionWorker, error)
func (w *ProjectionWorker) RunOnce(context.Context) (ProjectionRunResult, error)
func (w *ProjectionWorker) Run(context.Context) error
```

Validation rejects any profile/model/dimension mismatch before opening a
provider request.

- [x] **Step 2: Write failing PostgreSQL tests for ordered event reads, cursor creation, monotonic advance, duplicate replay, reset, status, and active-only vector rows.**

Tests must prove that current authority wins over event history: processing an
old `active` event after deletion deletes/skips the vector row.

- [x] **Step 3: Write failing worker tests for success, provider HTTP 503, wrong dimensions, concurrent worker lock, cancellation, and deletion/supersession committed while embedding is in flight.**

Use a deterministic local HTTP embedding endpoint. A blocked endpoint allows a
test to commit deletion before releasing the response; the late completion must
not restore the row.

- [x] **Step 4: Export the existing embedding constructor without changing request semantics.**

The exported constructor must continue omitting the unsupported optional
`dimensions` request field and must reject a response whose vector length is not
exactly 1024.

- [x] **Step 5: Implement the store and worker with a tenant/profile PostgreSQL advisory lock.**

`RunOnce` processes at most `BatchSize` events. It may hold one acquired
connection and advisory lock across the provider call, but it must not hold an
authority transaction across the network. Immediately before vector upsert it
re-reads the memory and content hash in the mutation transaction.

Canonical bounded failure codes:

```text
already_running
embedding_unavailable
embedding_dimension_mismatch
projection_read_error
projection_write_error
authority_changed
```

- [x] **Step 6: Run worker/store tests, native embedding tests, race tests, and commit.**

```bash
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' \
  go test -p 1 -count=1 ./internal/runtime -run 'Projection|Worker'
go test -count=1 ./internal/memorybackend -run 'Embed|Native'
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' \
  go test -race -p 1 -count=1 ./internal/runtime ./internal/memorybackend
git add internal/runtime/retrieval_types.go internal/runtime/retrieval_store.go \
  internal/runtime/retrieval_store_test.go internal/runtime/retrieval_worker.go \
  internal/runtime/retrieval_worker_test.go internal/memorybackend/embedding.go \
  internal/memorybackend/native_test.go
git commit -m "feat: process durable retrieval projections"
```

### Task 3: Shared Retrieval Coordinator And Audit

**Files:**
- Create: `internal/runtime/retrieval_coordinator.go`
- Create: `internal/runtime/retrieval_coordinator_test.go`
- Modify: `internal/runtime/retrieval_store.go`
- Modify: `internal/runtime/retrieval_types.go`
- Modify: `internal/runtime/conversation_store.go`

**Interfaces:**
- Consumes: existing lexical workspace/conversation search, linked-conversation scope resolution, Task 2 embedder, projection status, vector documents, and Task 1 audit table.
- Produces: `RetrievalMode`, `MemoryRetriever`, `RetrievalRequest`, `RetrievalResult`, `NewRetrievalCoordinator`, workspace/linked-conversation semantic search, exact lexical fallback, and idempotent `memory_retrieval_runs` records.

- [x] **Step 1: Define the coordinator contract.**

```go
type RetrievalMode string

const (
    RetrievalLexical RetrievalMode = "lexical"
    RetrievalShadow  RetrievalMode = "shadow"
    RetrievalVector  RetrievalMode = "vector"
)

type RetrievalRequest struct {
    OperationID   string
    TenantID      string
    ContinuityIDs []string
    Query         string
    Limit         int
    Mode          RetrievalMode
}

type RetrievalResult struct {
    Memories  []Memory
    Effective RetrievalMode
    Degraded  bool
    AuditID   string
}

type MemoryRetriever interface {
    Retrieve(context.Context, RetrievalRequest) (RetrievalResult, error)
}

func NewRetrievalCoordinator(store *Store, embedder Embedder, profile RetrievalProfile) (*RetrievalCoordinator, error)
```

- [x] **Step 2: Write failing pure and PostgreSQL tests for mode validation, exact shadow byte-equivalence, vector delivery, cursor-lag fallback, provider fallback, empty-vector operational fallback, authority/content-hash filtering, and audit replay conflict.**

Every fallback compares stable memory IDs and order to the already completed
lexical result. The audit stores SHA-256 and IDs only.

- [x] **Step 3: Add one store method that resolves the existing linked conversation root into a sorted authorized continuity-ID set.**

Do not duplicate bridge/link SQL inside the coordinator. Workspace passes its
single confirmed continuity; conversation obtains the set through the store.

- [x] **Step 4: Implement vector search with the frozen profile, tenant and authorized continuity filters, cosine ordering, deterministic ID tie break, and PostgreSQL authority/content-hash recheck.**

Request `max(20, limit*4)` candidates capped at 100, then truncate eligible
results to the caller limit. Do not fuse with lexical.

- [x] **Step 5: Implement idempotent non-sensitive audit recording.**

The request fingerprint covers tenant, sorted continuity IDs, query SHA-256,
limit, requested mode, and profile. Replaying the same operation returns the
same audit identity; changing any field fails.

- [x] **Step 6: Run coordinator, conversation-link, RLS, race tests, and commit.**

```bash
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' \
  go test -p 1 -count=1 ./internal/runtime -run 'Retrieval|ConversationLink'
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' \
  go test -race -p 1 -count=1 ./internal/runtime
git add internal/runtime/retrieval_coordinator.go \
  internal/runtime/retrieval_coordinator_test.go internal/runtime/retrieval_store.go \
  internal/runtime/retrieval_types.go internal/runtime/conversation_store.go
git commit -m "feat: coordinate governed runtime retrieval"
```

### Task 4: MCP, Web Chat, Authenticated API, And Operator Commands

**Files:**
- Create: `cmd/vermory/retrieval_runtime.go`
- Create: `cmd/vermory/retrieval_runtime_test.go`
- Modify: `cmd/vermory/main.go`
- Modify: `cmd/vermory/main_test.go`
- Modify: `cmd/vermory/web_chat.go`
- Modify: `cmd/vermory/web_chat_test.go`
- Modify: `cmd/vermory/serve.go`
- Modify: `cmd/vermory/serve_test.go`
- Modify: `internal/runtime/service.go`
- Modify: `internal/runtime/service_test.go`
- Modify: `internal/runtime/conversation_service.go`
- Modify: `internal/runtime/conversation_types.go`
- Modify: `internal/runtime/conversation_service_test.go`
- Modify: `internal/webchat/authenticated_handler.go`
- Modify: `internal/webchat/authenticated_handler_test.go`

**Interfaces:**
- Consumes: Task 3 `MemoryRetriever`, Task 2 worker/status/reset, existing MCP and Web Chat services.
- Produces: shared retrieval CLI flags, `retrieval-worker`, `retrieval-status`, `retrieval-rebuild`, opt-in retrieval for `mcp-stdio`, `web-chat`, and `serve`, with unchanged public MCP/Web Chat output schemas.

- [x] **Step 1: Write failing CLI tests for default lexical behavior, required non-lexical flags, frozen profile validation, missing API-key environment variable, secret-free errors, worker/status/rebuild registration, and absence of internal fields from MCP/Web Chat output.**

Required shared flags:

```text
retrieval-mode
retrieval-profile
embedding-base-url
embedding-api-key-env
embedding-model
embedding-dimensions
```

- [x] **Step 2: Add optional retriever injection while preserving all existing constructors.**

```go
func NewService(store *Store, tenantID string) *Service
func NewServiceWithRetriever(store *Store, tenantID string, retriever MemoryRetriever) *Service
```

Add `Retriever MemoryRetriever` to `ConversationServiceConfig`; its zero value
continues to call `SearchActiveConversationMemory` directly.

- [x] **Step 3: Modify workspace and conversation preparation to use the retriever only when configured.**

Workspace operation ID is `workspace-retrieval:` plus the request operation ID.
Conversation operation ID is `conversation-retrieval:` plus the turn operation
ID. Delivery context remains semantic text only.

- [x] **Step 4: Implement shared command-side option validation and coordinator construction.**

Lexical mode must not read an embedding environment variable or open a provider
client. Shadow/vector mode reads the key once into memory, creates the exported
embedder, and never prints configuration values containing credentials or the
database URL.

- [x] **Step 5: Implement the worker, status, and rebuild commands.**

`retrieval-status` writes one JSON object. `retrieval-rebuild` resets only the
selected tenant/profile vector rows and cursor. `retrieval-worker --once`
writes processed count, final cursor, lag, status, and bounded failure code.

- [x] **Step 6: Wire MCP, local Web Chat, and authenticated API to the same coordinator.**

The authenticated handler passes each authenticated principal tenant to the
coordinator. It does not start a cross-tenant worker. A missing/currently stale
tenant projection falls back to lexical.

- [x] **Step 7: Run command, MCP, Web Chat, authn, runtime, and race tests, then commit.**

```bash
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' \
  go test -p 1 -count=1 ./cmd/vermory ./internal/mcpserver ./internal/webchat ./internal/runtime
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' \
  go test -race -p 1 -count=1 ./cmd/vermory ./internal/mcpserver ./internal/webchat ./internal/runtime
git add cmd/vermory/retrieval_runtime.go cmd/vermory/retrieval_runtime_test.go \
  cmd/vermory/main.go cmd/vermory/main_test.go cmd/vermory/web_chat.go \
  cmd/vermory/web_chat_test.go cmd/vermory/serve.go cmd/vermory/serve_test.go \
  internal/runtime/service.go internal/runtime/service_test.go \
  internal/runtime/conversation_service.go internal/runtime/conversation_types.go \
  internal/runtime/conversation_service_test.go internal/webchat/authenticated_handler.go \
  internal/webchat/authenticated_handler_test.go
git commit -m "feat: expose opt-in production retrieval"
```

### Task 5: Freeze W09 Runtime Cases And End-To-End Database Acceptance

**Files:**
- Create: `runtime/cases/W09-production-retrieval-runtime/case.json`
- Create: `runtime/cases/W09-production-retrieval-runtime/README.md`
- Create: `internal/runtime/retrieval_acceptance_test.go`
- Modify: `internal/runtime/operations_acceptance_test.go`
- Modify: `internal/runtime/rls_migration_test.go`

**Interfaces:**
- Consumes: Tasks 1-4 complete runtime surfaces.
- Produces: frozen workspace semantic, exact technical, linked conversation, lifecycle, outage, lag/rebuild, and restricted-role trajectories with stable IDs and deterministic assertions.

- [x] **Step 1: Freeze case inputs and expected/forbidden facts before running the implementation.**

The case must use software release and deployment workflows, not the legacy
Bluebridge case. Include Chinese semantic paraphrase, mixed-language path/flag,
error code, model ID, same-scope distractor, cross-continuity distractor,
cross-tenant distractor, superseded text, and deleted text.

- [x] **Step 2: Write acceptance tests that materialize every fact through public runtime APIs and process projections through the worker.**

Direct inserts into vector documents, retrieval audit, or cursor tables are
forbidden except in explicit corruption/failure setup sections.

- [x] **Step 3: Require workspace MCP and linked-conversation Web Chat behavior.**

Tests assert:

```text
lexical default              unchanged
shadow delivered context     byte-identical to lexical
vector semantic recall       expected active fact present
cross-scope content          absent
proposed/superseded/deleted  absent
```

- [x] **Step 4: Require provider outage, lag, late completion, rebuild, RLS, and authority-fingerprint hard gates.**

The native restore acceptance must include schema 14, events, cursors, audits,
and vector row deletion/rebuild while preserving authority.

- [x] **Step 5: Run focused acceptance, full serial database, race, vet, tidy, and commit.**

```bash
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' \
  go test -p 1 -count=1 ./internal/runtime -run 'ProductionRetrievalAcceptance|OperationsAcceptance'
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' \
  go test -p 1 -count=1 ./...
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' \
  go test -race -p 1 -count=1 ./internal/runtime ./internal/webchat ./internal/mcpserver ./cmd/vermory
go vet ./...
go mod tidy
git diff --exit-code -- go.mod go.sum
git add runtime/cases/W09-production-retrieval-runtime \
  internal/runtime/retrieval_acceptance_test.go \
  internal/runtime/operations_acceptance_test.go internal/runtime/rls_migration_test.go
git commit -m "test: freeze production retrieval runtime cases"
```

### Task 6: Real SiliconFlow And Real Client Evidence

**Files:**
- Create: `docs/evidence/2026-07-14-production-retrieval-runtime.md`
- Create: `docs/evidence/snapshots/2026-07-14-production-retrieval-runtime.json`
- Modify: `docs/evaluation-matrix.md`
- Modify: `docs/superpowers/specs/2026-07-11-vermory-hypothesis-register.md`
- Modify: `README.md`
- Modify: `README.zh-CN.md`

**Interfaces:**
- Consumes: isolated release binary, dedicated PostgreSQL database, restricted runtime role, direct SiliconFlow embeddings, logged-in Grok CLI, frozen W09 cases.
- Produces: one real projection lifecycle, workspace MCP consumption/writeback, conversation consumption, outage/fallback, rebuild, RLS, and recovery evidence set without changing the default.

- [x] **Step 1: Build an isolated binary, create a dedicated database, migrate to schema 14, provision a non-owner runtime role, and record only safe versions, hashes, counts, and role boundaries.**

- [x] **Step 2: Seed W09 through runtime/operator APIs, run the fixed-tenant worker with direct SiliconFlow `BAAI/bge-m3`, and verify cursor current plus active-only row equality.**

The key is supplied through terminal-echo-disabled stdin into a transient
environment variable. It must never appear in history, files, arguments, or
artifacts.

- [x] **Step 3: Run one logged-in Grok MCP workspace task in explicit vector mode.**

Require Grok to call `prepare_context`, consume the Chinese semantic fact plus
exact technical facts, create and deterministically verify one artifact, and
call `commit_observation`. Validate the persisted delivery and proposed
writeback rather than trusting model self-report.

- [x] **Step 4: Run one real conversation turn and one shadow turn.**

The vector turn must consume the accepted linked-conversation fact. The shadow
turn must persist a context byte-identical to lexical while the audit contains
both lexical and vector IDs.

- [x] **Step 5: Stop the worker, commit a correction, prove cursor-lag fallback, catch up, force HTTP 503 fallback, then delete/rebuild vector rows and require result-ID equivalence plus unchanged authority fingerprint.**

- [x] **Step 6: Run restricted-role filter-omission and cross-tenant probes, native dump/restore, post-restore vector rebuild, and credential-shaped scans.**

- [x] **Step 7: Commit normalized JSON/Markdown evidence and update H-009 only to the state justified by W09.**

H-009 remains `testing` until a second independent retrieval batch and threshold
review. The evidence may state `production_path_integrated` but must not state
`accepted_default`.

```bash
git add docs/evidence/2026-07-14-production-retrieval-runtime.md \
  docs/evidence/snapshots/2026-07-14-production-retrieval-runtime.json \
  docs/evaluation-matrix.md docs/superpowers/specs/2026-07-11-vermory-hypothesis-register.md \
  README.md README.zh-CN.md
git commit -m "docs: record production retrieval runtime evidence"
```

### Task 7: Full Verification And Delivery

**Files:**
- Modify: `docs/superpowers/plans/2026-07-14-production-retrieval-runtime.md`
- Modify: Draft PR 1 body.

**Interfaces:**
- Consumes: all W09 implementation and evidence.
- Produces: green local/protected-CI gates, verified release artifact, clean commits, and an updated Draft PR while the overall Vermory goal remains active.

- [x] **Step 1: Run the complete serial PostgreSQL suite, selected runtime/new-package race suite, reality race, vet, tidy, module diff, and diff check.**

```bash
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' go test -p 1 -count=1 ./...
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' \
  go test -race -p 1 -count=1 ./internal/authn ./internal/runtime ./internal/webchat \
  ./internal/identitycli ./internal/operatorcli ./internal/mcpserver ./internal/provider \
  ./internal/memorybackend ./internal/retrievalablation ./cmd/vermory
go test -race -count=1 ./internal/reality
go vet ./...
go mod tidy
git diff --exit-code -- go.mod go.sum
git diff --check
```

- [x] **Step 2: Run Actionlint, GoReleaser check and four-platform snapshot/checksums, downloaded host archive execution, OpenClaw tests/typecheck/build/package, schema-14 replay, RLS/runtime-role checks, native backup/restore, and zero-match credential scan.**

- [x] **Step 3: Remove every dedicated database, temporary role, provider transcript containing headers, isolated HOME, downloaded artifact, local `dist/`, and temporary release host after normalized evidence is committed.**

- [x] **Step 4: Mark the checklist from fresh evidence, push `agent/grok-cli-runtime`, wait for protected CI, and independently verify the final artifact digest, four archive checksums/layouts, OpenClaw package, and darwin/arm64 execution.**

- [x] **Step 5: Append `Final Production Retrieval Runtime Delivery` to Draft PR 1 with the real result, preserved failures, exact non-claims, final run/job/artifact/digest, and confirmation that the PR remains Draft, `CLEAN`, `MERGEABLE`, and required `test=SUCCESS`.**

- [x] **Step 6: Keep the overall Vermory goal active. W09 does not complete the second independent retrieval batch, source authority ranking, embedding migration, scale/fault qualification, sealed evaluation, signing, or final release acceptance.**
