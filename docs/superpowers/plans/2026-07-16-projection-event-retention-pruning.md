# Projection Event Retention And Pruning Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use
> `superpowers:executing-plans` to implement this plan inline, task by task.
> Steps use checkbox (`- [ ]`) syntax for tracking. Do not dispatch subagents.

**Goal:** Bound PostgreSQL projection-event retention without allowing a new,
reset, failed, or lagging semantic profile to become falsely current after
history has been pruned.

**Architecture:** Migration 17 introduces a tenant retention floor and an
immutable prune receipt. Incremental workers, status, reset, and semantic
retrieval enforce that floor through an explicit `rebuild_required` state;
only `RebuildCurrent` can recover a projection whose retained tail is
insufficient. An operator-only, transactionally audited prune API computes the
minimum cursor/cutoff/tail bound and never mutates PostgreSQL authority.

**Tech Stack:** Go 1.26, PostgreSQL 18, pgvector 0.8.5, pgx v5, Goose, Cobra,
direct SiliconFlow OpenAI-compatible embeddings, existing Vermory runtime,
formal-profile, evidence, CI, and release tooling.

## Global Constraints

- PostgreSQL governed memories remain the only authority.
- `memory_projection_events` is a disposable delivery log, not memory truth.
- W18 does not delete governed memories, observations, supersession history,
  conversation turns, bridge records, retrieval audits, source-governance
  audits, or user-visible evidence.
- Authorized forgetting remains separate from event pruning.
- Lexical remains the default. Semantic profiles remain explicit opt-in.
- No Redis, second queue, hidden scheduler, table partitioning, NewAPI,
  automatic profile promotion, or automatic authority retention is added.
- Every prune is explicit, tenant-scoped, idempotent, auditable, and performed
  with an operator/admin database connection.
- Runtime workers may read the retention floor but may not write retention or
  prune-run state.
- A profile below the floor must stop with
  `projection_rebuild_required` before any embedding call or cursor advance.
- `RebuildCurrent` is the only recovery path from `rebuild_required`.
- Every production-code change follows red-green-refactor. Each test is run
  and observed failing before the corresponding implementation.
- Deterministic embeddings prove mechanics only. The W18 formal run is not
  complete until the direct `BAAI/bge-m3` probe succeeds.
- Failed formal attempts and injected failures are retained. No threshold may
  be changed merely to hide a failure.
- Credentials, DSNs, raw vectors, provider response bodies, private paths,
  tokens, and passwords must not enter Git, reports, logs, or GitHub.
- W18 completion does not complete the active overall Vermory goal.

---

### Task 1: Freeze The W18 Case And RED Schema Contract

**Files:**
- Create: `runtime/cases/W18-projection-event-retention-pruning/README.md`
- Create: `runtime/cases/W18-projection-event-retention-pruning/case.json`
- Create: `internal/runtime/projection_retention_case_test.go`
- Create: `internal/runtime/projection_retention_migration_test.go`

**Interfaces:**
- Consumes: schema 16, W17 case-loading conventions, and the frozen W18 design.
- Produces: `projectionRetentionCase`, `loadProjectionRetentionCase`, and RED
  assertions for migration 17.

- [x] **Step 1: Add the frozen case manifest and README.**

The manifest must encode this exact identity and arithmetic:

```json
{
  "version": "1",
  "id": "W18-projection-event-retention-pruning",
  "profile_name": "projection-event-retention-v1",
  "tenant_count": 4,
  "continuities_per_tenant": 5,
  "records_per_continuity": 1000,
  "initial_current_facts": 20000,
  "accelerated_epochs": 24,
  "revision_count": 72000,
  "delete_count": 4800,
  "new_fact_count": 4800,
  "tail_event_count": 153600,
  "total_generated_events": 173600,
  "final_current_facts": 20000,
  "retained_tail_per_tenant": 1000,
  "query_client_count": 16,
  "queries_per_client": 20,
  "worker_batch_size": 256,
  "snapshot_page_size": 250,
  "pool_max_connections": 48,
  "incumbent_profile_id": "siliconflow-bge-m3-1024-v1",
  "dimensional_profile_id": "siliconflow-qwen3-embedding-4b-2560-v3",
  "future_profile_id": "siliconflow-bge-large-zh-1024-v2",
  "real_provider_tenant_id": "w18-real-provider-tenant",
  "hard_gates": [
    "schema 17 creates tenant-isolated retention and prune audit state",
    "runtime workers can read but cannot mutate retention control tables",
    "no prune passes the slowest incremental cursor",
    "authority writes and active-profile queries continue during prune attempts",
    "interrupted prune deletion floor and audit changes roll back together",
    "the same pool recovers after PostgreSQL restart",
    "event retention reaches the calibrated bound after catch-up",
    "retention floor is monotonic and idempotent replay is byte-stable",
    "new subscribers below the floor rebuild with zero embedding work",
    "reset after pruning requires rebuild and leaves other profiles unchanged",
    "rebuilds match authority and deleted memories remain absent",
    "cross-tenant pruning and receipt access are blocked",
    "direct provider projection and query succeed after pruning",
    "lexical and the incumbent remain default with no promotion"
  ]
}
```

