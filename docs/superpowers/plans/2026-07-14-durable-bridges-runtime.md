# Durable Bridges Runtime Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Deliver durable Promote, Link, Export, Adopt, Rebind, and generic reversal operations that alter real workspace/conversation behavior without implicit pooling.

**Architecture:** A PostgreSQL bridge ledger stores idempotent operations, immutable events, promotion memory effects, exports, and conversation-link edges. `BridgeService` accepts anchors rather than caller-selected continuity IDs. Workspace and conversation consumers continue using their existing continuity services, with linked conversation governed-memory retrieval added as a bounded read path.

**Tech Stack:** Go, PostgreSQL 16+, pgx v5, goose, Cobra, `net/http`, MCP stdio, Grok CLI.

## Global Constraints

- No automatic promotion, linking, rebind, merge, or target selection.
- Only explicitly selected active governed memories may cross a bridge.
- Recent raw conversation history remains anchor-local.
- Tenant identity and continuity IDs are server-owned.
- Model-facing packets and export bodies contain semantic content only.
- Reversal stops future effects and redacts Vermory-controlled generated artifacts; it does not claim to erase external copies or valid historical use.
- PostgreSQL is authoritative and every operation is atomic and idempotent.
- Database-mutating tests run serially with `VERMORY_TEST_DATABASE_URL`.

---

### Task 1: Frozen Cases And Bridge Ledger

**Files:**
- Create: `reality/cases/B01-conversation-workspace-promotion/manifest.json`
- Create: `reality/cases/B01-conversation-workspace-promotion/events.jsonl`
- Create: `reality/cases/B01-conversation-workspace-promotion/fixtures/*.md`
- Create: `reality/cases/B01-conversation-workspace-promotion/fixture-lock.json`
- Create: `reality/cases/B02-linked-conversations-workspace-rebind/manifest.json`
- Create: `reality/cases/B02-linked-conversations-workspace-rebind/events.jsonl`
- Create: `reality/cases/B02-linked-conversations-workspace-rebind/fixtures/*.md`
- Create: `reality/cases/B02-linked-conversations-workspace-rebind/fixture-lock.json`
- Create: `internal/store/postgres/migrations/00007_durable_bridges.sql`
- Create: `internal/runtime/bridge_types.go`
- Create: `internal/runtime/bridge_store.go`
- Create: `internal/runtime/bridge_store_test.go`
- Modify: `internal/runtime/postgres_store.go`

- [x] **Step 1: Freeze B01/B02 and verify fixture hashes**

Use the repository reality-case schema. B01 must distinguish selected confirmed memory from unconfirmed/noise. B02 must contain linked, unrelated, pre-link, post-link, reversed-link, rebind, and reversed-rebind expectations.

- [x] **Step 2: Write failing ledger tests**

Test action/status constraints, operation replay fingerprint checks, append-only events, tenant scoping, and reversal replay semantics.

- [x] **Step 3: Run focused tests and verify RED**

```bash
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' go test -p 1 -count=1 ./internal/runtime -run 'TestBridgeLedger'
```

- [x] **Step 4: Implement migration and minimal ledger helpers**

Create `bridge_operations`, `bridge_events`, `bridge_memory_effects`, and `conversation_links`. Extend `ResetForTest` in dependency-safe order.

- [x] **Step 5: Run focused tests and commit**

```bash
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' go test -p 1 -count=1 ./internal/runtime -run 'TestBridgeLedger'
git add reality/cases/B01-conversation-workspace-promotion reality/cases/B02-linked-conversations-workspace-rebind internal/store/postgres/migrations/00007_durable_bridges.sql internal/runtime/bridge_types.go internal/runtime/bridge_store.go internal/runtime/bridge_store_test.go internal/runtime/postgres_store.go
git commit -m "feat: add durable bridge ledger"
```

### Task 2: Promote And Export Effects

**Files:**
- Modify: `internal/runtime/bridge_store.go`
- Modify: `internal/runtime/bridge_store_test.go`
- Create: `internal/runtime/bridge_service.go`
- Create: `internal/runtime/bridge_service_test.go`

- [x] **Step 1: Write failing promote/export tests**

Prove selected active conversation memory is copied to one workspace, noise/proposed/deleted/cross-tenant memory is rejected, source remains unchanged, export includes only selected semantic content, and replay is stable.

- [x] **Step 2: Run tests and verify RED**

```bash
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' go test -p 1 -count=1 ./internal/runtime -run 'TestBridge.*Promote|TestBridge.*Export'
```

- [x] **Step 3: Implement minimal transactional effects**

Promotion creates target observations, `bridge_promoted` active memories, projections, and effect rows. Export stores a bounded title/body/profile view without creating a target continuity.

- [x] **Step 4: Implement promote/export reversal**

Promotion reversal deletes generated target memories while preserving source. Export reversal sets status `revoked` and redacts the internal body.

- [x] **Step 5: Run runtime tests and commit**

```bash
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' go test -p 1 -count=1 ./internal/runtime
git add internal/runtime/bridge_store.go internal/runtime/bridge_store_test.go internal/runtime/bridge_service.go internal/runtime/bridge_service_test.go
git commit -m "feat: add promote and export bridge effects"
```

### Task 3: Conversation Link Retrieval And Reversal

**Files:**
- Modify: `internal/runtime/bridge_store.go`
- Modify: `internal/runtime/bridge_store_test.go`
- Modify: `internal/runtime/conversation_store.go`
- Modify: `internal/runtime/conversation_service.go`
- Modify: `internal/runtime/conversation_service_test.go`

- [x] **Step 1: Write failing link tests**

Prove pre-link isolation, post-link governed-memory sharing in both directions, sibling sharing through one primary, unrelated-thread isolation, raw recent-history locality, graph ambiguity rejection, and reversal isolation.

