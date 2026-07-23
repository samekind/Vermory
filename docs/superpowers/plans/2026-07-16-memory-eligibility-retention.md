# Memory Eligibility, Retention, And Forgetting Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use
> `superpowers:executing-plans` to implement this plan inline, task by task.
> Steps use checkbox (`- [ ]`) syntax for tracking. Do not dispatch subagents.

**Goal:** Qualify authoritative memory eligibility, temporal validity,
historical archive, and authorized forgetting across Vermory's workspace,
conversation, Global Defaults, bridge, Web Chat, OpenClaw, MCP, lexical, and
semantic paths without adding a rigid retention-class enum or hidden scheduler.

**Architecture:** Schema 18 keeps PostgreSQL authoritative, adds half-open
validity bounds and explicit archive lifecycle, records one database-derived
eligibility timestamp per delivery/retrieval, and stores immutable
tenant-scoped eligibility-operation receipts. Serving paths apply one shared
authority predicate before ranking; temporal expiry is computed at request
time, archive is explicit, and forget remains irreversible redaction.

**Tech Stack:** Go 1.25/1.26 module toolchain, PostgreSQL 18, pgvector 0.8.5,
pgx v5, Goose, Cobra, existing MCP/Web Chat/OpenClaw runtimes, local Grok CLI,
official Codex CLI, direct SiliconFlow `BAAI/bge-m3`, deterministic report and
protected GitHub Actions release tooling.

## Global Constraints

- PostgreSQL remains the only memory authority.
- Do not add a `retention_class` column in W19.
- Working context remains current input, observations, and recent turns; it is
  not silently promoted to governed memory.
- Continuity-durable and Global Default roles are derived from continuity and
  memory kind, not from one shared enum.
- `valid_from`/`valid_until` control current eligibility only; expiry never
  claims content deletion.
- The validity interval is exactly `[valid_from, valid_until)`.
- `archived` preserves authorized history and is never eligible for current
  delivery.
- `deleted` remains authorized forgetting and retains existing redaction
  semantics.
- One request uses one PostgreSQL-derived UTC `eligibility_as_of` across every
  context source and records it in delivery/retrieval evidence.
- No hidden scheduler, cron service, Redis, second queue, table partitioning,
  NewAPI, automatic Global Default promotion, or automatic profile promotion.
- Lexical remains the default; semantic retrieval remains explicit opt-in.
- Active future/expired facts may remain in disposable projections, but every
  serving query must join authority and reject ineligible rows before ranking.
- Runtime roles cannot mutate eligibility or archive state directly.
- Every operator mutation is tenant scoped, row locked, idempotent, conflict
  rejecting, content-free in audit, and safe against concurrent forget.
- Real-client evidence uses local Grok and Codex only; Gemini CLI is excluded.
- Deterministic embeddings prove mechanics only. Formal completion requires a
  direct SiliconFlow `BAAI/bge-m3` projection/query probe.
- Credentials, DSNs, raw vectors, provider response bodies, private paths,
  tokens, and passwords must not enter Git, reports, logs, PR text, or release
  artifacts.
- Failures and negative evidence remain visible. No timestamp, case, or gate
  may be changed merely to hide a failed attempt.
- W19 completion does not complete the active overall Vermory goal.

---

### Task 1: Freeze Reality Cases, Formal Manifest, And RED Schema Contract

**Files:**
- Create: `reality/cases/C02-housing-viewing-validity/manifest.json`
- Create: `reality/cases/C02-housing-viewing-validity/events.jsonl`
- Create: `reality/cases/C02-housing-viewing-validity/fixture-lock.json`
- Create: `reality/cases/C02-housing-viewing-validity/fixtures/housing-viewing.md`
- Create: `reality/cases/W03-workspace-workaround-validity/manifest.json`
- Create: `reality/cases/W03-workspace-workaround-validity/events.jsonl`
- Create: `reality/cases/W03-workspace-workaround-validity/fixture-lock.json`
- Create: `reality/cases/W03-workspace-workaround-validity/fixtures/workspace-workaround.md`
- Create: `runtime/cases/W19-memory-eligibility-retention/README.md`
- Create: `runtime/cases/W19-memory-eligibility-retention/case.json`
- Create: `internal/runtime/memory_eligibility_case_test.go`
- Create: `internal/runtime/memory_eligibility_migration_test.go`
- Modify: `internal/reality/freeze_test.go`

**Interfaces:**
- Consumes: existing G01/S01 reality contracts, schema 17, and the W19 design.
- Produces: frozen C02/W03 cases, `memoryEligibilityCase`,
  `loadMemoryEligibilityCase`, and RED schema-18 assertions.

- [x] **Step 1: Freeze C02 with authorized fixture provenance.**

The case must include:

```text
conversation anchor              housing-search-validity
temporary fact                   viewing Saturday 14:00
valid_until                      exact event boundary
durable independent fact         monthly budget ceiling
before-boundary expected use      viewing + budget
at/after-boundary forbidden use   old viewing
at/after-boundary expected use    budget remains
inspection expectation            viewing is expired, not deleted
```