The README must state that 24 accelerated epochs are workload compression,
not 24 months of wall-clock uptime.

- [x] **Step 2: Add case identity and arithmetic tests.**

Require:

```go
initial := manifest.TenantCount * manifest.ContinuitiesPerTenant * manifest.RecordsPerContinuity
tail := manifest.RevisionCount*2 + manifest.DeleteCount + manifest.NewFactCount
finalCurrent := initial - manifest.DeleteCount + manifest.NewFactCount
queries := manifest.QueryClientCount * manifest.QueriesPerClient
```

Assert `initial == 20000`, `tail == 153600`,
`manifest.TotalGeneratedEvents == initial+tail == 173600`,
`finalCurrent == 20000`, `queries == 320`, retained tail is exactly 1,000 per
tenant, and there are exactly 14 hard gates. Reject unknown manifest fields.

- [x] **Step 3: Add failing migration-17 assertions.**

On a fresh migrated database require:

```text
schema version                                      17
memory_projection_retention                         present
memory_projection_prune_runs                        present
retention primary key                               tenant_id
prune unique key                                    tenant_id + operation_id
cursor status check                                 includes rebuild_required
retention floor                                     non-negative bigint
retain_tail_events                                  non-negative integer
result check                                        pruned or noop
RLS                                                 enabled on both new tables
tenant policies                                     USING and WITH CHECK
PUBLIC privileges                                   revoked
```

Also require migration 17 Down/Up replay to preserve schema-16 behavior and
reject invalid cursor states, negative floors, negative tail counts, invalid
fingerprints, and invalid prune results.

- [x] **Step 4: Run focused tests and observe RED.**

Run:

```bash
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_w18_test?host=/tmp' \
  go test -p 1 -count=1 ./internal/runtime \
  -run 'TestProjectionRetentionCaseIsFrozen|TestProjectionRetentionSchema|TestRetrievalMigrationUpDown'
```

Expected: the case test passes and schema assertions fail because migration 17
and both retention tables do not exist.

- [x] **Step 5: Commit the frozen case boundary.**

```bash
git add runtime/cases/W18-projection-event-retention-pruning \
  internal/runtime/projection_retention_case_test.go \
  internal/runtime/projection_retention_migration_test.go
git commit -m "test: freeze projection retention qualification"
```

---

### Task 2: Add Migration 17 And Retention Domain Types

**Files:**
- Create: `internal/store/postgres/migrations/00017_projection_event_retention.sql`
- Create: `internal/runtime/projection_retention.go`
- Create: `internal/runtime/projection_retention_test.go`
- Modify: `internal/runtime/retrieval_types.go`
- Modify: `internal/runtime/postgres_store.go`
- Modify: `internal/runtime/operations_acceptance_test.go`

**Interfaces:**
- Consumes: schema RED tests from Task 1 and existing `Store` tenant context.
- Produces:

```go
const ProjectionStatusRebuildRequired = "rebuild_required"
const ProjectionFailureRebuildRequired = "projection_rebuild_required"

type ProjectionRetention struct {
    TenantID            string     `json:"tenant_id"`
    PrunedThroughEventID int64      `json:"pruned_through_event_id"`
    LastPrunedAt        *time.Time `json:"last_pruned_at,omitempty"`
	UpdatedAt           *time.Time `json:"updated_at,omitempty"`
}

func (s *Store) ProjectionRetention(ctx context.Context, tenantID string) (ProjectionRetention, error)
```

- [x] **Step 1: Add failing type and default-floor tests.**

Require a tenant with no row to return an effective floor of zero without
creating state. Require tenant validation and JSON fields to be stable. Add a
status serialization test for `pruned_through_event_id` and
`rebuild_required`.

- [x] **Step 2: Run the type tests and observe RED.**

```bash
go test -count=1 ./internal/runtime \
  -run 'TestProjectionRetentionDefaultsToZero|TestProjectionStatusReportsRetentionFloor'
```

Expected: compile failure because the types and method do not exist.

- [x] **Step 3: Implement migration 17.**

The Up migration must create the two tables exactly as frozen, extend the
cursor status constraint to include `rebuild_required`, create tenant and
created-time indexes for prune receipts, enable RLS, create tenant policies,
and revoke PUBLIC privileges. The Down migration must reject downgrade if any
cursor remains `rebuild_required`, then restore the schema-16 cursor check and
drop only W18 tables and policies.

- [x] **Step 4: Implement retention reads and status fields.**

Add `PrunedThroughEventID int64` and `RebuildRequired bool` to
`ProjectionStatus`. `RetrievalProjectionStatus` reads the effective floor in
the same tenant context and sets `RebuildRequired` only when status equals
`rebuild_required`.

- [x] **Step 5: Update reset/test cleanup and schema-version assertions.**

`ResetForTest` must truncate `memory_projection_prune_runs` and
`memory_projection_retention` before event/cursor tables. All release and
operations acceptance checks that intentionally assert latest schema must
expect 17.

- [x] **Step 6: Run migration and focused runtime tests.**

```bash
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_w18_test?host=/tmp' \
  go test -p 1 -count=1 ./internal/runtime \
  -run 'TestProjectionRetentionCaseIsFrozen|TestProjectionRetentionSchema|TestProjectionRetentionDefaultsToZero|TestProjectionStatusReportsRetentionFloor|TestRetrievalMigrationUpDown|TestOperationsAcceptance'
```

