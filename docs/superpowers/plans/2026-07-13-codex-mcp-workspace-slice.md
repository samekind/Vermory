# Codex MCP Workspace Slice Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use \`superpowers:executing-plans\` to implement this plan task-by-task. Steps use checkbox (\`- [ ]\`) syntax for tracking.

**Goal:** Build the first durable Vermory workspace continuity slice: local MCP \`stdio\` context preparation and observation write-back backed by PostgreSQL, with lexical projection rebuild and deterministic safety tests.

**Architecture:** A new \`internal/runtime\` package owns the workspace application contract and speaks only to a dedicated PostgreSQL authority store. The MCP adapter exposes \`prepare_context\` and \`commit_observation\`; one server-configured tenant owns the scope, so MCP callers cannot select a tenant. A rebuildable lexical projection is derived from active memory rows and is lifecycle-filtered before ranking.

**Tech Stack:** Go 1.25.7, PostgreSQL, pgx/v5, goose, Cobra, official \`github.com/modelcontextprotocol/go-sdk/mcp\` v1.2.0, \`go test\`.

## Global Constraints

- Target workspace-backed continuity only. Conversation, Global Defaults, bridges, Streamable HTTP, OpenClaw, UI, and generic provider changes are out of scope.
- PostgreSQL is authoritative; no Redis, mem0, MemOS, Supermemory, cache, or embedding provider is required by this slice.
- This slice builds lexical retrieval only. Vector fusion remains a separate evidence-gated hypothesis and must not be advertised here.
- The server owns one configured local \`tenant_id\`; MCP tool arguments never choose a tenant.
- An ambiguous binding returns \`needs_confirmation\`, creates no memory, and injects no continuity context.
- Logging uses stderr only; stdout belongs exclusively to MCP transport.
- Model and tool outputs are observations or proposals, never automatically active authority.
- Every new exported behavior begins with a failing test.

---

### Task 1: Create Workspace Runtime Contracts and Authority Store

**Files:**
- Create: \`internal/runtime/types.go\`
- Create: \`internal/runtime/types_test.go\`
- Create: \`internal/store/postgres/migrations/00002_workspace_runtime.sql\`
- Create: \`internal/runtime/postgres_store.go\`
- Create: \`internal/runtime/postgres_store_test.go\`

**Interfaces:**
- Produces: \`WorkspaceAnchor\`, \`PrepareContextRequest\`, \`CommitObservationRequest\`, \`Store.ResolveWorkspace\`, \`Store.CommitObservation\`, \`Store.DeleteMemory\`.
- Consumes: \`pgxpool.Pool\`; no existing project-centric claim tables.

- [ ] **Step 1: Write failing contract and store tests**

~~~go
func TestWorkspaceAnchorNormalizesRepoRoot(t *testing.T) {
    anchor, err := (WorkspaceAnchor{RepoRoot: "/work/acorn/../acorn/"}).Normalized()
    if err != nil || anchor.RepoRoot != "/work/acorn" {
        t.Fatalf("anchor=%#v err=%v", anchor, err)
    }
}

func TestStoreCommitObservationIsIdempotent(t *testing.T) {
    store, continuity := seededRuntimeStore(t)
    first, err := store.CommitObservation(ctx, "local", continuity, CommitObservationRequest{
        OperationID: "op-1", Kind: "user_correction", Content: "Use checkout_eta_v2.",
    })
    second, err2 := store.CommitObservation(ctx, "local", continuity, CommitObservationRequest{
        OperationID: "op-1", Kind: "user_correction", Content: "Use checkout_eta_v2.",
    })
    if err != nil || err2 != nil || first.ObservationID != second.ObservationID {
        t.Fatalf("first=%#v second=%#v errors=%v/%v", first, second, err, err2)
    }
}
~~~

- [ ] **Step 2: Verify RED**

Run: \`go test ./internal/runtime -run 'TestWorkspaceAnchorNormalizesRepoRoot|TestStoreCommitObservationIsIdempotent' -count=1\`

Expected: compile failure because the runtime package does not exist.

- [ ] **Step 3: Add the minimal schema and contracts**

~~~go
type WorkspaceAnchor struct {
    RepoRoot          string
    CWD               string
    ExplicitBindingID string
}

type PrepareContextRequest struct {
    OperationID string
    Workspace   WorkspaceAnchor
    Task        string
    MaxItems    int
}

type CommitObservationRequest struct {
    OperationID string
    DeliveryID  string
    Kind        string
    Content     string
    SourceRef   string
}
~~~

The migration creates only:
- \`continuity_spaces\`: tenant, workspace line, active/needs-confirmation state.
- \`continuity_bindings\`: tenant, continuity, normalized repo root, confirmed/ambiguous/retired state.
- \`observations\`: immutable content/source reference with unique tenant operation ID.
- \`governed_memories\`: continuity, origin observation, active/superseded/deleted lifecycle, revision/supersession link.
- \`memory_deliveries\`: exact context prepared for one task.
- \`memory_search_documents\`: disposable lexical materialization.

Use one PostgreSQL transaction for idempotent observation receipts, lifecycle transition, and search-document changes. Delete redacts protected authoritative content and removes its search document.

- [ ] **Step 4: Verify GREEN**

Run: \`DATABASE_URL="$VERMORY_TEST_DATABASE_URL" go test ./internal/runtime -run 'TestWorkspaceAnchorNormalizesRepoRoot|TestStoreCommitObservationIsIdempotent' -count=1\`

Expected: PASS. The integration test must explicitly skip only when \`VERMORY_TEST_DATABASE_URL\` is unset.

- [ ] **Step 5: Commit**

~~~bash
git add internal/runtime internal/store/postgres/migrations/00002_workspace_runtime.sql
git commit -m "feat: persist workspace continuity authority"
~~~

### Task 2: Build Context Preparation, Governance, and Rebuildable Lexical Retrieval

**Files:**
- Create: \`internal/runtime/service.go\`
- Create: \`internal/runtime/service_test.go\`
- Modify: \`internal/runtime/postgres_store.go\`
- Modify: \`internal/runtime/postgres_store_test.go\`

**Interfaces:**
- Consumes: Task 1 \`Store\` and contracts.
- Produces: \`Service.PrepareContext\`, \`Service.CommitObservation\`, \`Store.RebuildProjection\`, \`Store.SearchActiveMemory\`.

- [ ] **Step 1: Write failing service and forgetting tests**

~~~go
func TestPrepareContextExcludesOtherWorkspaceAndSupersededFacts(t *testing.T) {
    service := seededService(t)
    got, err := service.PrepareContext(ctx, PrepareContextRequest{
        OperationID: "prepare-1",
        Workspace: WorkspaceAnchor{RepoRoot: "/repo/web-checkout"},
        Task: "Find the current checkout flag",
        MaxItems: 5,
    })
    if err != nil || !strings.Contains(got.Context, "checkout_eta_v2") ||
        strings.Contains(got.Context, "ops_exception_queue_refresh") ||
        strings.Contains(got.Context, "checkout_eta_v1") {
        t.Fatalf("response=%#v err=%v", got, err)
    }
}

func TestDeletedMemoryDoesNotReturnAfterRebuild(t *testing.T) {
    store, continuity, memoryID := seededDeletableMemory(t)
    requireNoError(t, store.DeleteMemory(ctx, "local", continuity, memoryID))
    requireNoError(t, store.RebuildProjection(ctx, "local", continuity))
    requireNotContains(t, mustSearch(t, store, continuity, "orchid recovery code"), "ORCHID-7419")
}
~~~

- [ ] **Step 2: Verify RED**

Run: \`go test ./internal/runtime -run 'TestPrepareContextExcludesOtherWorkspaceAndSupersededFacts|TestDeletedMemoryDoesNotReturnAfterRebuild' -count=1\`

Expected: compile failure because \`Service\` and projection operations do not exist.

- [ ] **Step 3: Implement the smallest behavior**

~~~go
func (s *Service) PrepareContext(ctx context.Context, req PrepareContextRequest) (PrepareContextResponse, error)
func (s *Service) CommitObservation(ctx context.Context, req CommitObservationRequest) (CommitObservationResponse, error)
func (s *Store) RebuildProjection(ctx context.Context, tenantID, continuityID string) error
func (s *Store) SearchActiveMemory(ctx context.Context, tenantID, continuityID, query string, limit int) ([]Memory, error)
~~~

\`PrepareContext\` resolves a confirmed binding or returns \`needs_confirmation\` with empty context. It retrieves only active, non-deleted memory for the resolved continuity and records a delivery. It returns semantic facts only, never IDs, source paths, scores, or audit fields.

\`CommitObservation\` resolves the delivery continuity, writes an observation, and creates only a \`proposed\` item for \`agent_result\`. Explicit \`user_correction\` may supersede one named active item. Search uses exact normalized phrase matches first, then PostgreSQL \`simple\` full-text and \`pg_trgm\` candidates. Every candidate is rechecked against authoritative lifecycle state. Rebuild deletes and recreates search documents only from active authority rows.

- [ ] **Step 4: Verify GREEN**

Run: \`DATABASE_URL="$VERMORY_TEST_DATABASE_URL" go test ./internal/runtime -count=1\`

Expected: PASS, including cross-workspace isolation, stale suppression, idempotency, proposed agent output, deletion, and rebuild.

- [ ] **Step 5: Commit**

~~~bash
git add internal/runtime
git commit -m "feat: prepare governed workspace context"
~~~

### Task 3: Expose the Runtime over MCP Stdio

**Files:**
- Modify: \`go.mod\`
- Modify: \`go.sum\`
- Create: \`internal/mcpserver/server.go\`
- Create: \`internal/mcpserver/server_test.go\`
- Modify: \`cmd/vermory/main.go\`
- Modify: \`cmd/vermory/main_test.go\`

**Interfaces:**
- Consumes: Task 2 \`runtime.Service\`.
- Produces: \`vermory mcp-stdio --database-url ... --tenant-id local\` and MCP tools \`prepare_context\`, \`commit_observation\`.

- [ ] **Step 1: Write failing MCP tests**

~~~go
func TestPrepareContextToolReturnsNeedConfirmationWithoutContext(t *testing.T) {
    handler := New(testService(t), Config{TenantID: "local"})
    _, out, err := handler.PrepareContext(ctx, nil, PrepareContextInput{
        OperationID: "prepare-1", RepoRoot: "/ambiguous/repo", Task: "Continue work",
    })
    if err != nil || out.Status != "needs_confirmation" || out.Context != "" {
        t.Fatalf("out=%#v err=%v", out, err)
    }
}

func TestCommitObservationToolHasNoTenantInput(t *testing.T) {
    inputType := reflect.TypeFor[CommitObservationInput]()
    if _, ok := inputType.FieldByName("TenantID"); ok {
        t.Fatal("MCP input must not select a tenant")
    }
}
~~~

- [ ] **Step 2: Verify RED**

Run: \`go test ./internal/mcpserver ./cmd/vermory -count=1\`

Expected: compile failure because no MCP server exists.

- [ ] **Step 3: Implement the adapter and command**

Add the official SDK with:

~~~bash
go get github.com/modelcontextprotocol/go-sdk@v1.2.0
~~~

Register only these normal-flow tools:

~~~go
mcp.AddTool(server, &mcp.Tool{
    Name: "prepare_context", Description: "Resolve a workspace and return governed task context.",
}, handler.PrepareContext)
mcp.AddTool(server, &mcp.Tool{
    Name: "commit_observation", Description: "Record post-task evidence as a governed observation.",
}, handler.CommitObservation)
~~~

The Cobra command opens PostgreSQL once, constructs \`runtime.Service\`, and runs:

~~~go
server.Run(cmd.Context(), &mcp.StdioTransport{})
~~~

All diagnostics must use stderr. Validation failures become tool errors or structured \`needs_confirmation\` outputs, never regular stdout text.

- [ ] **Step 4: Verify GREEN**

Run: \`go test ./internal/mcpserver ./cmd/vermory -count=1\`

Expected: PASS and CLI help includes \`mcp-stdio\`.

- [ ] **Step 5: Commit**

~~~bash
git add go.mod go.sum internal/mcpserver cmd/vermory/main.go cmd/vermory/main_test.go
git commit -m "feat: expose workspace context over mcp"
~~~

### Task 4: Add Acceptance Fixture and Codex Replay Guide

**Files:**
- Create: \`runtime/cases/W02-codex-workspace-slice/case.json\`
- Create: \`runtime/cases/W02-codex-workspace-slice/seed.sql\`
- Create: \`internal/runtime/acceptance_test.go\`
- Create: \`docs/integrations/codex-mcp-workspace-slice.md\`

**Interfaces:**
- Consumes: Tasks 1-3.
- Produces: deterministic runtime acceptance and an exact real-Codex replay contract.

- [ ] **Step 1: Write failing end-to-end acceptance test**

~~~go
func TestWorkspaceSliceAcceptance(t *testing.T) {
    env := seedAcceptanceCase(t, "../../runtime/cases/W02-codex-workspace-slice")
    first := env.Prepare("prepare-1", "/fixtures/web-checkout", "Continue checkout work")
    requireContains(t, first.Context, "checkout_eta_v2")
    requireNotContains(t, first.Context, "ops_exception_queue_refresh")

    env.Commit("writeback-1", first.DeliveryID, "user_correction", "Use checkout_eta_v3.")
    second := env.Prepare("prepare-2", "/fixtures/web-checkout", "Use the current checkout flag")
    requireContains(t, second.Context, "checkout_eta_v3")
    requireNotContains(t, second.Context, "checkout_eta_v2")

    env.Delete("checkout_eta_v3")
    env.AssertAbsentAfterRebuild("checkout flag", "checkout_eta_v3")
}
~~~

- [ ] **Step 2: Verify RED**

Run: \`DATABASE_URL="$VERMORY_TEST_DATABASE_URL" go test ./internal/runtime -run TestWorkspaceSliceAcceptance -count=1\`

Expected: fixture-not-found or missing-helper failure.

- [ ] **Step 3: Add fixture and guide**

The fixture contains a confirmed workspace anchor, current \`checkout_eta_v2\`, stale \`checkout_eta_v1\`, a separate \`ops-console\` distractor, an explicit correction to \`checkout_eta_v3\`, and a deletion probe.

The guide contains:
- MCP configuration using the local \`vermory mcp-stdio\` binary.
- Required \`prepare_context\` then \`commit_observation\` order.
- Artifact/ledger locations and source-reference privacy requirements.
- Required real repository patch/test/command artifact.
- Explicit statement that scripted Go replay is not real Codex evidence.

- [ ] **Step 4: Verify GREEN**

Run: \`DATABASE_URL="$VERMORY_TEST_DATABASE_URL" go test -count=1 ./internal/runtime ./internal/mcpserver ./cmd/vermory && go test -count=1 ./... && go test -race ./internal/runtime && go vet ./... && go mod tidy -diff\`

Expected: PASS.

- [ ] **Step 5: Replay through Codex and preserve evidence**

Run the guide against one authorized real repository. Preserve MCP request/response ledger, repository task artifact, source correction, post-task observation, follow-up retrieval, deletion probes, and comparable baseline artifacts. If Codex does not call both tools, retain the failure and do not mark the slice complete.

- [ ] **Step 6: Commit**

~~~bash
git add runtime/cases/W02-codex-workspace-slice internal/runtime/acceptance_test.go docs/integrations/codex-mcp-workspace-slice.md
git commit -m "test: add workspace mcp acceptance case"
~~~

## Plan Self-Review

- Spec coverage: Tasks 1-4 implement durable binding, authority state, observation/write-back, delivery ledger, lexical rebuild, MCP stdio, update, isolation, deletion, and real-client evidence requirements.
- Scope: the plan deliberately does not implement conversation, Global Defaults, bridges, OpenClaw, HTTP, UI, vectors, or benchmark execution.
- Type consistency: Task 1 contracts feed Task 2 service, Task 3 MCP adapter, and Task 4 acceptance path.
- Placeholder scan: all tasks name files, interfaces, failing tests, verification commands, and completion criteria.