The fixture lock contains exact SHA-256 values and only authorized synthetic
or anonymized content.

- [x] **Step 2: Freeze W03 with temporary workaround and durable controls.**

The case must include:

```text
workspace anchor                 stable repository root
temporary fact                   disable cache until fixed boundary
durable facts                    deployment command + security constraint
before-boundary expected use      workaround present
at/after-boundary forbidden use   workaround absent
archive expectation               content inspectable, never delivered
forget control                    separate synthetic secret remains redacted
cross-client expectation          Codex and Grok see the same eligible state
```

- [x] **Step 3: Add the frozen W19 manifest.**

The manifest must encode exactly:

```json
{
  "version": "1",
  "id": "W19-memory-eligibility-retention",
  "profile_name": "memory-eligibility-retention-v1",
  "tenant_count": 4,
  "continuities_per_tenant": 5,
  "total_governed_facts": 10000,
  "current_open_ended": 4000,
  "scheduled": 1500,
  "expired": 1500,
  "archived": 1000,
  "superseded": 1000,
  "deleted": 1000,
  "query_client_count": 16,
  "queries_per_client": 20,
  "active_profile_id": "siliconflow-bge-m3-1024-v1",
  "direct_provider_tenant_id": "w19-direct-provider-tenant",
  "hard_gate_count": 16
}
```

The README states that validity boundaries are accelerated qualification
timestamps, not a wall-clock-duration claim.

- [x] **Step 4: Add case identity and arithmetic tests.**

Require:

```go
sum := current + scheduled + expired + archived + superseded + deleted
queries := queryClientCount * queriesPerClient
```

Assert `sum == 10000`, `queries == 320`, exactly four reality case IDs are
bound (`G01`, `S01`, `C02`, `W03`), exactly sixteen hard gates exist, fixture
locks match bytes, and unknown manifest fields are rejected.

- [x] **Step 5: Add failing schema-18 assertions.**

On a freshly migrated database require:

```text
schema version                                      18
governed_memories.valid_from                        present
governed_memories.valid_until                       present
validity interval check                             half-open-compatible
governed lifecycle                                  includes archived
memory_eligibility_operations                       present
operation unique key                                tenant_id + operation_id
action check                                        set_validity or archive
request fingerprint                                 64 lowercase hex
memory_deliveries.eligibility_as_of                 present and non-null
memory_retrieval_runs.eligibility_as_of             present and non-null
memory_is_eligible(...)                             present and immutable
RLS                                                 enabled on operation table
tenant policy                                       USING and WITH CHECK
PUBLIC privileges                                   revoked
```

Also require migration 18 Down/Up replay and reject invalid intervals,
unsupported actions, invalid fingerprints, cross-tenant foreign keys, and
unsafe downgrade while archived/validity/audit state remains.

- [x] **Step 6: Run reality/case tests and observe RED.**

```bash
go test -count=1 ./internal/reality \
  -run 'TestPublicCaseFixturesRemainFrozen|TestMemoryEligibilityRealityCases'

VERMORY_TEST_DATABASE_URL='postgresql:///vermory_w19_test?host=/tmp' \
  go test -p 1 -count=1 ./internal/runtime \
  -run 'TestMemoryEligibilityCaseIsFrozen|TestMemoryEligibilitySchema|TestRetrievalMigrationUpDown'
```

Expected: reality/case identity passes after fixtures are added; schema tests
fail because migration 18 does not exist.

- [x] **Step 7: Commit the frozen evidence boundary.**

```bash
git add reality/cases/C02-housing-viewing-validity \
  reality/cases/W03-workspace-workaround-validity \
  runtime/cases/W19-memory-eligibility-retention \
  internal/reality/freeze_test.go \
  internal/runtime/memory_eligibility_case_test.go \
  internal/runtime/memory_eligibility_migration_test.go
git commit -m "test: freeze memory eligibility qualification"
```

---

### Task 2: Add Schema 18, Eligibility Predicate, And Domain Types

**Files:**
- Create: `internal/store/postgres/migrations/00018_memory_eligibility_retention.sql`
- Create: `internal/runtime/memory_eligibility_types.go`
- Create: `internal/runtime/memory_eligibility_types_test.go`
- Modify: `internal/runtime/postgres_store.go`
- Modify: `internal/runtime/retrieval_types.go`
- Modify: `internal/runtime/operations_acceptance_test.go`
- Modify: latest-schema assertions under `internal/runtime/*test.go`

**Interfaces:**
- Consumes: RED migration contract from Task 1.
- Produces:

```go
type MemoryEffectiveState string

const (
    MemoryEffectiveProposed   MemoryEffectiveState = "proposed"
    MemoryEffectiveCurrent    MemoryEffectiveState = "current"
    MemoryEffectiveScheduled  MemoryEffectiveState = "scheduled"
    MemoryEffectiveExpired    MemoryEffectiveState = "expired"
    MemoryEffectiveArchived   MemoryEffectiveState = "archived"
    MemoryEffectiveSuperseded MemoryEffectiveState = "superseded"
    MemoryEffectiveRejected   MemoryEffectiveState = "rejected"
    MemoryEffectiveDeleted    MemoryEffectiveState = "deleted"
)

type EligibilitySnapshot struct {
    AsOf time.Time `json:"as_of"`
}

type MemoryValidity struct {
    ValidFrom  *time.Time `json:"valid_from,omitempty"`
    ValidUntil *time.Time `json:"valid_until,omitempty"`
}

func (s *Store) CurrentEligibilitySnapshot(ctx context.Context, tenantID string) (EligibilitySnapshot, error)
func EffectiveMemoryState(lifecycle, content string, validity MemoryValidity, asOf time.Time) MemoryEffectiveState
```