Expected: PASS.

- [x] **Step 7: Commit schema 17.**

```bash
git add internal/store/postgres/migrations/00017_projection_event_retention.sql \
  internal/runtime/projection_retention.go \
  internal/runtime/projection_retention_test.go \
  internal/runtime/retrieval_types.go internal/runtime/postgres_store.go \
  internal/runtime/operations_acceptance_test.go
git commit -m "feat: add projection retention floor"
```

---

### Task 3: Enforce Rebuild-Required Worker, Retrieval, Reset, And Rebuild Semantics

**Files:**
- Modify: `internal/runtime/retrieval_worker.go`
- Modify: `internal/runtime/retrieval_worker_test.go`
- Modify: `internal/runtime/retrieval_store.go`
- Modify: `internal/runtime/retrieval_store_test.go`
- Modify: `internal/runtime/retrieval_coordinator.go`
- Modify: `internal/runtime/retrieval_coordinator_test.go`
- Modify: `internal/runtime/retrieval_dimension_routing_test.go`
- Modify: `internal/runtime/dimensional_migration_profile_test.go`
- Modify: `internal/runtime/operations_acceptance_test.go`

**Interfaces:**
- Consumes: Task 2 floor and status constants.
- Produces:

```go
func ensureProjectionCursor(
    ctx context.Context,
    connection *pgxpool.Conn,
    tenantID string,
    profileID string,
) (rebuildRequired bool, err error)
```

and floor-aware reset/status/retrieval behavior.

- [x] **Step 1: Add RED worker tests.**

Cover these independent behaviors:

1. A cursor below the floor becomes `rebuild_required`, returns
   `projection_rebuild_required`, performs zero embedding calls, and does not
   advance.
2. A previously unseen profile after pruning is created at the floor in
   `rebuild_required`, not at zero/idle.
3. A cursor at or above the floor can process retained events normally.
4. A `failed` cursor at or above the floor remains retryable and retention
   blocking.
5. `RebuildCurrent` succeeds from `rebuild_required`, advances to the captured
   watermark, clears the failure code, and then drains newer retained events.

- [x] **Step 2: Run worker tests and observe RED.**

```bash
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_w18_test?host=/tmp' \
  go test -p 1 -count=1 ./internal/runtime \
  -run 'TestProjectionWorker.*Retention|TestProjectionWorker.*RebuildRequired'
```

Expected: failures because workers ignore the retention floor.

- [x] **Step 3: Implement worker start enforcement.**

Under the existing tenant/profile advisory lock, read the floor before setting
`running`. If the cursor is absent after pruning, insert it at the floor with
`rebuild_required`. If the cursor is below the floor, update status and failure
code atomically and return without calling the embedder. Never let ordinary
`RunOnce` clear `rebuild_required`.

- [x] **Step 4: Add RED retrieval and reset tests.**

Require vector mode to degrade to lexical with these exact failure codes:

```text
running            projection_not_current
failed             projection_not_current
lag > 0            projection_lag
rebuild_required   projection_rebuild_required
cursor < floor     projection_rebuild_required
```

Require reset after pruning to clear only the selected physical projection and
set cursor to floor/`rebuild_required`; authority, lexical state, other profile
rows, other profile cursor, and another tenant must remain byte-identical.

- [x] **Step 5: Run retrieval/reset tests and observe RED.**

```bash
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_w18_test?host=/tmp' \
  go test -p 1 -count=1 ./internal/runtime \
  -run 'TestRetrievalCoordinator.*ProjectionState|TestResetVectorProjection.*Retention'
```

Expected: failures because currentness only checks lag and reset returns to
zero/idle.

- [x] **Step 6: Implement floor-aware currentness and reset.**

Semantic currentness is exactly:

```go
current := status.Status == "idle" &&
    status.LastEventID >= status.PrunedThroughEventID &&
    status.Lag == 0
```

Reset reads the floor in the same transaction that clears vectors and upserts
the cursor. It writes `last_event_id=floor`, `status=rebuild_required`, and
`last_error_code=projection_rebuild_required`.

- [x] **Step 7: Run focused and regression tests.**

```bash
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_w18_test?host=/tmp' \
  go test -p 1 -count=1 ./internal/runtime \
  -run 'TestProjectionWorker|TestRetrievalCoordinator|TestResetVectorProjection|TestRetrievalDimension|TestOperationsAcceptance'
```

Expected: PASS. Existing reset tests must be updated to the new safe contract,
not weakened.

- [x] **Step 8: Commit the state-machine boundary.**

```bash
git add internal/runtime/retrieval_worker.go \
  internal/runtime/retrieval_worker_test.go \
  internal/runtime/retrieval_store.go internal/runtime/retrieval_store_test.go \
  internal/runtime/retrieval_coordinator.go \
  internal/runtime/retrieval_coordinator_test.go \
  internal/runtime/retrieval_dimension_routing_test.go \
  internal/runtime/dimensional_migration_profile_test.go \
  internal/runtime/operations_acceptance_test.go
git commit -m "fix: require rebuild below retention floor"
```

