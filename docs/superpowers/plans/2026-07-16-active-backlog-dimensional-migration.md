# Active-Backlog Dimensional Migration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use
> `superpowers:executing-plans` to implement this plan inline, task by task.
> Steps use checkbox (`- [ ]`) syntax for tracking. Do not dispatch subagents.

**Goal:** Qualify a 2560-dimensional candidate projection class while the
1024-dimensional incumbent continues serving and both profiles consume an
active PostgreSQL event backlog through restart, deletion, reset, rebuild, and
a real direct-provider probe.

**Architecture:** Migration 16 keeps the existing `vector(1024)` table and
adds an isolated `halfvec(2560)` table. A closed projection-class enum maps each
supported profile to fixed SQL; no runtime string becomes a table name. One
opt-in W17 harness drives a dedicated PostgreSQL 18 cluster with deterministic
scale embeddings and a separate direct SiliconFlow probe.

**Tech Stack:** Go 1.26, PostgreSQL 18, pgvector 0.8.5, pgx v5, Goose, Cobra,
direct SiliconFlow OpenAI-compatible embeddings, existing Vermory runtime and
release tooling.

## Global Constraints

- PostgreSQL governed memories remain the only authority.
- Lexical remains the product default; `siliconflow-bge-m3-1024-v1` remains the
  active semantic profile.
- The candidate is exactly `siliconflow-qwen3-embedding-4b-2560-v3`, direct
  `https://api.siliconflow.cn/v1`, model `Qwen/Qwen3-Embedding-4B`, 2560
  dimensions, lifecycle `candidate`.
- No Redis, mem0, MemOS, Supermemory, NewAPI, arbitrary SQL identifiers,
  automatic promotion, or automatic rollback.
- Every production-code change follows red-green-refactor; the failing test is
  run and observed before implementation.
- Deterministic embeddings prove mechanics only. The formal run is incomplete
  until the direct 2560-dimensional provider probe succeeds.
- Failed formal attempts are retained and never overwritten by a passing run.
- No credentials, raw vectors, provider response bodies, private paths, DSNs,
  tokens, or passwords enter Git or GitHub.
- W17 completion does not complete the active overall Vermory goal.

**Provider correction:** The initially frozen `BAAI/bge-small-zh-v1.5` tuple
returned HTTP 400/provider code 20012 (`Model does not exist`) in a direct
preflight. The provider returned 1024 dimensions for Qwen3-Embedding-0.6B,
2560 for Qwen3-Embedding-4B, and 4096 for Qwen3-Embedding-8B. The plan now
freezes the available non-Pro 4B/2560 tuple. The failed 512 attempt and the
earlier pre-request zsh `status` wrapper error remain retained evidence.

**Index correction:** Fresh migration execution proved pgvector 0.8.5 rejects
HNSW indexes on `vector` columns above 2000 dimensions. Its documented
`halfvec` HNSW limit is 4000. W17 therefore uses physical class
`halfvec_2560`, `halfvec(2560)` storage, and `halfvec_cosine_ops`; all 2560
provider dimensions remain present and the precision change is explicit.

---

### Task 1: Freeze The W17 Case And Schema Contract

**Files:**
- Create: `runtime/cases/W17-active-backlog-dimensional-migration/README.md`
- Create: `runtime/cases/W17-active-backlog-dimensional-migration/case.json`
- Create: `internal/runtime/dimensional_migration_case_test.go`
- Create: `internal/runtime/retrieval_dimension_migration_test.go`

**Interfaces:**
- Consumes: schema 15 profile registry and existing W11/W12 case-loading
  conventions.
- Produces: `dimensionalMigrationCase`, `loadDimensionalMigrationCase`, and
  failing database assertions for migration 16.

- [x] **Step 1: Add the frozen case manifest and README.**

The manifest must encode:

```json
{
  "version": "1",
  "id": "W17-active-backlog-dimensional-migration",
  "profile_name": "active-backlog-dimension-v1",
  "tenant_count": 4,
  "continuities_per_tenant": 5,
  "records_per_continuity": 1000,
  "initial_active_count": 20000,
  "revision_count": 2000,
  "delete_count": 500,
  "new_fact_count": 500,
  "tail_event_count": 5000,
  "query_client_count": 16,
  "queries_per_client": 20,
  "snapshot_page_size": 250,
  "worker_batch_size": 128,
  "pool_max_connections": 48,
  "incumbent_profile_id": "siliconflow-bge-m3-1024-v1",
  "candidate_profile_id": "siliconflow-qwen3-embedding-4b-2560-v3",
  "real_provider_tenant_id": "w17-real-provider-tenant",
  "reference_hardware": {
    "os": "Darwin arm64",
    "cpu": "Apple M4 Pro",
    "memory_gib": 48,
    "storage": "local NVMe",
    "postgresql": "18.4",
    "pgvector": "0.8.5"
  },
  "calibrated_limits": {
    "authority_seed_seconds": 600,
    "incumbent_snapshot_seconds": 600,
    "candidate_snapshot_seconds": 600,
    "writer_seconds": 300,
    "tail_catchup_seconds": 300,
    "restart_recovery_seconds": 60,
    "query_p95_ms": 2000,
    "query_p99_ms": 5000,
    "database_size_gib": 8
  },
  "hard_gates": [
    "schema 16 isolates vector_1024 and halfvec_2560 projection classes",
    "authority writes do not wait for candidate embedding work",
    "incumbent vector queries continue while candidate backlog is active",
    "immediate restart commits no interrupted candidate vector or cursor",
    "the same incumbent and candidate pools recover after restart",
    "revisions deletions and new facts create exactly 5000 tail events",
    "both physical classes converge to the same 20000 eligible memory IDs",
    "superseded deleted redacted proposed and cross-scope rows remain absent",
    "candidate reset and rebuild leave incumbent and authority unchanged",
    "direct SiliconFlow projection and retrieval use exactly 2560 dimensions",
    "retrieval audits separate incumbent and candidate operations",
    "incumbent remains active default and candidate remains unpromoted"
  ]
}
```

- [x] **Step 2: Add case-identity and arithmetic tests.**

Require exact identity, hardware profile, profile IDs, 20,000 initial facts,
5,000 tail events, 320 queries, 12 hard gates, and this arithmetic:

```go
initial := manifest.TenantCount * manifest.ContinuitiesPerTenant * manifest.RecordsPerContinuity
tail := manifest.RevisionCount*2 + manifest.DeleteCount + manifest.NewFactCount
finalActive := initial - manifest.DeleteCount + manifest.NewFactCount
```

Assert `initial == 20000`, `tail == 5000`, and `finalActive == 20000`.

- [x] **Step 3: Add failing migration-16 assertions.**

The test must migrate a fresh database and require:

```text
schema version                         16
new profile                            siliconflow-qwen3-embedding-4b-2560-v3
profile model                          Qwen/Qwen3-Embedding-4B
profile dimensions                     2560
profile projection_class               halfvec_2560
profile lifecycle                      candidate
existing profile projection_class      vector_1024
table                                  memory_vector_documents_2560
embedding type                         halfvec(2560)
tenant-aware foreign keys              continuity and governed memory
profile foreign key                    memory_retrieval_profiles
RLS enabled and forced by runtime role tenant context
HNSW halfvec_cosine_ops index           present
```

Also assert PostgreSQL rejects a 1024-dimensional literal inserted into the
2560 table and rejects the 2560 profile ID in the 1024 table through the profile
class constraint introduced by migration 16.

- [x] **Step 4: Run the focused tests and observe RED.**

Run:

```bash
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' \
  go test -p 1 -count=1 ./internal/runtime \
  -run 'TestDimensionalMigrationCaseIsFrozen|TestRetrievalDimensionMigrationSchema'
```

Expected: the case test passes and the schema test fails because migration 16,
the 2560 table, and the candidate profile do not exist.

---

### Task 2: Add Migration 16 And Closed Profile Classes

**Files:**
- Create: `internal/store/postgres/migrations/00016_dimensional_projection_class.sql`
- Modify: `internal/runtime/retrieval_types.go`
- Modify: `internal/runtime/retrieval_store_test.go`
- Modify: `internal/runtime/retrieval_dimension_migration_test.go`

**Interfaces:**
- Consumes: `RetrievalProfileSpec`, `RetrievalProfile.Validate`, migration 15
  registry, and the schema tests from Task 1.
- Produces: `ProjectionClass`, `ProjectionClass1024`, `ProjectionClass2560`,
  `DimensionalMigrationRetrievalProfileID`, and migration 16.

- [x] **Step 1: Add failing profile-validation tests.**

Require:

```go
const DimensionalMigrationRetrievalProfileID = "siliconflow-qwen3-embedding-4b-2560-v3"

spec, ok := SupportedRetrievalProfile(DimensionalMigrationRetrievalProfileID)
// ok, direct SiliconFlow, Qwen/Qwen3-Embedding-4B, 2560,
// ProjectionClass2560, candidate
```

Reject a candidate with the wrong base URL, model, dimensions, projection
class, or profile ID. Require both existing profiles to report
`ProjectionClass1024`.

- [x] **Step 2: Run the profile tests and observe RED.**

Run:

```bash
go test -count=1 ./internal/runtime \
  -run 'TestSupportedRetrievalProfiles|TestRetrievalProfileValidation'
```

Expected: compile failure because the new constants and field do not exist.

- [x] **Step 3: Implement the closed projection-class type.**

Add:

```go
type ProjectionClass string

const (
    ProjectionClass1024 ProjectionClass = "vector_1024"
    ProjectionClass2560 ProjectionClass = "halfvec_2560"
)
```

Add `ProjectionClass ProjectionClass` to both profile structs. Keep the
supported-profile map compile-time and immutable. Validation compares the
entire tuple to the supported spec.

- [x] **Step 4: Implement migration 16.**

The Up migration must:

1. add `projection_class` to `memory_retrieval_profiles`;
2. assign both existing rows `vector_1024`;
3. make the column non-null with a two-value check;
4. register the 2560 candidate;
5. create `memory_vector_documents_2560` with exact FKs, RLS, scope index, and
   HNSW cosine index;
6. add constraints that bind the incumbent table to `vector_1024` and the new
   table to `halfvec_2560` through composite profile-class foreign keys;
7. revoke PUBLIC privileges.

The profile registry needs a unique `(profile_id, projection_class)` key so
both typed tables can reference the immutable class. The Down migration must
remove only candidate audit/cursor/projection rows and the candidate registry
row before restoring schema 15.

- [x] **Step 5: Run migration Up, Down, and profile tests.**

Run:

```bash
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' \
  go test -p 1 -count=1 ./internal/runtime \
  -run 'TestDimensionalMigrationCaseIsFrozen|TestRetrievalDimensionMigrationSchema|TestRetrievalMigrationUpDown|TestSupportedRetrievalProfiles|TestRetrievalProfileValidation'
```

Expected: PASS, including wrong-dimension and wrong-class database rejection.

- [x] **Step 6: Commit the frozen schema boundary.**

```bash
git add runtime/cases/W17-active-backlog-dimensional-migration \
  internal/store/postgres/migrations/00016_dimensional_projection_class.sql \
  internal/runtime/dimensional_migration_case_test.go \
  internal/runtime/retrieval_dimension_migration_test.go \
  internal/runtime/retrieval_types.go internal/runtime/retrieval_store_test.go
git commit -m "feat: add dimensional projection classes"
```

---

### Task 3: Route Store, Worker, And Coordinator By Physical Class

**Files:**
- Modify: `internal/runtime/retrieval_store.go`
- Modify: `internal/runtime/retrieval_store_test.go`
- Modify: `internal/runtime/retrieval_worker.go`
- Modify: `internal/runtime/retrieval_worker_test.go`
- Modify: `internal/runtime/retrieval_coordinator.go`
- Modify: `internal/runtime/retrieval_coordinator_test.go`
- Modify: `internal/runtime/retrieval_profile_migration_test.go`

**Interfaces:**
- Consumes: Task 2 profile classes and existing status/reset/search/worker
  contracts.
- Produces: class-specific fixed SQL for count, reset, search, upsert, delete,
  snapshot rebuild, and authority-change handling.

- [x] **Step 1: Add failing store tests for class isolation.**

Seed one tenant and one active memory. Insert deterministic 1024 and 2560
vectors through the real store/worker paths. Require:

- each profile status counts only its physical table;
- 1024 search reads only the incumbent table;
- 2560 search reads only the candidate table;
- resetting the candidate clears only 2560 rows and its cursor;
- resetting the incumbent clears only 1024 rows and its cursor;
- the same operation ID under two profile-specific retrieval fingerprints is
  not treated as the same audit request.

- [x] **Step 2: Add failing worker tests for 2560 upsert, delete, rebuild, and races.**

Use deterministic embedders returning exact dimensions. Require:

- candidate event processing writes one 2560 row and no 1024 row;
- candidate deletion removes the 2560 row;
- candidate snapshot rebuild populates current authority and advances only the
  candidate cursor;
- a 1024 result from the candidate embedder records
  `embedding_dimension_mismatch` and advances no cursor;
- an authority revision or deletion during candidate embedding returns
  `authority_changed` and leaves no stale 2560 row;
- incumbent and candidate advisory locks are independent for the same tenant.

- [x] **Step 3: Run the focused tests and observe RED.**

Run:

```bash
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' \
  go test -p 1 -count=1 ./internal/runtime \
  -run 'Test.*ProjectionClass|Test.*Dimensional.*Worker|Test.*Candidate.*Reset'
```

Expected: failure because all SQL still targets `memory_vector_documents`.

- [x] **Step 4: Add a private fixed SQL selector.**

Use a closed helper that returns predeclared SQL strings or a private struct of
queries for `ProjectionClass1024` and `ProjectionClass2560`. It must return an
error for any unknown class. Do not return a table name for interpolation.

The selected query set must cover:

```text
count
reset delete
search
event delete
event upsert
snapshot clear
snapshot upsert
```

- [x] **Step 5: Refactor store and worker operations to the selected query set.**

Keep lifecycle, tenant, continuity, content hash, `updated_at`, advisory lock,
cursor, and audit semantics unchanged. The only behavioral change is selecting
the qualified physical vector class.

- [x] **Step 6: Run focused, race, and existing profile migration tests.**

Run:

```bash
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' \
  go test -p 1 -count=1 ./internal/runtime \
  -run 'Test.*Projection|Test.*RetrievalCoordinator|TestLiveRetrievalProfileMigration'

VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' \
  go test -race -p 1 -count=1 ./internal/runtime \
  -run 'Test.*ProjectionClass|Test.*Dimensional.*Worker'
```

Expected: PASS; the live provider test may skip unless its explicit environment
is present.

- [x] **Step 7: Commit the runtime routing.**

```bash
git add internal/runtime/retrieval_store.go \
  internal/runtime/retrieval_store_test.go \
  internal/runtime/retrieval_worker.go \
  internal/runtime/retrieval_worker_test.go \
  internal/runtime/retrieval_coordinator.go \
  internal/runtime/retrieval_coordinator_test.go \
  internal/runtime/retrieval_profile_migration_test.go
git commit -m "feat: route dimensional vector projections"
```

---

### Task 4: Extend Runtime Role, Recovery, Reset, And CLI Contracts

**Files:**
- Modify: `internal/authn/provision.go`
- Modify: `internal/authn/postgres_test.go`
- Modify: `internal/runtime/postgres_store.go`
- Modify: `internal/runtime/rls_migration_test.go`
- Modify: `internal/runtime/operations_acceptance_test.go`
- Modify: `internal/runtime/retrieval_migration_test.go`
- Modify: `internal/operationsprofile/ha_failover_profile_test.go`
- Modify: `cmd/vermory/retrieval_runtime.go`
- Modify: `cmd/vermory/retrieval_runtime_test.go`

**Interfaces:**
- Consumes: schema 16 and Task 3 class routing.
- Produces: restricted runtime access to the 2560 table, schema-16 reset and
  recovery inventories, and explicit candidate CLI validation.

- [x] **Step 1: Add failing role, reset, recovery, and CLI tests.**

Require:

- restricted runtime role has tenant-scoped CRUD on
  `memory_vector_documents_2560` and does not own it;
- a tenant context cannot see another tenant's 2560 rows even when the
  application query omits a tenant predicate;
- `ResetForTest` truncates both projection tables;
- operations migration replay reports schema 16;
- logical dump/restore preserves both profile registry classes and 2560 rows;
- projection reset/rebuild can reconstruct 2560 rows from governed authority;
- CLI accepts the exact candidate tuple and rejects a mismatched model,
  dimension, class, or base URL without printing the key;
- W16's opt-in harness expects current schema 16 when rerun, without rewriting
  its immutable schema-15 evidence snapshot.

- [x] **Step 2: Run focused tests and observe RED.**

Run:

```bash
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' \
  go test -p 1 -count=1 ./internal/authn ./internal/runtime ./cmd/vermory \
  -run 'Test.*RuntimeRole|TestOperationsRecovery|Test.*Dimensional|Test.*RetrievalRuntime'
```