- [x] **Step 1: Add failing type, interval, boundary, and database-clock tests.**

Cover unbounded, future, exact `valid_from`, before `valid_until`, exact
`valid_until`, archived, superseded, rejected, deleted/redacted, invalid zero
times, UTC normalization, tenant validation, and stable JSON fields.

- [x] **Step 2: Run type tests and observe RED.**

```bash
go test -count=1 ./internal/runtime \
  -run 'TestEffectiveMemoryState|TestMemoryValidityValidation|TestCurrentEligibilitySnapshot'
```

Expected: compile failure because the types and methods do not exist.

- [x] **Step 3: Implement migration 18.**

The Up migration must:

- add nullable `valid_from` and `valid_until` to `governed_memories`;
- require `valid_until > valid_from` when both exist;
- extend lifecycle with `archived`;
- add non-null `eligibility_as_of` to deliveries and retrieval runs, backfilled
  from their existing creation timestamps;
- create immutable `memory_is_eligible(lifecycle, content, valid_from,
  valid_until, as_of)`;
- create `memory_eligibility_operations` with tenant-bearing foreign keys,
  exact action/fingerprint checks, unique `(tenant_id, operation_id)`, and
  previous/result fields;
- enable RLS, create tenant policy, revoke PUBLIC privileges, and add indexes
  for memory history and tenant operation inspection.

The Down migration must reject downgrade if archived rows, non-null validity,
or eligibility-operation receipts remain, then remove only schema-18 objects
and restore schema-17 checks.

- [x] **Step 4: Implement domain types and request-level database time.**

Use `SELECT clock_timestamp()` under tenant context. Normalize to UTC and
reject a zero or non-UTC `as_of` in diagnostic APIs. Keep normal client APIs
unable to supply arbitrary historical time.

- [x] **Step 5: Extend inspection and audit DTOs without changing model-facing prose.**

Add `ValidFrom`, `ValidUntil`, and `EffectiveState` to `GovernedMemory`. Add
`EligibilityAsOf` to retrieval/delivery inspection receipts. Do not include
these fields in `Memory` or semantic context text.

- [x] **Step 6: Update reset, migration replay, dump/restore, and latest-schema checks.**

`ResetForTest` truncates the operation table before governed memories. Every
intentional latest-schema assertion expects 18. Operations acceptance verifies
schema-18 replay and dump/restore compatibility.

- [x] **Step 7: Run focused schema and type tests.**

```bash
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_w19_test?host=/tmp' \
  go test -p 1 -count=1 ./internal/runtime \
  -run 'TestMemoryEligibilityCaseIsFrozen|TestMemoryEligibilitySchema|TestEffectiveMemoryState|TestMemoryValidityValidation|TestCurrentEligibilitySnapshot|TestRetrievalMigrationUpDown|TestOperationsAcceptance'
```

Expected: PASS.

- [x] **Step 8: Commit schema 18.**

```bash
git add internal/store/postgres/migrations/00018_memory_eligibility_retention.sql \
  internal/runtime/memory_eligibility_types.go \
  internal/runtime/memory_eligibility_types_test.go \
  internal/runtime/postgres_store.go internal/runtime/retrieval_types.go \
  internal/runtime/operations_acceptance_test.go internal/runtime/*test.go
git commit -m "feat: add memory eligibility authority"
```

---

### Task 3: Enforce One Eligibility Snapshot Across Every Serving Path

**Files:**
- Create: `internal/runtime/memory_eligibility_query_test.go`
- Modify: `internal/runtime/postgres_store.go`
- Modify: `internal/runtime/conversation_store.go`
- Modify: `internal/runtime/conversation_service.go`
- Modify: `internal/runtime/global_defaults_store.go`
- Modify: `internal/runtime/service.go`
- Modify: `internal/runtime/retrieval_coordinator.go`
- Modify: `internal/runtime/retrieval_projection_sql.go`
- Modify: `internal/runtime/retrieval_store.go`
- Modify: `internal/runtime/retrieval_worker.go`
- Modify: `internal/runtime/bridge_store.go`
- Modify: `internal/runtime/bridge_service.go`
- Modify: `internal/runtime/source_candidate_store.go`
- Modify: `internal/runtime/source_match_store.go`
- Modify: `internal/runtime/source_formation_store.go`
- Modify: corresponding focused tests in `internal/runtime`

**Interfaces:**
- Consumes: schema function `memory_is_eligible` and `EligibilitySnapshot`.
- Produces: at-time internal store methods and `RetrievalRequest.EligibilityAsOf`.