- [x] **Step 2: Run tests and verify RED**

```bash
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' go test -p 1 -count=1 ./internal/runtime -run 'TestBridge.*Link|TestConversation.*Linked'
```

- [x] **Step 3: Implement link transactions and scoped search**

Add one active primary-child edge per bridge. Add conversation-memory search that resolves the active root group while leaving `ListRecentConversationObservations` unchanged.

- [x] **Step 4: Implement reversal and replay**

Reversal marks the edge reversed. Fresh deliveries stop cross-continuity retrieval; historical deliveries remain audit evidence.

- [x] **Step 5: Run runtime tests and commit**

```bash
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' go test -p 1 -count=1 ./internal/runtime
git add internal/runtime/bridge_store.go internal/runtime/bridge_store_test.go internal/runtime/conversation_store.go internal/runtime/conversation_service.go internal/runtime/conversation_service_test.go
git commit -m "feat: add reversible conversation links"
```

### Task 4: Workspace Adopt And Rebind

**Files:**
- Modify: `internal/runtime/bridge_store.go`
- Modify: `internal/runtime/bridge_store_test.go`
- Modify: `internal/runtime/bridge_service.go`
- Modify: `internal/runtime/bridge_service_test.go`

- [x] **Step 1: Write failing adopt/rebind tests**

Prove adopt adds an alias, rebind retires old and confirms new, both preserve continuity/memory, target conflicts fail, and reversal restores exact prior binding state.

- [x] **Step 2: Run tests and verify RED**

```bash
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' go test -p 1 -count=1 ./internal/runtime -run 'TestBridge.*Adopt|TestBridge.*Rebind'
```

- [x] **Step 3: Implement transactional binding effects**

Use existing normalized workspace roots and binding states. No path inference or directory-name matching is allowed.

- [x] **Step 4: Implement exact reversal**

Adopt reversal retires only the added alias. Rebind reversal retires the new root and restores the old binding.

- [x] **Step 5: Run runtime tests and commit**

```bash
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' go test -p 1 -count=1 ./internal/runtime
git add internal/runtime/bridge_store.go internal/runtime/bridge_store_test.go internal/runtime/bridge_service.go internal/runtime/bridge_service_test.go
git commit -m "feat: add reversible workspace anchor bridges"
```

### Task 5: HTTP And CLI Operator Surfaces

**Files:**
- Modify: `internal/webchat/handler.go`
- Modify: `internal/webchat/handler_test.go`
- Modify: `cmd/vermory/web_chat.go`
- Modify: `internal/operatorcli/command.go`
- Modify: `internal/operatorcli/command_test.go`
- Modify: `cmd/vermory/main.go`
- Create: `docs/integrations/durable-bridges-runtime.md`

- [x] **Step 1: Write failing HTTP/CLI tests**

Cover all five actions, inspect, reverse, unknown-field rejection, server-owned tenant, durable replay after recreation, and safe errors.

- [x] **Step 2: Run tests and verify RED**

```bash
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' go test -p 1 -count=1 ./internal/webchat ./internal/operatorcli ./cmd/vermory -run 'Test.*Bridge'
```

- [x] **Step 3: Implement endpoints and commands**

HTTP and CLI must call the same `BridgeService`. No normal workspace MCP authority tools are added.

- [x] **Step 4: Document semantics and limits**

Document snapshot promotion, linked governed-memory versus local raw history, export revocation limits, and exact adopt/rebind reversal.

- [x] **Step 5: Run package tests and commit**

```bash
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' go test -p 1 -count=1 ./internal/webchat ./internal/operatorcli ./cmd/vermory
git add internal/webchat/handler.go internal/webchat/handler_test.go cmd/vermory/web_chat.go internal/operatorcli/command.go internal/operatorcli/command_test.go cmd/vermory/main.go docs/integrations/durable-bridges-runtime.md
git commit -m "feat: expose durable bridge governance"
```

### Task 6: B01/B02 Acceptance, Real Replay, And Release Evidence

**Files:**
- Modify: `internal/runtime/acceptance_test.go`
- Modify: `internal/webchat/acceptance_test.go`
- Create: `docs/evidence/2026-07-14-durable-bridges-runtime.md`
- Modify: `docs/superpowers/plans/2026-07-14-durable-bridges-runtime.md`

- [x] **Step 1: Add B01/B02 acceptance tests**

Run the frozen trajectories through real PostgreSQL services and consumer boundaries. Deterministic checks must inspect actual deliveries, bindings, lifecycle rows, link groups, and export bodies.

- [x] **Step 2: Run a real external MCP promote/reverse replay**

Promote one confirmed conversation memory into a confirmed workspace, consume it through the built MCP stdio process, reverse the bridge, and prove a fresh MCP delivery omits it.

- [x] **Step 3: Run a real Grok link/reverse replay**

Use two Web Chat anchors with confirmed synthetic governed memories. Prove linked memory changes a real Grok answer, reverse the link, and use fresh persisted delivery context rather than model self-report to prove isolation.

- [x] **Step 4: Run release verification**

```bash
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' go test -p 1 -count=1 ./...
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' go test -race -p 1 -count=1 ./internal/runtime ./internal/webchat ./internal/operatorcli ./cmd/vermory ./internal/provider
go vet ./...
go mod tidy
git diff --exit-code -- go.mod go.sum
go build -o /tmp/vermory-bridges-release ./cmd/vermory
git diff --check
```

- [x] **Step 5: Complete checklist, commit, push, and update Draft PR**

Evidence must separate deterministic authority from model behavior and preserve failed probes. Keep PR 1 Draft and keep the overall project goal active.