Expected: failures for missing 2560 privileges, reset inventory, schema version,
recovery inventory, and candidate CLI tuple.

- [x] **Step 3: Extend served-table and reset inventories.**

Add `memory_vector_documents_2560` to `authn.servedTables`, runtime validation,
test reset, backup authority inventory, restore assertions, and RLS inventory.
Do not grant runtime access to `memory_retrieval_profiles` or any legacy
ungoverned table.

- [x] **Step 4: Make CLI profile resolution explicit.**

Add a helper that resolves a supported profile ID to its frozen tuple. For
worker, snapshot, status, reset, and semantic retrieval commands, a candidate
profile must not inherit the incumbent model or dimension silently. Explicit
flags may confirm the tuple but cannot mutate it.

- [x] **Step 5: Run database, RLS, role, CLI, recovery, and full serial tests.**

Run:

```bash
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' \
  go test -p 1 -count=1 ./internal/authn ./internal/runtime ./cmd/vermory

VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' \
  go test -p 1 -count=1 ./...
```

Expected: PASS with profile tests skipped only when their explicit opt-in
environment is absent.

- [x] **Step 6: Commit the deployment boundary.**

```bash
git add internal/authn/provision.go internal/authn/postgres_test.go \
  internal/runtime/postgres_store.go internal/runtime/rls_migration_test.go \
  internal/runtime/operations_acceptance_test.go \
  internal/runtime/retrieval_migration_test.go \
  internal/operationsprofile/ha_failover_profile_test.go \
  cmd/vermory/retrieval_runtime.go cmd/vermory/retrieval_runtime_test.go
git commit -m "feat: operate dimensional projection classes"
```

---

### Task 5: Build The Deterministic W17 Harness And Report

**Files:**
- Create: `internal/runtime/dimensional_migration_report.go`
- Create: `internal/runtime/dimensional_migration_report_test.go`
- Create: `internal/runtime/dimensional_migration_profile_helpers_test.go`
- Create: `internal/runtime/dimensional_migration_profile_test.go`

**Interfaces:**
- Consumes: frozen W17 manifest, dedicated PostgreSQL helpers, runtime
  observation/governance transactions, both projection classes, and
  coordinator audit behavior.
- Produces: deterministic miniature coverage, opt-in formal orchestration,
  atomic JSON/Markdown reports, attempt history, and replay protection.

- [x] **Step 1: Add failing report tests.**

Require normalized JSON and Markdown to include:

```text
run and request fingerprints
case and implementation hashes
environment versions
profile registry and physical classes
authority event lexical vector and cursor counts
snapshot watermarks and skipped-changed counts
writer timing and embedding-block proof
incumbent query modes latency and outage result
restart interruption and same-pool recovery
candidate reset and rebuild equivalence
real provider dimensions and request hashes
chronological preserved failures
12 hard-gate booleans
explicit non-claims
```

Require atomic writes, byte-identical completed-run replay, and conflicting
run-ID rejection.

- [x] **Step 2: Run report tests and observe RED.**

Run:

```bash
go test -count=1 ./internal/runtime -run 'TestDimensionalMigrationReport'
```

Expected: compile failure because the report types do not exist.

- [x] **Step 3: Implement deterministic report serialization.**

Use stable field ordering, sorted tenant/profile entries, RFC3339 timestamps,
SHA-256 fingerprints, and temp-file-plus-rename writes. Never serialize DSNs,
API keys, raw vectors, or provider bodies.

- [x] **Step 4: Add a miniature end-to-end profile test.**

The default suite uses the shared test database with a reduced manifest:

```text
2 tenants
2 continuities per tenant
10 initial facts per continuity
8 revisions
4 deletions
4 new facts
24 tail events
4 query clients
4 queries per client
```

It must execute incumbent snapshot, blocked candidate snapshot, active writer,
incumbent queries, candidate tail convergence, class-equivalence assertions,
candidate reset, and candidate rebuild. It does not start or stop PostgreSQL
and does not call a real provider.

- [x] **Step 5: Add the opt-in dedicated-cluster profile.**

Run only when:

```text
VERMORY_DIMENSIONAL_MIGRATION_PROFILE=1
VERMORY_POSTGRES_BIN_DIR=/path/to/postgresql-18/bin
VERMORY_DIMENSIONAL_MIGRATION_ROOT=/dedicated/root
VERMORY_DIMENSIONAL_MIGRATION_RUN_ID=<immutable-run-id>
VERMORY_LIVE_EMBEDDING_API_KEY=<process-only-secret>
```