---

### Task 4: Implement Atomic Audited Pruning And Idempotency

**Files:**
- Modify: `internal/runtime/projection_retention.go`
- Modify: `internal/runtime/projection_retention_test.go`
- Create: `internal/runtime/projection_prune_integration_test.go`

**Interfaces:**
- Consumes: Task 2 schema and Task 3 cursor semantics.
- Produces:

```go
type ProjectionPruneRequest struct {
    OperationID     string
    Cutoff           time.Time
    RetainTailEvents int
}

type ProjectionPruneReceipt struct {
    ID                   string    `json:"id"`
    TenantID             string    `json:"tenant_id"`
    OperationID          string    `json:"operation_id"`
    RequestFingerprint   string    `json:"request_fingerprint"`
    Cutoff                time.Time `json:"cutoff"`
    RetainTailEvents      int       `json:"retain_tail_events"`
    SafeCursorEventID     int64     `json:"safe_cursor_event_id"`
    PreviousFloorEventID  int64     `json:"previous_floor_event_id"`
    NewFloorEventID       int64     `json:"new_floor_event_id"`
    DeletedEvents         int64     `json:"deleted_events"`
    Result                string    `json:"result"`
    CreatedAt             time.Time `json:"created_at"`
    Replayed              bool      `json:"replayed"`
}

func (s *Store) PruneProjectionEvents(
    ctx context.Context,
    tenantID string,
    request ProjectionPruneRequest,
) (ProjectionPruneReceipt, error)
```

- [x] **Step 1: Add RED validation/fingerprint tests.**

Reject blank tenant/operation ID, zero cutoff, negative tail count, overlong
operation ID, and conflicting operation-ID reuse. Require UTC-normalized cutoff
and SHA-256 fingerprint over only normalized tenant, operation, cutoff, and
tail values. Same request replay must return the original receipt with only
`Replayed=true` changed in memory; persisted bytes remain unchanged.

- [x] **Step 2: Add RED prune-bound integration tests.**

Use interleaved tenant event IDs and three profiles:

- current active cursor;
- intentionally slow candidate cursor;
- one `rebuild_required` cursor.

Require the selected bound to be the minimum of slowest non-rebuild cursor,
old-enough event, and retain-tail bound. `rebuild_required` does not block.
No-subscriber tenants use the latest tenant event as cursor bound. A noop still
writes one idempotent receipt and never lowers the floor.

- [x] **Step 3: Run prune tests and observe RED.**

```bash
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_w18_test?host=/tmp' \
  go test -p 1 -count=1 ./internal/runtime \
  -run 'TestProjectionPrune'
```

Expected: compile failure because the API does not exist.

- [x] **Step 4: Implement normalized request and receipt replay.**

Validation and fingerprinting must be pure helpers. Existing receipt lookup
occurs inside the same transaction and tenant advisory lock as pruning.
Fingerprint mismatch returns `projection prune operation conflict` without
revealing the DSN or request internals.

- [x] **Step 5: Implement one-transaction pruning.**

The transaction must:

1. acquire `pg_advisory_xact_lock` for the tenant retention stream;
2. insert-or-lock the tenant floor row;
3. calculate the minimum non-`rebuild_required` cursor;
4. calculate cutoff and tail bounds by tenant row ordering, not global ID
   arithmetic;
5. delete only rows with `event_id > previous_floor` and
   `event_id <= selected_bound`;
6. set floor to the maximum event ID actually deleted, never merely the
   selected bound;
7. insert `pruned` or `noop` receipt;
8. commit all three mutations atomically.

The method must never delete events from another tenant and must not modify
authority, vector documents, lexical documents, retrieval audits, or cursors.

- [x] **Step 6: Add concurrent insert and monotonicity tests.**

Insert new events after bound selection and prove they survive. Run two
different operation IDs concurrently and prove the advisory lock serializes
floor movement. Require every subsequent floor to be greater than or equal to
the previous floor.

- [x] **Step 7: Run focused and package tests.**

```bash
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_w18_test?host=/tmp' \
  go test -p 1 -count=1 ./internal/runtime -run 'TestProjectionPrune|TestProjectionRetention'
```

Expected: PASS.

- [x] **Step 8: Commit the prune transaction.**

```bash
git add internal/runtime/projection_retention.go \
  internal/runtime/projection_retention_test.go \
  internal/runtime/projection_prune_integration_test.go
git commit -m "feat: prune projection events atomically"
```

---

### Task 5: Add Runtime Read-Only Boundary, RLS Proofs, CLI, And Restart Recovery

**Files:**
- Modify: `internal/authn/provision.go`
- Modify: `internal/authn/postgres_test.go`
- Modify: `internal/runtime/postgres_store.go`
- Modify: `internal/runtime/tenant_context.go`
- Modify: `internal/runtime/rls_migration_test.go`
- Modify: `internal/runtime/tenant_pool_test.go`
- Modify: `internal/runtime/operations_acceptance_test.go`
- Modify: `cmd/vermory/main.go`
- Modify: `cmd/vermory/retrieval_runtime.go`
- Modify: `cmd/vermory/retrieval_runtime_test.go`
- Create: `internal/runtime/projection_prune_restart_test.go`