- [x] **Step 1: Add failing half-open-boundary tests for lexical workspace and conversation search.**

Seed current, scheduled, exact-boundary expired, archived, superseded, deleted,
and unrelated control memories. Assert only current rows return, including
linked-conversation search.

- [x] **Step 2: Add failing request-snapshot tests for context assembly.**

Use a controllable store test hook or transaction barrier to cross a validity
boundary between Global Defaults and memory lookup. Assert the completed
delivery uses one `eligibility_as_of`, not two wall-clock decisions.

- [x] **Step 3: Add failing production lexical/vector/shadow tests.**

Require:

- lexical and vector candidate SQL reject future/expired/archive rows;
- a deliberately stale vector row is never delivered;
- lexical degradation uses the identical `EligibilityAsOf`;
- retrieval audit stores the timestamp;
- request replay with a different timestamp or query conflicts.

- [x] **Step 4: Add failing bridge and source-current tests.**

Expired/archived source memory cannot be promoted, linked delivery cannot
surface it, export cannot include it, and current source candidate/match sets
exclude it. Authorized inspection still sees permitted historical content.

- [x] **Step 5: Run focused tests and observe RED.**

```bash
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_w19_test?host=/tmp' \
  go test -p 1 -count=1 ./internal/runtime \
  -run 'TestMemoryEligibilityLexicalBoundary|TestConversationEligibilitySnapshot|TestRetrievalEligibilityModes|TestBridgeEligibility|TestSourceEligibility'
```

Expected: expired/future/archive controls leak because current SQL checks only
`lifecycle_status = 'active'`.

- [x] **Step 6: Add at-time store methods and preserve compatibility wrappers.**

Normal wrappers obtain a current database snapshot when no larger operation
already owns one. Context services obtain one snapshot first, then pass it to
Global Defaults, memory search/retrieval, bridge selection, delivery creation,
and audit insertion.

- [x] **Step 7: Update every serving SQL path to use `memory_is_eligible`.**

Do not replace it with scattered `now()` expressions. Preserve existing
scope, source-authority, exact-match, ranking, and limit behavior.

- [x] **Step 8: Preserve projection storage for natural activation.**

The worker and rebuild still project active non-redacted facts regardless of
temporal eligibility. Vector and lexical serving joins apply validity. Archive
and forget continue to produce/remove absent state through lifecycle/content
changes.

- [x] **Step 9: Run focused, package, and race tests.**

```bash
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_w19_test?host=/tmp' \
  go test -p 1 -count=1 ./internal/runtime \
  -run 'TestMemoryEligibilityLexicalBoundary|TestConversationEligibilitySnapshot|TestRetrievalEligibilityModes|TestBridgeEligibility|TestSourceEligibility'

VERMORY_TEST_DATABASE_URL='postgresql:///vermory_w19_test?host=/tmp' \
  go test -p 1 -count=1 ./internal/runtime ./internal/webchat ./internal/mcpserver

VERMORY_TEST_DATABASE_URL='postgresql:///vermory_w19_test?host=/tmp' \
  go test -race -p 1 -count=1 ./internal/runtime ./internal/webchat ./internal/mcpserver
```

Expected: PASS.

- [x] **Step 10: Commit unified serving eligibility.**

```bash
git add internal/runtime internal/webchat internal/mcpserver
git commit -m "feat: enforce memory eligibility at retrieval"
```

---

### Task 4: Add Idempotent Validity And Archive Governance

**Files:**
- Create: `internal/runtime/memory_eligibility_store.go`
- Create: `internal/runtime/memory_eligibility_store_test.go`
- Create: `internal/runtime/memory_eligibility_service.go`
- Create: `internal/runtime/memory_eligibility_service_test.go`
- Modify: `internal/runtime/governance.go`
- Modify: `internal/runtime/postgres_store.go`
- Modify: `internal/operatorcli/command.go`
- Modify: `internal/operatorcli/command_test.go`
- Modify: `cmd/vermory/main.go`
- Modify: `cmd/vermory/main_test.go`

**Interfaces:**
- Consumes: schema-18 operation table and eligibility types.
- Produces:

```go
type SetMemoryValidityRequest struct {
    OperationID string
    TenantID string
    ContinuityID string
    MemoryID string
    ValidFrom *time.Time
    ValidUntil *time.Time
}

type ArchiveMemoryRequest struct {
    OperationID string
    TenantID string
    ContinuityID string
    MemoryID string
}

type MemoryEligibilityReceipt struct {
    OperationID string `json:"operation_id"`
    ContinuityID string `json:"continuity_id"`
    MemoryID string `json:"memory_id"`
    Action string `json:"action"`
    PreviousState MemoryEffectiveState `json:"previous_state"`
    ResultState MemoryEffectiveState `json:"result_state"`
    PreviousValidity MemoryValidity `json:"previous_validity"`
    ResultValidity MemoryValidity `json:"result_validity"`
    Replayed bool `json:"replayed"`
}

func (s *Store) SetMemoryValidity(context.Context, SetMemoryValidityRequest) (MemoryEligibilityReceipt, error)
func (s *Store) ArchiveMemory(context.Context, ArchiveMemoryRequest) (MemoryEligibilityReceipt, error)
```

