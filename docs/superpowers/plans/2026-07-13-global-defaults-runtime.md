# Global Defaults Runtime Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Deliver an explicit, keyed, auditable Global Defaults runtime consumed by both workspace and conversation paths, with G01/S01 acceptance and real Grok CLI evidence.

**Architecture:** PostgreSQL remains the sole authority. One active `global_defaults` continuity exists per tenant, and explicit service operations manage keyed governed memories through active, superseded, and deleted states. Workspace and conversation services read semantic defaults into their existing context deliveries but cannot mutate the global layer.

**Tech Stack:** Go, PostgreSQL 16+, pgx v5, goose migrations, Cobra, `net/http`, Grok CLI provider adapter.

## Global Constraints

- Do not add automatic promotion from source text, chat, or model output.
- Do not expose keys, UUIDs, lifecycle fields, or database metadata in model-facing context.
- Tenant identity is server-owned at HTTP and runtime boundaries.
- PostgreSQL is authoritative; search documents are rebuildable projections.
- Deleted content must be redacted from origin observations, governed memories, search projections, and prior deliveries.
- Database-mutating test packages run serially against `VERMORY_TEST_DATABASE_URL`.
- Gemini CLI is retired and must not be used.

---

### Task 1: Schema And Store Invariants

**Files:**
- Create: `internal/store/postgres/migrations/00006_global_defaults.sql`
- Create: `internal/runtime/global_defaults_store.go`
- Create: `internal/runtime/global_defaults_store_test.go`
- Modify: `internal/runtime/postgres_store.go`
- Modify: `internal/runtime/types.go`

**Interfaces:**
- Produces: `EnsureGlobalDefaultsContinuity`, `ListActiveGlobalDefaults`, `ListGlobalDefaults`, `SetGlobalDefault`, `CorrectGlobalDefault`, and `ForgetGlobalDefault` store methods.
- Produces: `ObservationKindGlobalDefaultSet` and keyed `GovernedMemory` inspection data.

- [x] **Step 1: Write failing migration/store tests**

Cover one active continuity per tenant, unique active key, set replay, conflicting replay rejection, correction preserving key, deletion redaction, projection rebuild, and cross-tenant target rejection.

- [x] **Step 2: Run the focused tests and verify RED**

Run:

```bash
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' go test -p 1 -count=1 ./internal/runtime -run 'TestGlobalDefaultStore'
```

Expected: compile failure or missing migration/runtime methods.

- [x] **Step 3: Add migration and minimal store implementation**

Migration requirements:

```sql
CHECK (continuity_line IN ('workspace', 'conversation', 'global_defaults'))
CREATE UNIQUE INDEX ... WHERE continuity_line = 'global_defaults' AND state = 'active';
ALTER TABLE governed_memories ADD COLUMN memory_key TEXT NOT NULL DEFAULT '';
CREATE UNIQUE INDEX ... WHERE lifecycle_status = 'active' AND memory_key <> '';
```

Store transactions must validate tenant, continuity line, target lifecycle, preserved key, operation idempotency, and projection writes.

- [x] **Step 4: Run focused and existing runtime tests and verify GREEN**

Run:

```bash
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' go test -p 1 -count=1 ./internal/runtime
```

Expected: PASS.

- [x] **Step 5: Commit the store slice**

```bash
git add internal/store/postgres/migrations/00006_global_defaults.sql internal/runtime/global_defaults_store.go internal/runtime/global_defaults_store_test.go internal/runtime/postgres_store.go internal/runtime/types.go
git commit -m "feat: add global defaults authority store"
```

### Task 2: Explicit Management Service

**Files:**
- Create: `internal/runtime/global_defaults_types.go`
- Create: `internal/runtime/global_defaults_service.go`
- Create: `internal/runtime/global_defaults_service_test.go`