Reuse the W11 dedicated-cluster safety rules: absolute dedicated root,
loopback-only TCP, `unix_socket_directories=''`, PostgreSQL 18 tool matching,
test-owned process cleanup, dynamic ports, and retained logs. Do not modify the
Homebrew service or shared test database.

- [x] **Step 6: Implement the frozen workload and hard gates.**

The formal harness must:

1. seed 20,000 initial facts and build lexical plus incumbent vectors;
2. capture candidate watermarks before writers begin;
3. block candidate embedding while 5,000 tail events are committed;
4. run 320 incumbent queries during candidate lag;
5. stop PostgreSQL `immediate` during one candidate embedding;
6. prove zero partial row and zero interrupted cursor advance;
7. restart and reuse both pools;
8. drain both profile cursors to zero lag;
9. compare the exact 20,000 eligible memory-ID sets and content hashes;
10. reset and rebuild one tenant's candidate rows without incumbent drift;
11. perform the direct 2560-dimensional provider projection/query probe;
12. write the completed normalized report and attempt history.

- [x] **Step 7: Run report, miniature, race, and full serial tests.**

Run:

```bash
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' \
  go test -p 1 -count=1 ./internal/runtime \
  -run 'TestDimensionalMigrationReport|TestDimensionalMigrationHarnessMiniature'

VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' \
  go test -race -p 1 -count=1 ./internal/runtime \
  -run 'TestDimensionalMigrationHarnessMiniature|Test.*ProjectionClass'

VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' \
  go test -p 1 -count=1 ./...
```

Expected: PASS; the formal profile remains skipped without all explicit opt-in
variables.

- [x] **Step 8: Commit the harness.**

```bash
git add internal/runtime/dimensional_migration_report.go \
  internal/runtime/dimensional_migration_report_test.go \
  internal/runtime/dimensional_migration_profile_helpers_test.go \
  internal/runtime/dimensional_migration_profile_test.go
git commit -m "test: qualify active backlog dimensional migration"
```

---

### Task 6: Run The Formal Profile And Commit Evidence

**Files:**
- Create: `docs/evidence/2026-07-16-active-backlog-dimensional-migration.md`
- Create: `docs/evidence/snapshots/2026-07-16-active-backlog-dimensional-migration.json`
- Modify: `docs/evaluation-matrix.md`
- Modify: `docs/superpowers/specs/2026-07-11-vermory-hypothesis-register.md`
- Modify: `README.md`
- Modify: `README.zh-CN.md`

**Interfaces:**
- Consumes: Task 5 formal harness and process-only provider credential.
- Produces: one immutable W17 run with normalized evidence and explicit
  non-claims.

- [x] **Step 1: Verify formal-run prerequisites without printing secrets.**

Check PostgreSQL 18 binaries, pgvector 0.8.5, available disk space, an empty
dedicated root under `/Volumes/JSData/ComputerScience/Mac`, and only whether the
provider key is present. Do not print the key or an authenticated request.

- [x] **Step 2: Run the formal profile once from a clean root.**

Use an immutable run ID:

```bash
VERMORY_DIMENSIONAL_MIGRATION_PROFILE=1 \
VERMORY_POSTGRES_BIN_DIR='<postgresql-18-bin>' \
VERMORY_DIMENSIONAL_MIGRATION_ROOT='<dedicated-root>' \
VERMORY_DIMENSIONAL_MIGRATION_RUN_ID='<run-id>' \
VERMORY_LIVE_EMBEDDING_API_KEY='<process-only-secret>' \
  go test -p 1 -count=1 -v ./internal/runtime \
  -run '^TestActiveBacklogDimensionalMigrationProfile$'
```

Expected: PASS only if all 12 gates and the real 2560-dimensional probe pass.
If it fails, retain the root and attempt report, fix the implementation, and
use a new run ID.

- [x] **Step 3: Replay the completed run ID.**

Run the same command again. Require byte-identical JSON and Markdown, no live
PostgreSQL process, no provider request, and a bounded replay duration.

- [x] **Step 4: Independently inspect evidence.**

Verify:

```text
report hashes match files
case hash matches committed case
implementation revision matches the executed binary
all counts and arithmetic match the manifest
both physical ID sets and authority hashes match
all cursors have zero lag
candidate reset isolation is true
provider dimensions equal 2560
incumbent remains active and candidate remains candidate
every failed attempt is listed
no secret-shaped value appears
```

- [x] **Step 5: Commit normalized evidence and product boundaries.**

The evidence document must distinguish deterministic scale mechanics from the
two-request direct-provider probe. Update H-011 and H-012 only for what W17
actually proves. Keep long-duration retention, sealed evaluation, cross-host
HA, signing, and final release acceptance open.

```bash
git add docs/evidence/2026-07-16-active-backlog-dimensional-migration.md \
  docs/evidence/snapshots/2026-07-16-active-backlog-dimensional-migration.json \
  docs/evaluation-matrix.md \
  docs/superpowers/specs/2026-07-11-vermory-hypothesis-register.md \
  README.md README.zh-CN.md
git commit -m "docs: record dimensional migration evidence"
```

---

### Task 7: Close Local And Protected Delivery Gates

**Files:**
- Modify: `docs/superpowers/plans/2026-07-16-active-backlog-dimensional-migration.md`
- Modify: Draft PR 1 body only after the final protected artifact is verified.

**Interfaces:**
- Consumes: all W17 commits and evidence.
- Produces: checked plan, clean pushed branch, protected CI success, independently
  verified artifact chain, and a Draft PR update. The overall goal stays active.

- [x] **Step 1: Run the complete local release gates.**

```bash
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' \
  go test -p 1 -count=1 ./...

VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' \
  go test -race -p 1 -count=1 \
  ./internal/runtime ./internal/authn ./internal/webchat ./cmd/vermory ./internal/reality

go vet ./...
go mod tidy
git diff --exit-code -- go.mod go.sum
go build -trimpath ./cmd/vermory
pnpm -C integrations/openclaw check
pnpm -C integrations/openclaw pack --dry-run
go run github.com/goreleaser/goreleaser/v2@v2.17.0 \
  check --config .goreleaser.yaml
go run github.com/goreleaser/goreleaser/v2@v2.17.0 \
  release --snapshot --clean --skip=publish --config .goreleaser.yaml
git diff --check
```

Remove only the untracked local binary generated by the explicit build. Do not
delete evidence attempts, PostgreSQL logs, or unrelated user files.

- [x] **Step 2: Mark every completed plan checkbox and commit the checklist.**

```bash
git add docs/superpowers/plans/2026-07-16-active-backlog-dimensional-migration.md
git commit -m "docs: close dimensional migration qualification"
```

- [x] **Step 3: Push the branch and wait for protected CI.**

```bash
git push origin agent/grok-cli-runtime
gh run watch --exit-status
```

Require the protected `test` check to pass on the exact final checklist head.
Do not reuse a prior artifact.

- [x] **Step 4: Independently verify the final GitHub artifact.**

Download the raw Actions artifact ZIP and verify:

```text
API byte size equals downloaded byte size
API SHA-256 digest equals transport ZIP SHA-256
all four Go archive checksums pass
each Go archive has exactly LICENSE, README.md, README.zh-CN.md, vermory
OpenClaw tgz has exactly the expected 12 entries
Darwin arm64 runs version and database --help
all binaries have expected GOOS/GOARCH, CGO_ENABLED=0, -trimpath=true,
vcs.modified=false, and the final synthetic merge revision
synthetic merge signature is valid
synthetic merge second parent is the final checklist head
```

- [x] **Step 5: Verify final repository publication state.**

Require:

```text
PR OPEN
PR Draft
PR CLEAN
PR MERGEABLE
required test SUCCESS
remote head equals local final checklist head
tags 0
Releases 0
worktree clean
```

- [x] **Step 6: Append one W17 delivery section to Draft PR 1.**

Record only final immutable IDs, formal run ID and report hashes, CI run/job,
artifact ID/name/size/digest, synthetic merge signature and second parent, PR
state, zero tags/releases, and the remaining active-goal boundaries. Do not
duplicate an existing W17 section and do not create a tag or Release.

- [x] **Step 7: Keep the overall Vermory goal active.**

W17 closes only active-backlog dimensional migration for the named
1024-to-2560 profile. Remaining work includes genuine external sealed
evaluation, long-duration retention and pruning, cross-host HA evidence,
artifact signing, and final release acceptance. Do not call
`update_goal(status="complete")`.