- [x] **Step 1: Add failing request-validation and receipt tests.**

Reject blank IDs, invalid intervals, non-UTC input, same operation with
different request, wrong continuity, cross-tenant target, deleted/superseded/
rejected/archived validity changes, and unsupported archive transitions.

- [x] **Step 2: Add failing idempotency and content-free audit tests.**

Equal replay returns the same persisted before/result values with
`Replayed=true`. The audit row must not contain memory content, source content,
provider output, DSN, or credentials.

- [x] **Step 3: Add failing archive projection tests.**

Archive must atomically change lifecycle, remove lexical current projection,
emit an absent outbox event, preserve authorized inspection content, and make
rebuild keep it absent.

- [x] **Step 4: Add failing validity-extension and expiry tests.**

An expired active memory may be extended. A future memory may become current
without mutation or worker execution. A correction creates an unbounded new
revision by default and never inherits the old deadline.

- [x] **Step 5: Run governance tests and observe RED.**

```bash
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_w19_test?host=/tmp' \
  go test -p 1 -count=1 ./internal/runtime ./internal/operatorcli ./cmd/vermory \
  -run 'TestMemoryEligibilityRequestValidation|TestMemoryEligibilityOperationReplay|TestArchiveMemory|TestExtendExpiredMemory|TestMemoryEligibilityCommands'
```

Expected: compile failure because operations and commands do not exist.

- [x] **Step 6: Implement row-locked store mutations and immutable receipts.**

Use one transaction, tenant context, target `FOR UPDATE`, request fingerprint,
insert-or-replay semantics, and exact rows-affected checks. Do not update
validity or lifecycle before replay conflict is resolved.

- [x] **Step 7: Make forget participate in the same row-lock ordering.**

`deleteMemoryTx` must lock the target before inspecting lifecycle/content. A
late validity/archive operation cannot write after committed deletion.

- [x] **Step 8: Add the operator CLI surface.**

Add:

```text
vermory memory set-validity \
  --database-url ... --tenant-id ... --continuity-id ... \
  --operation-id ... --memory-id ... \
  [--valid-from RFC3339] [--valid-until RFC3339]

vermory memory archive \
  --database-url ... --tenant-id ... --continuity-id ... \
  --operation-id ... --memory-id ...
```

The command prints structured JSON and never accepts raw content.

- [x] **Step 9: Run focused and full governance tests.**

```bash
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_w19_test?host=/tmp' \
  go test -p 1 -count=1 ./internal/runtime ./internal/operatorcli ./cmd/vermory

VERMORY_TEST_DATABASE_URL='postgresql:///vermory_w19_test?host=/tmp' \
  go test -race -p 1 -count=1 ./internal/runtime ./internal/operatorcli ./cmd/vermory
```

Expected: PASS.

- [x] **Step 10: Commit governance operations.**

```bash
git add internal/runtime/memory_eligibility_* internal/runtime/governance.go \
  internal/runtime/postgres_store.go internal/operatorcli cmd/vermory
git commit -m "feat: govern memory validity and archive"
```

---

### Task 5: Qualify Concurrency, Failure, RLS, Restart, And Restore

**Files:**
- Create: `internal/runtime/memory_eligibility_fault_test.go`
- Create: `internal/runtime/memory_eligibility_recovery_test.go`
- Create: `internal/runtime/memory_eligibility_rls_test.go`
- Modify: `internal/runtime/operations_acceptance_test.go`
- Modify: `internal/runtime/retrieval_worker_test.go`
- Modify: `docs/integrations/identity-authorization-rls.md`

**Interfaces:**
- Consumes: Tasks 2-4.
- Produces: deterministic failure and operations acceptance for schema 18.

- [x] **Step 1: Add a deterministic forget-versus-extension race.**

Pause validity update after target lock acquisition, issue forget through a
second connection, release in controlled order, and run both orderings. Require
one valid serialized outcome, no resurrection, no false success receipt, and
deleted content redaction.

- [x] **Step 2: Add interrupted-transaction rollback.**

Inside one validity/archive transaction mutate authority and insert receipt,
then stop the dedicated PostgreSQL cluster with `immediate` before commit.
After restart require authority, projection state, and receipt insertion all
rolled back together and the same pool recovers.

- [x] **Step 3: Add stale-projection and provider-outage controls.**

Keep vector rows for an expired memory deliberately. Require both current
vector serving and lexical degradation to reject it. Simulate embedding timeout
and ensure failure cannot bypass eligibility.

- [x] **Step 4: Add runtime-role and RLS attacks.**

The runtime role may read eligible memory and its tenant's authorized audit
view, but direct `INSERT`/`UPDATE`/`DELETE` on eligibility operations or
governed validity fails. Cross-tenant receipt and target access returns zero or
permission error according to the service contract.

- [x] **Step 5: Add dump/restore and rebuild equivalence.**

Dump schema-18 authority, restore to a fresh database, reprovision runtime
roles, rebuild lexical and both vector classes, and compare current/scheduled/
expired/archived/deleted fingerprints. Forgotten content must remain absent.