**Interfaces:**
- Consumes: `Store.PruneProjectionEvents` and `ProjectionPruneReceipt`.
- Produces: `newRetrievalPruneEventsCommand()` and a runtime-role privilege
  contract with explicit read-write and read-only table sets.

- [x] **Step 1: Add RED runtime-role tests.**

Require the restricted runtime role to have:

```text
memory_projection_retention     SELECT only
memory_projection_prune_runs    no SELECT/INSERT/UPDATE/DELETE
```

All existing runtime-served tables retain their existing required privileges.
`ValidateRuntimeRole` must reject blanket CRUD on either retention-control
table and must reject missing retention-floor SELECT.

- [x] **Step 2: Run role tests and observe RED.**

```bash
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_w18_test?host=/tmp' \
  go test -p 1 -count=1 ./internal/runtime \
  -run 'TestRuntimeRole|TestRLSMigration|TestTenantPool'
```

Expected: failures because runtime privilege validation assumes blanket CRUD.

- [x] **Step 3: Split runtime privilege validation by access class.**

Replace one blanket table list with these exact access classes:

```go
readWriteTables := []string{
    "continuity_spaces", "continuity_bindings", "conversation_bindings", "observations",
    "governed_memories", "memory_deliveries", "memory_search_documents", "conversation_turns",
    "bridge_operations", "bridge_events", "bridge_memory_effects", "conversation_links",
    "source_match_decisions", "source_formation_runs", "source_formation_items",
    "memory_projection_events", "memory_projection_cursors", "memory_vector_documents",
    "memory_vector_documents_2560", "memory_retrieval_runs",
}
readOnlyTables := []string{"memory_projection_retention"}
forbiddenTables := []string{
    "vermory_auth.api_tokens",
    "public.projects", "public.sources", "public.source_versions", "public.claims",
    "public.capsules", "public.capsule_claims", "public.packets", "public.audit_logs",
    "public.wcef_runs", "public.memory_projection_prune_runs",
}
```

Validate exact access for each class. Migration/deployment test setup must
grant runtime SELECT on retention and no mutation privilege on either control
table.

- [x] **Step 4: Add RED RLS and cross-tenant prune tests.**

Prove a restricted tenant session cannot read another tenant's floor, cannot
read prune receipts, and cannot mutate either table. Prove an admin prune of
tenant A cannot count/delete tenant B events or return tenant B receipts.

- [x] **Step 5: Add RED CLI tests.**

Require root registration and flags:

```text
retrieval-prune-events
--database-url
--tenant-id
--operation-id
--before
--retain-tail-events
```

Reject missing values, malformed RFC3339, negative tail count, and unsupported
positional arguments. Successful output is one JSON receipt. Errors and output
must not contain the supplied DSN, password, API keys, or environment values.

- [x] **Step 6: Implement the operator CLI.**

Open an ordinary admin `Store`, validate schema, call
`PruneProjectionEvents`, and JSON-encode only the receipt. The command must not
call `Migrate` implicitly and must not accept runtime-role credentials as safe
merely because they connect.

- [x] **Step 7: Add and run real restart-rollback test.**

Using the existing dedicated PostgreSQL 18 helper pattern, pause a transaction
after delete/floor/audit writes but before commit, stop that dedicated cluster
with `immediate`, restart it, reuse the same pool, and assert:

```text
deleted events restored
floor unchanged
receipt absent
same pool usable
retry with operation ID succeeds
```

Run:

```bash
VERMORY_W18_RESTART_TEST=1 \
VERMORY_POSTGRES18_BIN='/opt/homebrew/opt/postgresql@18/bin' \
VERMORY_W18_ROOT="/tmp/vermory-w18-restart-$(date -u +%Y%m%dT%H%M%SZ)" \
  go test -p 1 -count=1 ./internal/runtime -run TestProjectionPruneRestartRollback -v
```

Expected: PASS and one retained injected failure record in test output.

- [x] **Step 8: Run CLI, RLS, role, and recovery regressions.**

```bash
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_w18_test?host=/tmp' \
  go test -p 1 -count=1 ./cmd/vermory ./internal/runtime \
  -run 'TestRetrievalPrune|TestRuntimeRole|TestRLSMigration|TestTenantPool|TestOperationsAcceptance'
```

Expected: PASS.

- [x] **Step 9: Commit the operator boundary.**

```bash
git add internal/authn/provision.go internal/authn/postgres_test.go \
  internal/runtime/postgres_store.go internal/runtime/tenant_context.go \
  internal/runtime/rls_migration_test.go internal/runtime/tenant_pool_test.go \
  internal/runtime/operations_acceptance_test.go \
  internal/runtime/projection_prune_restart_test.go \
  cmd/vermory/main.go cmd/vermory/retrieval_runtime.go \
  cmd/vermory/retrieval_runtime_test.go
git commit -m "feat: expose audited projection pruning"
```

---

### Task 6: Build Deterministic Miniature And Formal W18 Harnesses

**Files:**
- Create: `internal/runtime/projection_retention_profile_helpers_test.go`
- Create: `internal/runtime/projection_retention_profile_test.go`
- Create: `internal/runtime/projection_retention_formal_profile_test.go`
- Create: `internal/runtime/projection_retention_report.go`
- Create: `internal/runtime/projection_retention_report_test.go`