**Interfaces:**
- Produces: `NewGlobalDefaultsService`, `Inspect`, `Set`, `Correct`, and `Forget`.
- Produces: validated request and receipt types used by HTTP and CLI surfaces.

- [x] **Step 1: Write failing service tests**

Test explicit set/inspect/correct/forget, normalized key validation, duplicate active key rejection, idempotent replay, and unconfigured service rejection.

- [x] **Step 2: Run focused tests and verify RED**

```bash
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' go test -p 1 -count=1 ./internal/runtime -run 'TestGlobalDefaultsService'
```

- [x] **Step 3: Implement the minimal service**

The service must own the tenant, call only explicit global-default store methods, and never accept a continuity ID or tenant ID from the caller.

- [x] **Step 4: Run focused tests and verify GREEN**

```bash
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' go test -p 1 -count=1 ./internal/runtime -run 'TestGlobalDefaultsService'
```

- [x] **Step 5: Commit the service slice**

```bash
git add internal/runtime/global_defaults_types.go internal/runtime/global_defaults_service.go internal/runtime/global_defaults_service_test.go
git commit -m "feat: add explicit global defaults service"
```

### Task 3: Workspace And Conversation Consumption

**Files:**
- Modify: `internal/runtime/service.go`
- Modify: `internal/runtime/service_test.go`
- Modify: `internal/runtime/conversation_service.go`
- Modify: `internal/runtime/conversation_service_test.go`
- Modify: `internal/runtime/acceptance_test.go`

**Interfaces:**
- Consumes: `ListActiveGlobalDefaults`.
- Produces: `BuildWorkspaceContext` and an extended `BuildConversationContext` that render semantic defaults before scoped context.

- [x] **Step 1: Write failing consumer tests**

Prove both paths receive the same active default, scoped memory remains isolated, the task instruction appears only as the current prompt, and deletion removes the default from replayed consumers.

- [x] **Step 2: Run focused tests and verify RED**

```bash
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' go test -p 1 -count=1 ./internal/runtime -run 'Test.*GlobalDefault|TestG01'
```

- [x] **Step 3: Implement semantic context composition**

Render only non-empty, non-redacted semantic content under `Global defaults:`. Keep workspace deliveries attached to workspace continuities and conversation deliveries attached to conversation continuities.

- [x] **Step 4: Run all runtime tests and verify GREEN**

```bash
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' go test -p 1 -count=1 ./internal/runtime
```

- [x] **Step 5: Commit the consumer slice**

```bash
git add internal/runtime/service.go internal/runtime/service_test.go internal/runtime/conversation_service.go internal/runtime/conversation_service_test.go internal/runtime/acceptance_test.go
git commit -m "feat: deliver global defaults to runtime consumers"
```

### Task 4: HTTP And CLI Management Surfaces

**Files:**
- Modify: `internal/webchat/handler.go`
- Modify: `internal/webchat/handler_test.go`
- Modify: `cmd/vermory/web_chat.go`
- Modify: `cmd/vermory/web_chat_test.go`
- Modify: `internal/operatorcli/command.go`
- Modify: `internal/operatorcli/command_test.go`
- Modify: `cmd/vermory/main.go`

**Interfaces:**
- Consumes: `GlobalDefaultsService`.
- Produces: `/v1/defaults` endpoints and `vermory defaults` commands.

- [x] **Step 1: Write failing HTTP and CLI tests**

Test all four operations, server-owned tenant behavior, unknown JSON rejection, not-found safety, and durable receipts after server/command recreation.

- [x] **Step 2: Run focused tests and verify RED**

```bash
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' go test -p 1 -count=1 ./internal/webchat ./internal/operatorcli ./cmd/vermory -run 'Test.*Default'
```

- [x] **Step 3: Implement the surfaces**

Wire the same store and server-owned tenant into conversation and default services. Do not accept tenant or continuity identifiers in request bodies.

- [x] **Step 4: Run package tests and verify GREEN**