- [x] **Step 6: Run failure and operations tests.**

```bash
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_w19_test?host=/tmp' \
  go test -p 1 -count=1 ./internal/runtime \
  -run 'TestMemoryEligibilityForgetRace|TestMemoryEligibilityImmediateStopRollback|TestMemoryEligibilityStaleVector|TestMemoryEligibilityRuntimeRole|TestMemoryEligibilityDumpRestore|TestOperationsAcceptance'

VERMORY_TEST_DATABASE_URL='postgresql:///vermory_w19_test?host=/tmp' \
  go test -race -p 1 -count=10 ./internal/runtime \
  -run 'TestMemoryEligibilityForgetRace|TestMemoryEligibilityRequestSnapshot'
```

Expected: PASS.

- [x] **Step 7: Document the operations boundary.**

Update identity/operations documentation to distinguish expiry, archive, and
forget and explain that validity is serving-time authority, not a scheduler.

- [x] **Step 8: Commit fault and recovery qualification.**

```bash
git add internal/runtime/memory_eligibility_* \
  internal/runtime/operations_acceptance_test.go \
  internal/runtime/retrieval_worker_test.go \
  docs/integrations/identity-authorization-rls.md
git commit -m "test: qualify memory eligibility failures"
```

---

### Task 6: Replay G01, S01, C02, And W03 Through Real Clients

**Files:**
- Create: `docs/integrations/memory-eligibility-retention-runtime.md`
- Create: `internal/runtime/memory_eligibility_real_client_test.go`
- Modify: `internal/runtime/conversation_service_test.go`
- Modify: `internal/runtime/service_test.go`
- Modify: `internal/mcpserver/server_test.go`
- Modify: `internal/webchat/handler_test.go`
- Modify: `integrations/openclaw/src/client.test.ts`
- Modify: `integrations/openclaw/src/index.test.ts`
- Runtime artifacts: outside Git until normalized and secret-scanned.

**Interfaces:**
- Consumes: real local Grok CLI login, official Codex CLI, production MCP/Web
  Chat/OpenClaw entry points, and frozen reality cases.
- Produces: deterministic replay harness plus real-client artifact hashes.

- [x] **Step 1: Add deterministic G01/C02 Web Chat tests.**

Require one task-local English request, a later unrelated Chinese request,
pre-boundary viewing use, exact-boundary viewing suppression, durable budget
recall, and identical `eligibility_as_of` evidence inside each turn.

- [x] **Step 2: Add deterministic W03 MCP and S01 forget tests.**

Require temporary workaround delivery before boundary, absence afterward,
durable command/security controls, archive inspection, and synthetic-secret
forget redaction through prepare/commit/rebuild.

- [x] **Step 3: Add OpenClaw eligibility regression tests.**

The plugin remains transport-only. Prepared external turns must receive the
server's filtered context, must not send lifecycle metadata to the model, and
must not cache an expired memory independently.

- [x] **Step 4: Run all deterministic client tests.**

```bash
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_w19_test?host=/tmp' \
  go test -p 1 -count=1 ./internal/runtime ./internal/webchat ./internal/mcpserver

pnpm -C integrations/openclaw test
pnpm -C integrations/openclaw typecheck
pnpm -C integrations/openclaw build
pnpm -C integrations/openclaw pack --dry-run
```

Expected: PASS.

- [x] **Step 5: Run the real Grok Web Chat trajectory.**

Use an isolated authenticated Grok wrapper with no tools or private session
memory. Replay G01 and C02 through the real `web-chat` HTTP lifecycle. Preserve
request/response artifacts outside Git, normalize semantic output, and assert:

```text
task-local English applied only inside task
later unrelated request is Chinese
expired appointment not acted upon
durable budget remains available
no internal metadata exposed
```

- [x] **Step 6: Run the real Codex MCP workspace trajectory.**

Use a disposable W03 workspace and a temporary MCP registration. Require the
official Codex CLI to call `prepare_context`, inspect the real workspace, create
one bounded artifact, and call `commit_observation`. Repeat after expiry or use
a second frozen boundary snapshot. Preserve proof that Codex consumed the
eligible durable facts and did not use the expired workaround.

- [x] **Step 7: Run an independent Grok MCP workspace control.**

Use the same continuity with an isolated Grok call so cross-client continuity
is demonstrated without relying on either client's private session cache.

- [x] **Step 8: Preserve failures and normalize real-client evidence.**

Record canceled approvals, CLI timeouts, provider failures, or invalid outputs
chronologically. Commit only hashes, semantic outputs, command versions, and
bounded non-secret excerpts.

- [x] **Step 9: Commit client integration and runbook.**

```bash
git add docs/integrations/memory-eligibility-retention-runtime.md \
  internal/runtime internal/webchat internal/mcpserver integrations/openclaw
git commit -m "test: replay memory eligibility through clients"
```

---

### Task 7: Build The Formal W19 Profile And Deterministic Report