**Interfaces:**
- Consumes: Tasks 1-5 and W17 deterministic/provisioned PostgreSQL helpers.
- Produces:

```go
type ProjectionRetentionReport struct {
    Version                int                            `json:"version"`
    RunID                  string                         `json:"run_id"`
    CaseSHA256             string                         `json:"case_sha256"`
    ImplementationRevision string                         `json:"implementation_revision"`
    RequestFingerprint     string                         `json:"request_fingerprint"`
    PostgreSQLVersion      string                         `json:"postgresql_version"`
    PGVectorVersion        string                         `json:"pgvector_version"`
    StartedAt              time.Time                      `json:"started_at"`
    CompletedAt            time.Time                      `json:"completed_at"`
    Environment            ProjectionRetentionEnvironment `json:"environment"`
    Policy                 ProjectionRetentionPolicy      `json:"policy"`
    Counts                 ProjectionRetentionCounts      `json:"counts"`
    Epochs                 []ProjectionRetentionEpoch     `json:"epochs"`
    Profiles               []ProjectionRetentionProfile   `json:"profiles"`
    Receipts               []ProjectionPruneReceipt       `json:"receipts"`
    Restart                ProjectionRetentionRestart     `json:"restart"`
    Queries                ProjectionRetentionQueries     `json:"queries"`
    Rebuild                ProjectionRetentionRebuild     `json:"rebuild"`
    Provider               ProjectionRetentionProvider    `json:"provider"`
    HardGates              map[string]bool                `json:"hard_gates"`
    Failures               []ProjectionRetentionFailure   `json:"failures"`
    NonClaims              []string                       `json:"non_claims"`
}

type ProjectionRetentionEnvironment struct {
    OS string `json:"os"`
    CPU string `json:"cpu"`
    MemoryGiB int `json:"memory_gib"`
    SchemaVersion int `json:"schema_version"`
}

type ProjectionRetentionPolicy struct {
    Cutoff time.Time `json:"cutoff"`
    RetainTailEvents int `json:"retain_tail_events"`
}

type ProjectionRetentionCounts struct {
    InitialCurrent int `json:"initial_current"`
    GeneratedEvents int `json:"generated_events"`
    PrunedEvents int `json:"pruned_events"`
    RetainedEvents int `json:"retained_events"`
    FinalCurrent int `json:"final_current"`
    LexicalRows int `json:"lexical_rows"`
    IncumbentVectors int `json:"incumbent_vectors"`
    DimensionalVectors int `json:"dimensional_vectors"`
}

type ProjectionRetentionEpoch struct {
    Epoch int `json:"epoch"`
    TenantID string `json:"tenant_id"`
    Generated int `json:"generated"`
    Pruned int64 `json:"pruned"`
    Retained int64 `json:"retained"`
    Floor int64 `json:"floor"`
    SlowCursor int64 `json:"slow_cursor"`
}

type ProjectionRetentionProfile struct {
    TenantID string `json:"tenant_id"`
    ProfileID string `json:"profile_id"`
    Status string `json:"status"`
    LastEventID int64 `json:"last_event_id"`
    Floor int64 `json:"floor"`
    Lag int64 `json:"lag"`
    VectorCount int64 `json:"vector_count"`
}

type ProjectionRetentionRestart struct {
    FailureCode string `json:"failure_code"`
    DeletedEventsRolledBack bool `json:"deleted_events_rolled_back"`
    FloorRolledBack bool `json:"floor_rolled_back"`
    ReceiptRolledBack bool `json:"receipt_rolled_back"`
    SamePoolRecovered bool `json:"same_pool_recovered"`
    RecoveryDurationMS int64 `json:"recovery_duration_ms"`
}

type ProjectionRetentionQueries struct {
    Successful int `json:"successful"`
    CrossScopeResults int `json:"cross_scope_results"`
    LexicalDegradations int `json:"lexical_degradations"`
    P50MS int `json:"p50_ms"`
    P95MS int `json:"p95_ms"`
    P99MS int `json:"p99_ms"`
}

type ProjectionRetentionRebuild struct {
    FutureSubscriberZeroCalls bool `json:"future_subscriber_zero_calls"`
    ResetRequiredRebuild bool `json:"reset_required_rebuild"`
    AuthorityIDHashEquivalent bool `json:"authority_id_hash_equivalent"`
    DeletedMemoryAbsent bool `json:"deleted_memory_absent"`
}

type ProjectionRetentionProvider struct {
    BaseURL string `json:"base_url"`
    Model string `json:"model"`
    Dimensions int `json:"dimensions"`
    Requests int `json:"requests"`
    DurationMS int64 `json:"duration_ms"`
    ProjectionResponseSHA256 string `json:"projection_response_sha256"`
    QueryResponseSHA256 string `json:"query_response_sha256"`
}

type ProjectionRetentionFailure struct {
    Phase string `json:"phase"`
    Attempt int `json:"attempt"`
    Code string `json:"code"`
    Message string `json:"message"`
    Retried bool `json:"retried"`
}

type ProjectionRetentionArtifactPaths struct {
    JSON string
    Markdown string
}

func ValidateProjectionRetentionReport(ProjectionRetentionReport) error
func WriteProjectionRetentionReport(root string, report ProjectionRetentionReport) (ProjectionRetentionArtifactPaths, bool, error)
func ReadProjectionRetentionReport(path string) (ProjectionRetentionReport, error)
```