```bash
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' go test -p 1 -count=1 ./internal/webchat ./internal/operatorcli ./cmd/vermory
```

- [x] **Step 5: Commit the surface slice**

```bash
git add internal/webchat/handler.go internal/webchat/handler_test.go cmd/vermory/web_chat.go cmd/vermory/web_chat_test.go internal/operatorcli/command.go internal/operatorcli/command_test.go cmd/vermory/main.go
git commit -m "feat: expose global defaults management"
```

### Task 5: G01 And S01 Automated Acceptance

**Files:**
- Modify: `internal/runtime/acceptance_test.go`
- Modify: `internal/webchat/acceptance_test.go`
- Create: `docs/integrations/global-defaults-runtime.md`

**Interfaces:**
- Consumes: complete service, HTTP, CLI, workspace, and conversation paths.
- Produces: executable G01/S01 evidence and operator documentation.

- [x] **Step 1: Add failing end-to-end acceptance tests**

G01 must set Chinese, consume in workspace/chat, apply an English task-local prompt, inspect unchanged state, replay a later Chinese task, then correct/delete and replay both consumers. S01 must prove source/chat paths cannot promote and deleted content is absent after rebuild.

- [x] **Step 2: Run acceptance tests and verify RED when any gate is absent**

```bash
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' go test -p 1 -count=1 ./internal/runtime ./internal/webchat -run 'TestG01|TestS01'
```

- [x] **Step 3: Complete minimal behavior and documentation**

Document commands, endpoint contracts, precedence, explicit authority, replay semantics, and deletion behavior without exposing implementation fields in normal user-facing examples.

- [x] **Step 4: Run acceptance and full serial suites**

```bash
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' go test -p 1 -count=1 ./...
```

- [x] **Step 5: Commit acceptance assets**

```bash
git add internal/runtime/acceptance_test.go internal/webchat/acceptance_test.go docs/integrations/global-defaults-runtime.md
git commit -m "test: prove global defaults runtime hard gates"
```

### Task 6: Real Grok Replay And Release Evidence

**Files:**
- Create: `docs/evidence/2026-07-13-grok-global-defaults-runtime.md`
- Modify: `docs/superpowers/plans/2026-07-13-global-defaults-runtime.md`

**Interfaces:**
- Consumes: built `vermory` binary, PostgreSQL, loopback Web Chat, and authenticated local Grok CLI.
- Produces: redacted, reproducible real-provider evidence and a completed checklist.

- [x] **Step 1: Build and start the real runtime**

Use a temporary tenant and local database. Set the Chinese default through the explicit management API or CLI, then run the loopback Web Chat with `--provider grok-cli`.

- [x] **Step 2: Execute the G01 replay**

Capture durable receipts for Chinese default behavior, English task-local override, later unrelated Chinese behavior, unchanged inspection, workspace injection, correction/deletion, and post-delete absence. Do not record credentials or deleted sensitive content.

- [x] **Step 3: Run release verification**

```bash
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' go test -p 1 -count=1 ./...
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' go test -race -p 1 -count=1 ./internal/runtime ./internal/webchat ./internal/operatorcli ./cmd/vermory ./internal/provider
go vet ./...
go mod tidy
git diff --exit-code -- go.mod go.sum
go build ./cmd/vermory
git diff --check
```

- [x] **Step 4: Write evidence and complete every checklist item**

Evidence must distinguish deterministic assertions from real-model behavior and include commands, provider/model identity, durable receipt IDs, lifecycle results, and redaction-safe excerpts.

- [x] **Step 5: Commit, push, and update the Draft PR**

```bash
git add docs/evidence/2026-07-13-grok-global-defaults-runtime.md docs/superpowers/plans/2026-07-13-global-defaults-runtime.md
git commit -m "docs: complete global defaults runtime evidence"
git push origin agent/grok-cli-runtime
```

Update Draft PR 1 with the new runtime scope and verification evidence. Keep it Draft.