**Files:**
- Create: `internal/runtime/memory_eligibility_profile_helpers_test.go`
- Create: `internal/runtime/memory_eligibility_formal_profile_test.go`
- Create: `internal/runtime/memory_eligibility_report.go`
- Create: `internal/runtime/memory_eligibility_report_test.go`

**Interfaces:**
- Consumes: frozen W19 manifest, Tasks 2-6, direct SiliconFlow embedding
  configuration, and normalized real-client artifact hashes.
- Produces: deterministic `report.json`, `report.md`, replay validation, and a
  complete failure ledger.

- [x] **Step 1: Add report schema and RED validation tests.**

The report must include:

```text
run/case/schema/revision identity
PostgreSQL and pgvector versions
corpus counts by effective/lifecycle state
query counts and latency percentiles
baseline task outcomes
current recall and stale misuse
archive misuse and deletion residue
Global Default pollution
scope leakage
operation replay/conflict/race outcomes
restart and restore fingerprints
projection and degradation evidence
real client/model/artifact hashes
direct provider tuple and response hashes
sixteen ordered hard gates
chronological failure ledger
explicit non-claims
```

Reject invalid arithmetic, failed gates, non-monotonic latency, duplicate
receipts, missing client hashes, unexpected provider request count, secrets,
DSNs, vectors, and raw provider bodies.

- [x] **Step 2: Add deterministic write/replay/conflict tests.**

The same report writes byte-identical JSON/Markdown. Replay returns existing
paths without database or provider startup. A different report under the same
run ID is rejected without overwrite.

- [x] **Step 3: Build the miniature profile.**

Run a small schema-18 corpus through all sixteen gates using deterministic
embeddings. Verify counts, boundary behavior, race outcomes, projection
serving filters, restore equivalence, and report validation before scaling.

- [x] **Step 4: Run the miniature profile.**

```bash
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_w19_test?host=/tmp' \
VERMORY_W19_PROFILE=mini \
VERMORY_W19_ARTIFACT_ROOT=/tmp/vermory-w19-mini \
  go test -p 1 -count=1 ./internal/runtime \
  -run TestMemoryEligibilityFormalProfile -v -timeout 30m
```

Expected: PASS with no direct-provider claim.

- [x] **Step 5: Run the full formal profile with direct provider proof.**

Use a fresh database and artifact root. The API key remains in the established
environment variable and is never echoed.

```bash
VERMORY_W19_FORMAL_PROFILE=1 \
VERMORY_W19_RUN_ID='<stable-run-id>' \
VERMORY_W19_ARTIFACT_ROOT='<fresh-artifact-root>' \
VERMORY_W19_POSTGRES_ROOT='<fresh-postgres-root>' \
  go test -p 1 -count=1 ./internal/runtime \
  -run TestMemoryEligibilityFormalProfile -v -timeout 60m
```

Require exact 10,000-state arithmetic, 320 scoped queries, zero cross-scope
results, zero stale/archive/deleted delivery, all sixteen gates, and one direct
`BAAI/bge-m3` projection/query probe.

- [x] **Step 6: Run offline replay.**

Unset the provider credential and use invalid PostgreSQL roots/binary paths.
Replay must validate the completed report before any database or network
startup and preserve artifact bytes.

- [x] **Step 7: Commit the profile and report layer.**

```bash
git add internal/runtime/memory_eligibility_*
git commit -m "test: add memory eligibility formal profile"
```

---

### Task 8: Normalize Evidence, Decide H-007, And Update Product Docs

**Files:**
- Create: `docs/evidence/2026-07-16-memory-eligibility-retention.md`
- Create: `docs/evidence/snapshots/2026-07-16-memory-eligibility-retention.json`
- Modify: `docs/superpowers/specs/2026-07-11-vermory-hypothesis-register.md`
- Modify: `docs/evaluation-matrix.md`
- Modify: `README.md`
- Modify: `README.zh-CN.md`
- Modify: `docs/integrations/global-defaults-runtime.md`
- Modify: `docs/integrations/openclaw-runtime.md`

**Interfaces:**
- Consumes: accepted formal report and real-client evidence.
- Produces: committed evidence, H-007 decision, user/operator documentation,
  and explicit non-claims.

- [x] **Step 1: Copy only normalized report evidence.**

Verify source report hashes before copying. Do not copy raw client transcripts,
provider responses, database paths, credentials, or vectors.

- [x] **Step 2: Write the evidence narrative.**

Separate:

- implemented behavior;
- deterministic database and retrieval evidence;
- real Grok/Codex behavior;
- baseline outcomes;
- retained failures;
- limits and non-claims.

State clearly that expiry is not deletion and archive is not deletion.

- [x] **Step 3: Update H-007 only from accepted evidence.**

If all gates pass, change H-007 to supported with the orthogonal interpretation
from the design. Record the evidence artifact, falsifier, and remaining
cross-deployment/privacy-policy boundaries. Do not mark a generic compliance
or universal retention policy accepted.

- [x] **Step 4: Update README, evaluation matrix, and integration docs.**

Describe user-visible behavior and commands without exposing internal field
names in normal product copy. Developer/operator sections may describe
validity, archive, audit, and `eligibility_as_of`.

- [x] **Step 5: Run evidence scans and validate arithmetic.**