- [x] **Step 1: Add RED report validation and replay tests.**

The report must include case/revision/fingerprint, schema and versions, per
tenant/epoch generated-pruned-retained counts, all profile cursor/floor
positions, ordered receipts, restart evidence, query metrics, rebuild
equivalence, provider hashes/timing, chronological failures, non-claims, and
exactly 14 hard gates. Reject missing/false gates, unsorted percentiles,
inconsistent counts, invalid hashes, duplicate receipts, secret-shaped values,
and conflicting same-run replay.

- [x] **Step 2: Run report tests and observe RED.**

```bash
go test -count=1 ./internal/runtime \
  -run 'TestProjectionRetentionReport|TestProjectionRetentionReplay'
```

Expected: compile failure because report types do not exist.

- [x] **Step 3: Implement deterministic report serialization.**

JSON uses indented deterministic structures and newline termination. Markdown
renders the same normalized values. Existing identical JSON/Markdown returns
`replayed=true`; any conflict fails without overwriting. Secret scan must cover
both serialized forms before atomic rename.

- [x] **Step 4: Add and run a miniature end-to-end profile.**

Use 2 tenants, 2 continuities each, 20 facts each, 3 epochs, two established
profiles, one future subscriber, 10 retained events per tenant, and one
interrupted prune. Exercise all fourteen gates with deterministic embedders.

```bash
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_w18_test?host=/tmp' \
  go test -p 1 -count=1 ./internal/runtime \
  -run TestProjectionRetentionMiniatureProfile -v
```

Expected: PASS with no cross-scope result and exact authority/vector ID/hash
equivalence after rebuild.

- [x] **Step 5: Implement the opt-in formal harness.**

The formal test runs only when `VERMORY_W18_FORMAL=1`. Required environment:

```text
VERMORY_W18_RUN_ID
VERMORY_W18_ROOT
VERMORY_W18_ARTIFACT_ROOT
VERMORY_POSTGRES18_BIN
VERMORY_IMPLEMENTATION_REVISION
SILICONFLOW_API_KEY
```

The harness must use a new dedicated root, direct
`https://api.siliconflow.cn/v1`, `BAAI/bge-m3`, and 1024 dimensions. Existing
completed run IDs replay offline before checking PostgreSQL binaries or the
provider credential. A conflicting implementation/case fingerprint fails.

- [x] **Step 6: Encode the full frozen trajectory.**

The formal test must execute all six design phases and record:

```text
4 tenants
20 continuities
20,000 initial current facts
24 accelerated epochs
72,000 revisions
4,800 deletions
4,800 new facts
153,600 tail events
173,600 total generated events
20,000 final current facts
1,000 retained events per tenant
320 concurrent incumbent queries
0 cross-scope results
1 real restart rollback
1 future-subscriber zero-call rejection
1 reset/rebuild exact-equivalence cycle
2 direct provider requests
14/14 hard gates
```

Prune attempts during the slow-candidate phase must prove the floor never
passes its cursor. After catch-up, retention must reach the calibrated bound.

- [x] **Step 7: Run all deterministic W18 tests.**

```bash
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_w18_test?host=/tmp' \
  go test -p 1 -count=1 ./internal/runtime \
  -run 'TestProjectionRetention|TestProjectionPrune|TestRetrievalCoordinator.*ProjectionState|TestResetVectorProjection.*Retention' -v
```

Expected: PASS.

- [x] **Step 8: Commit the formal harness.**

```bash
git add internal/runtime/projection_retention_profile_helpers_test.go \
  internal/runtime/projection_retention_profile_test.go \
  internal/runtime/projection_retention_formal_profile_test.go \
  internal/runtime/projection_retention_report.go \
  internal/runtime/projection_retention_report_test.go
git commit -m "test: add projection retention fault profile"
```

---

### Task 7: Run The Formal Profile And Commit Evidence

**Files:**
- Create: `docs/evidence/2026-07-16-projection-event-retention-pruning.md`
- Create: `docs/evidence/snapshots/2026-07-16-projection-event-retention-pruning.json`
- Modify only if needed for factual documentation: `README.md`
- Modify only if needed for factual documentation: `README.zh-CN.md`

**Interfaces:**
- Consumes: Task 6 formal harness and a process-only provider credential.
- Produces: committed normalized W18 evidence with hashes and retained failures.

- [x] **Step 1: Run a fresh formal profile.**

Use a unique run ID and dedicated root. Keep the provider key only in the
process environment. Do not echo it or persist shell history containing it.

```bash
run_id="projection-retention-$(date -u +%Y%m%dT%H%M%SZ)"
root="/tmp/vermory-w18-${run_id}"
VERMORY_W18_FORMAL=1 \
VERMORY_W18_RUN_ID="$run_id" \
VERMORY_W18_ROOT="$root" \
VERMORY_W18_ARTIFACT_ROOT='/tmp/vermory-w18-artifacts' \
VERMORY_POSTGRES18_BIN='/opt/homebrew/opt/postgresql@18/bin' \
VERMORY_IMPLEMENTATION_REVISION="$(git rev-parse HEAD)" \
  go test -p 1 -count=1 ./internal/runtime -run TestProjectionRetentionFormalProfile -v
```

Expected: PASS, 14/14 hard gates, and a normalized `report.json` plus
`report.md`.

- [x] **Step 2: Replay the completed run offline.**

Unset `SILICONFLOW_API_KEY`, point PostgreSQL binary/root variables at invalid
paths, rerun the same run ID, and require byte-identical report hashes and no
network/database startup.

```bash
env -u SILICONFLOW_API_KEY \
VERMORY_W18_FORMAL=1 \
VERMORY_W18_RUN_ID="$run_id" \
VERMORY_W18_ROOT='/invalid/offline-replay-root' \
VERMORY_W18_ARTIFACT_ROOT='/tmp/vermory-w18-artifacts' \
VERMORY_POSTGRES18_BIN='/invalid/offline-postgres-bin' \
VERMORY_IMPLEMENTATION_REVISION="$(git rev-parse HEAD)" \
  go test -p 1 -count=1 ./internal/runtime -run TestProjectionRetentionFormalProfile -v
```

- [x] **Step 3: Inspect evidence and scan for secrets.**

```bash
rg -n '(sk-[A-Za-z0-9]|postgres(ql)?://[^[:space:]]+:[^[:space:]@]+@|Bearer[[:space:]]+[A-Za-z0-9._-]+)' \
  docs/evidence /tmp/vermory-w18-artifacts
```

Expected: no matches. Verify report counts, receipts, chronological failures,
non-claims, hashes, and latency ordering manually against the case manifest.

- [x] **Step 4: Commit the normalized snapshot and evidence narrative.**

The narrative must distinguish implemented behavior, formal verification,
retained failures, and explicit non-claims. It must not claim wall-clock months,
automatic authority retention, cross-host HA, sealed evaluation, signing, or
final release acceptance.

```bash
git add docs/evidence/2026-07-16-projection-event-retention-pruning.md \
  docs/evidence/snapshots/2026-07-16-projection-event-retention-pruning.json \
  README.md README.zh-CN.md
git commit -m "docs: record projection retention evidence"
```

---

### Task 8: Run Release Gates And Protected Delivery

**Files:**
- Modify: `docs/superpowers/plans/2026-07-16-projection-event-retention-pruning.md`
- Modify: Draft PR 1 body through `gh pr edit` only after final local checks.

**Interfaces:**
- Consumes: all previous tasks and existing W17 protected-delivery workflow.
- Produces: clean final checklist head, protected CI, verified artifact and
  synthetic merge, and exactly one W18 PR section.

- [x] **Step 1: Run formatting and full local gates.**

```bash
gofmt -w $(rg --files cmd/vermory internal/runtime -g '*.go')
go vet ./...
go test -count=1 ./...
git diff --check
git status --short
```

Expected: all checks pass; only intentional plan-checkbox/evidence changes are
present before the final checklist commit.

- [x] **Step 2: Verify migration replay and release build.**

Run the existing operations acceptance suite against schema 17, then run the
same release/build commands used by `.github/workflows/ci.yml` and
`.github/workflows/release.yml`. Verify Linux `amd64` and `arm64` binaries are
produced without credentials.

- [x] **Step 3: Mark every completed checkbox and commit the final checklist.**

Do not mark a checkbox until its command and expected result have been freshly
verified.

```bash
git add docs/superpowers/plans/2026-07-16-projection-event-retention-pruning.md
git commit -m "docs: close projection retention qualification"
```

- [x] **Step 4: Push the final checklist head and run protected CI.**

```bash
git push origin agent/grok-cli-runtime
gh run list --branch agent/grok-cli-runtime --limit 10
```

Wait for the run whose second parent/head corresponds to the final checklist
revision. Do not treat an earlier green run as final evidence.

- [x] **Step 5: Download and independently verify the final artifact.**

Verify artifact ID/name/size/SHA-256, embedded `git-info.txt`, synthetic merge
parents, checksums, binary execution, and that the second parent equals the
final checklist head. Record these values in the W18 evidence document if the
workflow does not already preserve them.

- [x] **Step 6: Update Draft PR 1 exactly once.**

Append one and only one section headed:

```text
## Final W18 Delivery
```

Include the final checklist revision, CI run/job, artifact identity/hash,
synthetic merge, 14/14 gate result, provider tuple, and explicit remaining
boundaries. Confirm the PR remains OPEN, Draft, CLEAN, and MERGEABLE.

- [x] **Step 7: Verify final repository state.**

```bash
git status --short --branch
git rev-parse HEAD
git rev-parse origin/agent/grok-cli-runtime
gh pr view 1 --json state,isDraft,mergeStateStatus,mergeable,body,url
git tag --list
gh release list
```

Expected: local and remote heads match, worktree is clean, PR has exactly one
W18 section, no release/tag was created, and the overall Vermory goal remains
active for external sealed evaluation, authority-retention qualification,
cross-host HA, signing, and final release acceptance.