```bash
rg -n -i \
  'sk-[A-Za-z0-9]|api[_-]?key|authorization:|bearer |postgresql://|/Users/|/Volumes/|raw_vector|embedding":|provider_response' \
  docs/evidence/2026-07-16-memory-eligibility-retention.md \
  docs/evidence/snapshots/2026-07-16-memory-eligibility-retention.json \
  README.md README.zh-CN.md
```

Expected: no secret/private-path matches. Manually verify counts, gates,
failure ordering, artifact hashes, and baseline outcomes against the manifest.

- [x] **Step 6: Commit normalized evidence.**

```bash
git add docs/evidence/2026-07-16-memory-eligibility-retention.md \
  docs/evidence/snapshots/2026-07-16-memory-eligibility-retention.json \
  docs/superpowers/specs/2026-07-11-vermory-hypothesis-register.md \
  docs/evaluation-matrix.md README.md README.zh-CN.md \
  docs/integrations/global-defaults-runtime.md \
  docs/integrations/openclaw-runtime.md
git commit -m "docs: record memory eligibility evidence"
```

---

### Task 9: Run Release Gates And Protected Delivery

**Files:**
- Modify: `docs/superpowers/plans/2026-07-16-memory-eligibility-retention.md`
- Modify: Draft PR 1 body through `gh pr edit` only after final local checks.

**Interfaces:**
- Consumes: all previous tasks and W18 protected-delivery workflow.
- Produces: clean final checklist head, protected CI, independently verified
  release artifact and synthetic merge, and exactly one W19 PR section.

- [x] **Step 1: Run formatting and complete local gates.**

```bash
gofmt -w $(rg --files cmd/vermory internal -g '*.go')

VERMORY_TEST_DATABASE_URL='postgresql:///vermory_w19_test?host=/tmp' \
  go test -p 1 -count=1 ./...

VERMORY_TEST_DATABASE_URL='postgresql:///vermory_w19_test?host=/tmp' \
  go test -race -p 1 -count=1 \
    ./internal/authn \
    ./internal/runtime \
    ./internal/webchat \
    ./internal/identitycli \
    ./internal/operatorcli \
    ./internal/mcpserver \
    ./cmd/vermory \
    ./internal/provider \
    ./internal/memorybackend \
    ./internal/retrievalablation

go test -count=1 -race ./internal/reality
go vet ./...
go mod tidy
git diff --exit-code -- go.mod go.sum
git diff --check

pnpm -C integrations/openclaw install --frozen-lockfile
pnpm -C integrations/openclaw check
pnpm -C integrations/openclaw pack --dry-run
```

Expected: PASS with no unintentional diff.

- [x] **Step 2: Verify operations and release build.**

Run schema-18 migration replay, immediate-stop recovery, dump/restore,
projection rebuild, trimpath migration outside the repository, GoReleaser
v2.17.0 config check and four-platform snapshot, four archive checksums/layouts,
OpenClaw 12-entry package, Darwin arm64 `version` and
`memory set-validity --help`, and static Linux binary inspection.

- [x] **Step 3: Mark completed checkboxes and commit the checklist.**

Do not mark a checkbox until its command and expected result have fresh
evidence.

```bash
git add docs/superpowers/plans/2026-07-16-memory-eligibility-retention.md
git commit -m "docs: close memory eligibility qualification"
```

- [ ] **Step 4: Push the checklist head and require protected CI on that exact head.**

```bash
git push origin agent/grok-cli-runtime
gh run list --branch agent/grok-cli-runtime --limit 10
```

Do not reuse W18 or an earlier W19 run.

- [ ] **Step 5: Independently verify the final artifact.**

Verify:

```text
artifact API ID/name/size/digest
independent transport ZIP byte count and SHA-256
four Go archive checksums
exact four-file archive layout
OpenClaw exact 12-entry layout
Darwin arm64 version and memory set-validity --help
all binary GOOS/GOARCH tuples
CGO_ENABLED=0
-trimpath=true
vcs.modified=false
synthetic merge signature valid
synthetic merge second parent equals final checklist head
```

Retain and reject any truncated or mismatched download before retrying.

- [ ] **Step 6: Update Draft PR 1 exactly once.**

Append one and only one section:

```text
## Final W19 Delivery
```

Include formal run/revision, report hashes, final checklist head, protected
CI run/job, artifact identity/digest/transport hash, synthetic merge/signature/
second parent, 16/16 gates, direct provider tuple, real Grok/Codex artifact
hashes, PR state, zero tags/releases, and remaining overall-goal boundaries.

- [ ] **Step 7: Verify final repository state.**

```bash
git status --short --branch
git rev-parse HEAD
git rev-parse origin/agent/grok-cli-runtime
gh pr view 1 --json state,isDraft,mergeStateStatus,mergeable,body,url
gh pr checks 1
git tag --list
gh release list
```

Expected:

```text
clean worktree
local head == remote head
exactly one Final W19 Delivery section
PR OPEN
PR Draft
PR CLEAN
PR MERGEABLE
required test SUCCESS
tags 0
Releases 0
overall Vermory goal active
```
