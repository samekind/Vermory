# Local Operator CLI Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a local, explicitly trusted CLI path for workspace confirmation, scoped inspection, source recording, correction, and forgetting without granting governance authority to MCP clients.

**Architecture:** `internal/runtime` receives a small `GovernanceService` facade that owns a configured tenant and composes existing PostgreSQL authority transactions. A focused `internal/operatorcli` Cobra package renders JSON receipts and is registered by `cmd/vermory`; it neither edits database rows directly nor exposes any operator action through MCP.

**Tech Stack:** Go 1.25.7, Cobra, PostgreSQL, pgx/v5, goose, `encoding/json`, `go test`.

## Global Constraints

- PostgreSQL remains the only authority; this work adds no cache, vector, embedding, mem0, MemOS, Supermemory, or provider dependency.
- Normal MCP operation remains exactly `prepare_context` and `commit_observation`; the latter can create only `proposed` agent results.
- Every command uses explicit `--database-url` and `--tenant-id`; no command accepts tenant selection through MCP.
- Unknown workspace inspection returns `needs_confirmation` without mutation. Every mutation rejects an unconfirmed root.
- `memory add-source` creates one active source fact and never infers supersession. `memory correct` requires an explicit active target. `memory forget` requires an explicit target and stores only the bounded content `Operator requested deletion.`.
- Every memory mutation requires a caller-supplied `--operation-id`; repeated IDs are governed by the existing tenant-scoped idempotency contract.
- Command results are JSON on stdout. Errors remain on stderr through Cobra. MCP context remains semantic-only and does not expose governance metadata.
- `rebind`, bridges, conversation continuity, Global Defaults, HTTP, UI, authentication, roles, and remote administration are out of scope.

---

### Task 1: Add the Trusted Runtime Governance Facade

**Files:**
- Create: `internal/runtime/governance.go`
- Create: `internal/runtime/governance_test.go`
- Modify: `internal/runtime/postgres_store.go`

**Interfaces:**
- Consumes: `Store.ConfirmWorkspaceBinding`, `Store.ResolveWorkspace`, `Store.CommitGovernedObservation`, `Store.RebuildProjection`, and the `governed_memories` lifecycle tables.
- Produces: `NewGovernanceService`, scoped inspect/list operations, and three explicit trusted mutations for the CLI package.

- [x] **Step 1: Write failing governance tests**

Create `internal/runtime/governance_test.go` with the following tests. Reuse the existing package-private `openTestStore`, `mustSearch`, and `requireNoError` helpers from runtime tests.

```go
func TestGovernanceInspectDoesNotCreateUnknownWorkspace(t *testing.T) {
    service := NewGovernanceService(openTestStore(t), "local")

    got, err := service.InspectWorkspace(context.Background(), "/repo/unknown")
    requireNoError(t, err)
    if got.Status != ResolutionNeedsConfirmation || got.ContinuityID != "" {
        t.Fatalf("unknown workspace was attached: %#v", got)
    }
}

func TestGovernanceSourceCorrectionAndForgetStayScoped(t *testing.T) {
    ctx := context.Background()
    store := openTestStore(t)
    service := NewGovernanceService(store, "local")
    _, err := service.ConfirmWorkspace(ctx, "/repo/web-checkout")
    requireNoError(t, err)
    _, err = service.ConfirmWorkspace(ctx, "/repo/ops-console")
    requireNoError(t, err)

    source, err := service.AddSource(ctx, "/repo/web-checkout", GovernanceWriteRequest{
        OperationID: "operator-source-v1",
        Content:     "Use checkout_eta_v1 for the staged checkout release.",
        SourceRef:   "fixture:operator:v1",
    })
    requireNoError(t, err)
    if source.Memory.Status != "active" {
        t.Fatalf("source fact is not active: %#v", source)
    }

    corrected, err := service.Correct(ctx, "/repo/web-checkout", source.Memory.MemoryID, GovernanceWriteRequest{
        OperationID: "operator-correct-v2",
        Content:     "Use checkout_eta_v2 for the staged checkout release.",
    })
    requireNoError(t, err)
    if corrected.Memory.Status != "active" {
        t.Fatalf("correction is not active: %#v", corrected)
    }

    resolution, err := service.InspectWorkspace(ctx, "/repo/web-checkout")
    requireNoError(t, err)
    requireNoError(t, store.RebuildProjection(ctx, "local", resolution.ContinuityID))
    matches := mustSearch(t, store, resolution.ContinuityID, "checkout_eta_v1")
    if len(matches) != 1 || !strings.Contains(matches[0].Content, "checkout_eta_v2") {
        t.Fatalf("stale source was not replaced: %#v", matches)
    }

    forgotten, err := service.Forget(ctx, "/repo/web-checkout", corrected.Memory.MemoryID, "operator-forget-v2")
    requireNoError(t, err)
    if forgotten.Memory.Status != "deleted" {
        t.Fatalf("forget did not delete the named fact: %#v", forgotten)
    }
    requireNoError(t, store.RebuildProjection(ctx, "local", resolution.ContinuityID))
    for _, query := range []string{"checkout_eta_v2", "Which checkout flag should the staged release use?"} {
        if got := mustSearch(t, store, resolution.ContinuityID, query); len(got) != 0 {
            t.Fatalf("deleted fact returned for %q: %#v", query, got)
        }
    }
}

func TestGovernanceRejectsCrossWorkspaceCorrectionAndReplaysSource(t *testing.T) {
    ctx := context.Background()
    store := openTestStore(t)
    service := NewGovernanceService(store, "local")
    _, err := service.ConfirmWorkspace(ctx, "/repo/web-checkout")
    requireNoError(t, err)
    _, err = service.ConfirmWorkspace(ctx, "/repo/ops-console")
    requireNoError(t, err)
    source, err := service.AddSource(ctx, "/repo/ops-console", GovernanceWriteRequest{
        OperationID: "operator-ops-source",
        Content: "Run ops_exception_queue_refresh before handling incidents.",
        SourceRef: "fixture:operator:ops",
    })
    requireNoError(t, err)

    _, err = service.Correct(ctx, "/repo/web-checkout", source.Memory.MemoryID, GovernanceWriteRequest{
        OperationID: "operator-cross-scope-correction",
        Content: "Do not cross scope.",
    })
    if err == nil {
        t.Fatal("expected cross-workspace correction to fail")
    }

    replay, err := service.AddSource(ctx, "/repo/ops-console", GovernanceWriteRequest{
        OperationID: "operator-ops-source",
        Content: "Run ops_exception_queue_refresh before handling incidents.",
        SourceRef: "fixture:operator:ops",
    })
    requireNoError(t, err)
    if !replay.Observation.Replayed || !replay.Memory.Replayed || replay.Memory.MemoryID != source.Memory.MemoryID {
        t.Fatalf("source replay was not idempotent: first=%#v replay=%#v", source, replay)
    }
}
```

- [x] **Step 2: Run the focused runtime tests to verify RED**

Run:

```bash
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' go test -count=1 ./internal/runtime -run 'TestGovernance'
```

Expected: compile failure because `GovernanceService`, `GovernanceWriteRequest`, and its methods do not exist.

- [x] **Step 3: Add the smallest scoped facade and list query**

In `internal/runtime/postgres_store.go`, add the list type and query; it must filter by both tenant and continuity before returning any content:

```go
type GovernedMemory struct {
    ID                 string `json:"id"`
    LifecycleStatus    string `json:"lifecycle_status"`
    Content            string `json:"content"`
    SupersedesMemoryID string `json:"supersedes_memory_id,omitempty"`
}

func (s *Store) ListGovernedMemories(ctx context.Context, tenantID, continuityID string) ([]GovernedMemory, error) {
    rows, err := s.pool.Query(ctx, `
SELECT id::text, lifecycle_status, content, COALESCE(supersedes_memory_id::text, '')
FROM governed_memories
WHERE tenant_id = $1 AND continuity_id = $2::uuid
ORDER BY created_at ASC, id ASC`, tenantID, continuityID)
    if err != nil {
        return nil, fmt.Errorf("list governed memories: %w", err)
    }
    defer rows.Close()

    memories := make([]GovernedMemory, 0)
    for rows.Next() {
        var memory GovernedMemory
        if err := rows.Scan(&memory.ID, &memory.LifecycleStatus, &memory.Content, &memory.SupersedesMemoryID); err != nil {
            return nil, fmt.Errorf("scan governed memory: %w", err)
        }
        memories = append(memories, memory)
    }
    if err := rows.Err(); err != nil {
        return nil, fmt.Errorf("iterate governed memories: %w", err)
    }
    return memories, nil
}
```

Create `internal/runtime/governance.go` with one tenant-owned facade. It must call `CommitGovernedObservation` rather than composing SQL in the CLI:

```go
package runtime

import (
    "context"
    "fmt"
    "strings"
)

const localOperatorSourceRef = "operator:local"

type GovernanceWriteRequest struct {
    OperationID string
    Content     string
    SourceRef   string
}

type GovernanceService struct {
    store    *Store
    tenantID string
}

func NewGovernanceService(store *Store, tenantID string) *GovernanceService {
    return &GovernanceService{store: store, tenantID: strings.TrimSpace(tenantID)}
}

func (s *GovernanceService) ConfirmWorkspace(ctx context.Context, repoRoot string) (WorkspaceResolution, error) {
    if err := s.configured(); err != nil {
        return WorkspaceResolution{}, err
    }
    continuityID, err := s.store.ConfirmWorkspaceBinding(ctx, s.tenantID, repoRoot)
    if err != nil {
        return WorkspaceResolution{}, err
    }
    anchor, err := (WorkspaceAnchor{RepoRoot: repoRoot}).Normalized()
    if err != nil {
        return WorkspaceResolution{}, err
    }
    return WorkspaceResolution{Status: ResolutionResolved, ContinuityID: continuityID, RepoRoot: anchor.RepoRoot}, nil
}

func (s *GovernanceService) InspectWorkspace(ctx context.Context, repoRoot string) (WorkspaceResolution, error) {
    if err := s.configured(); err != nil {
        return WorkspaceResolution{}, err
    }
    return s.store.ResolveWorkspace(ctx, s.tenantID, WorkspaceAnchor{RepoRoot: repoRoot})
}

func (s *GovernanceService) ListWorkspaceMemories(ctx context.Context, repoRoot string) (WorkspaceResolution, []GovernedMemory, error) {
    resolution, err := s.confirmedWorkspace(ctx, repoRoot)
    if err != nil {
        return WorkspaceResolution{}, nil, err
    }
    memories, err := s.store.ListGovernedMemories(ctx, s.tenantID, resolution.ContinuityID)
    return resolution, memories, err
}

func (s *GovernanceService) AddSource(ctx context.Context, repoRoot string, write GovernanceWriteRequest) (GovernedObservationReceipt, error) {
    if strings.TrimSpace(write.SourceRef) == "" {
        return GovernedObservationReceipt{}, fmt.Errorf("source_ref is required for source facts")
    }
    return s.commit(ctx, repoRoot, CommitObservationRequest{OperationID: write.OperationID, Kind: ObservationKindSourceUpdate, Content: write.Content, SourceRef: write.SourceRef})
}

func (s *GovernanceService) Correct(ctx context.Context, repoRoot, memoryID string, write GovernanceWriteRequest) (GovernedObservationReceipt, error) {
    if strings.TrimSpace(memoryID) == "" {
        return GovernedObservationReceipt{}, fmt.Errorf("memory_id is required for correction")
    }
    return s.commit(ctx, repoRoot, CommitObservationRequest{OperationID: write.OperationID, Kind: ObservationKindUserCorrection, Content: write.Content, SourceRef: localOperatorSourceRef, SupersedesMemoryID: memoryID})
}

func (s *GovernanceService) Forget(ctx context.Context, repoRoot, memoryID, operationID string) (GovernedObservationReceipt, error) {
    if strings.TrimSpace(memoryID) == "" {
        return GovernedObservationReceipt{}, fmt.Errorf("memory_id is required for forget")
    }
    return s.commit(ctx, repoRoot, CommitObservationRequest{OperationID: operationID, Kind: ObservationKindForgetRequest, Content: "Operator requested deletion.", SourceRef: localOperatorSourceRef, TargetMemoryID: memoryID})
}

func (s *GovernanceService) commit(ctx context.Context, repoRoot string, request CommitObservationRequest) (GovernedObservationReceipt, error) {
    resolution, err := s.confirmedWorkspace(ctx, repoRoot)
    if err != nil {
        return GovernedObservationReceipt{}, err
    }
    return s.store.CommitGovernedObservation(ctx, s.tenantID, resolution.ContinuityID, request)
}

func (s *GovernanceService) confirmedWorkspace(ctx context.Context, repoRoot string) (WorkspaceResolution, error) {
    resolution, err := s.InspectWorkspace(ctx, repoRoot)
    if err != nil {
        return WorkspaceResolution{}, err
    }
    if resolution.Status != ResolutionResolved {
        return WorkspaceResolution{}, fmt.Errorf("workspace requires confirmation: %s", resolution.RepoRoot)
    }
    return resolution, nil
}

func (s *GovernanceService) configured() error {
    if s.store == nil || s.tenantID == "" {
        return fmt.Errorf("governance service is not configured")
    }
    return nil
}
```

- [x] **Step 4: Run the focused runtime tests to verify GREEN**

Run:

```bash
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' go test -count=1 ./internal/runtime -run 'TestGovernance|TestPrepareContext|TestDeletedMemory'
```

Expected: PASS. The new test proves source activation, explicitly targeted correction, deletion after rebuild, idempotency, and cross-workspace rejection; existing tests prove context and lifecycle behavior remains intact.

- [x] **Step 5: Commit the runtime facade**

```bash
git add internal/runtime/governance.go internal/runtime/governance_test.go internal/runtime/postgres_store.go
git commit -m "feat: add trusted workspace governance runtime"
```

### Task 2: Add Local Cobra Governance Commands

**Files:**
- Create: `internal/operatorcli/command.go`
- Create: `internal/operatorcli/command_test.go`
- Modify: `cmd/vermory/main.go`
- Modify: `cmd/vermory/main_test.go`

**Interfaces:**
- Consumes: `runtime.OpenStore`, `Store.Migrate`, `NewGovernanceService`, `WorkspaceResolution`, `GovernedMemory`, and `GovernedObservationReceipt` from Task 1.
- Produces: `vermory workspace confirm|inspect` and `vermory memory inspect|add-source|correct|forget` with JSON stdout receipts.

- [x] **Step 1: Write failing command tests**

Create `internal/operatorcli/command_test.go`. The helper opens and resets `VERMORY_TEST_DATABASE_URL` with exported runtime methods, then runs a fresh Cobra root for every invocation so flag state cannot leak between commands.

```go
func TestWorkspaceAndMemoryCommandsCompleteGovernedFlow(t *testing.T) {
    databaseURL := resetCommandStore(t)

    unknown := runJSONCommand(t, databaseURL, "workspace", "inspect", "--repo-root", "/repo/web-checkout")
    if unknown.Status != string(runtime.ResolutionNeedsConfirmation) || unknown.ContinuityID != "" {
        t.Fatalf("unknown workspace was attached: %#v", unknown)
    }

    confirmed := runJSONCommand(t, databaseURL, "workspace", "confirm", "--repo-root", "/repo/web-checkout")
    if confirmed.Status != string(runtime.ResolutionResolved) || confirmed.ContinuityID == "" {
        t.Fatalf("workspace was not confirmed: %#v", confirmed)
    }

    source := runJSONCommand(t, databaseURL, "memory", "add-source", "--repo-root", "/repo/web-checkout", "--operation-id", "cli-source-v1", "--source-ref", "fixture:cli:v1", "--content", "Use checkout_eta_v1 for the staged checkout release.")
    if source.MemoryStatus != "active" || source.MemoryID == "" {
        t.Fatalf("source receipt=%#v", source)
    }

    corrected := runJSONCommand(t, databaseURL, "memory", "correct", "--repo-root", "/repo/web-checkout", "--operation-id", "cli-correct-v2", "--memory-id", source.MemoryID, "--content", "Use checkout_eta_v2 for the staged checkout release.")
    if corrected.MemoryStatus != "active" || corrected.MemoryID == "" {
        t.Fatalf("correction receipt=%#v", corrected)
    }

    listed := runMemoryListCommand(t, databaseURL, "/repo/web-checkout")
    if !containsMemory(listed.Memories, source.MemoryID, "superseded") || !containsMemory(listed.Memories, corrected.MemoryID, "active") {
        t.Fatalf("unexpected scoped memory list: %#v", listed)
    }

    forgotten := runJSONCommand(t, databaseURL, "memory", "forget", "--repo-root", "/repo/web-checkout", "--operation-id", "cli-forget-v2", "--memory-id", corrected.MemoryID)
    if forgotten.MemoryStatus != "deleted" || forgotten.MemoryID != corrected.MemoryID {
        t.Fatalf("forget receipt=%#v", forgotten)
    }

    assertNoActiveCheckoutFact(t, databaseURL, confirmed.ContinuityID)
}

func TestMemoryCommandsRejectUnconfirmedWorkspace(t *testing.T) {
    databaseURL := resetCommandStore(t)
    err := runCommand(t, databaseURL, "memory", "add-source", "--repo-root", "/repo/unconfirmed", "--operation-id", "cli-reject", "--source-ref", "fixture:reject", "--content", "Must not persist.")
    if err == nil || !strings.Contains(err.Error(), "workspace requires confirmation") {
        t.Fatalf("unexpected mutation error: %v", err)
    }
}
```

Add the test helpers in the same file. They invoke the public commands exactly
as a local operator would, parse stdout JSON, and use direct runtime reads
only for the final projection assertion:

```go
type commandReceipt struct {
    Status       string `json:"status"`
    ContinuityID string `json:"continuity_id"`
    MemoryID     string `json:"memory_id"`
    MemoryStatus string `json:"memory_status"`
    Replayed     bool   `json:"replayed"`
}

type commandMemoryList struct {
    ContinuityID string                   `json:"continuity_id"`
    Memories     []runtime.GovernedMemory `json:"memories"`
}

func resetCommandStore(t *testing.T) string {
    t.Helper()
    databaseURL := os.Getenv("VERMORY_TEST_DATABASE_URL")
    if databaseURL == "" {
        t.Skip("VERMORY_TEST_DATABASE_URL is not set")
    }
    store, err := runtime.OpenStore(context.Background(), databaseURL)
    if err != nil {
        t.Fatal(err)
    }
    t.Cleanup(store.Close)
    if err := store.Migrate(context.Background()); err != nil {
        t.Fatal(err)
    }
    if err := store.ResetForTest(context.Background()); err != nil {
        t.Fatal(err)
    }
    return databaseURL
}

func runCommand(t *testing.T, databaseURL string, args ...string) error {
    t.Helper()
    root := &cobra.Command{Use: "vermory", SilenceErrors: true, SilenceUsage: true}
    root.AddCommand(NewWorkspaceCommand(), NewMemoryCommand())
    root.SetOut(&bytes.Buffer{})
    root.SetErr(&bytes.Buffer{})
    root.SetArgs(append(args, "--database-url", databaseURL, "--tenant-id", "local"))
    return root.Execute()
}

func runJSONCommand(t *testing.T, databaseURL string, args ...string) commandReceipt {
    t.Helper()
    var output bytes.Buffer
    root := &cobra.Command{Use: "vermory", SilenceErrors: true, SilenceUsage: true}
    root.AddCommand(NewWorkspaceCommand(), NewMemoryCommand())
    root.SetOut(&output)
    root.SetErr(&bytes.Buffer{})
    root.SetArgs(append(args, "--database-url", databaseURL, "--tenant-id", "local"))
    if err := root.Execute(); err != nil {
        t.Fatal(err)
    }
    var receipt commandReceipt
    if err := json.Unmarshal(output.Bytes(), &receipt); err != nil {
        t.Fatal(err)
    }
    return receipt
}

func runMemoryListCommand(t *testing.T, databaseURL, repoRoot string) commandMemoryList {
    t.Helper()
    var output bytes.Buffer
    root := &cobra.Command{Use: "vermory", SilenceErrors: true, SilenceUsage: true}
    root.AddCommand(NewWorkspaceCommand(), NewMemoryCommand())
    root.SetOut(&output)
    root.SetErr(&bytes.Buffer{})
    root.SetArgs([]string{"memory", "inspect", "--repo-root", repoRoot, "--database-url", databaseURL, "--tenant-id", "local"})
    if err := root.Execute(); err != nil {
        t.Fatal(err)
    }
    var listed commandMemoryList
    if err := json.Unmarshal(output.Bytes(), &listed); err != nil {
        t.Fatal(err)
    }
    return listed
}

func containsMemory(memories []runtime.GovernedMemory, id, status string) bool {
    for _, memory := range memories {
        if memory.ID == id && memory.LifecycleStatus == status {
            return true
        }
    }
    return false
}

func assertNoActiveCheckoutFact(t *testing.T, databaseURL, continuityID string) {
    t.Helper()
    store, err := runtime.OpenStore(context.Background(), databaseURL)
    if err != nil {
        t.Fatal(err)
    }
    t.Cleanup(store.Close)
    if err := store.RebuildProjection(context.Background(), "local", continuityID); err != nil {
        t.Fatal(err)
    }
    matches, err := store.SearchActiveMemory(context.Background(), "local", continuityID, "checkout_eta_v2", 6)
    if err != nil {
        t.Fatal(err)
    }
    if len(matches) != 0 {
        t.Fatalf("deleted CLI fact returned: %#v", matches)
    }
}
```

The existing independent `internal/mcpserver.TestServerAdvertisesOnlyNormalFlowTools`
remains the MCP-boundary test. Do not add an `operatorcli` dependency on
`mcpserver`.

In `cmd/vermory/main_test.go`, add a registration test:

```go
func TestOperatorCommandsAreRegistered(t *testing.T) {
    names := map[string]bool{}
    for _, command := range newRootCommand().Commands() {
        names[command.Name()] = true
    }
    for _, want := range []string{"workspace", "memory"} {
        if !names[want] {
            t.Fatalf("expected root command %q", want)
        }
    }
}

func TestOperatorMemoryForgetHasNoFreeTextFlag(t *testing.T) {
    root := newRootCommand()
    for _, parent := range root.Commands() {
        if parent.Name() != "memory" {
            continue
        }
        for _, child := range parent.Commands() {
            if child.Name() != "forget" {
                continue
            }
            if child.Flags().Lookup("reason") != nil || child.Flags().Lookup("content") != nil {
                t.Fatal("forget command must not accept free-text deletion content")
            }
            return
        }
    }
    t.Fatal("expected memory forget command")
}
```

- [x] **Step 2: Run the command tests to verify RED**

Run:

```bash
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' go test -count=1 ./internal/operatorcli ./cmd/vermory
```

Expected: package or command-registration failure because `internal/operatorcli` and the root commands do not exist.

- [x] **Step 3: Implement the focused command package**

Create `internal/operatorcli/command.go`. Use JSON to keep receipts parseable even when a fact contains whitespace or a newline:

```go
package operatorcli

import (
    "context"
    "encoding/json"
    "fmt"
    "strings"

    "vermory/internal/runtime"

    "github.com/spf13/cobra"
)

type connectionOptions struct {
    databaseURL string
    tenantID    string
}

type workspaceOutput struct {
    Status       string `json:"status"`
    ContinuityID string `json:"continuity_id,omitempty"`
    RepoRoot     string `json:"repo_root"`
}

type mutationOutput struct {
    ContinuityID  string `json:"continuity_id"`
    ObservationID string `json:"observation_id"`
    MemoryID      string `json:"memory_id"`
    MemoryStatus  string `json:"memory_status"`
    Replayed      bool   `json:"replayed"`
}

type memoryListOutput struct {
    ContinuityID string                   `json:"continuity_id"`
    RepoRoot     string                   `json:"repo_root"`
    Memories     []runtime.GovernedMemory `json:"memories"`
}

func NewWorkspaceCommand() *cobra.Command {
    options := connectionOptions{}
    command := &cobra.Command{Use: "workspace", Short: "Manage trusted workspace bindings"}
    addConnectionFlags(command, &options)

    var confirmRoot string
    confirm := &cobra.Command{Use: "confirm", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
        return withGovernance(cmd.Context(), options, func(service *runtime.GovernanceService) error {
            resolution, err := service.ConfirmWorkspace(cmd.Context(), confirmRoot)
            if err != nil {
                return err
            }
            return writeJSON(cmd, workspaceOutput{Status: string(resolution.Status), ContinuityID: resolution.ContinuityID, RepoRoot: resolution.RepoRoot})
        })
    }}
    confirm.Flags().StringVar(&confirmRoot, "repo-root", "", "absolute workspace root")
    _ = confirm.MarkFlagRequired("repo-root")

    var inspectRoot string
    inspect := &cobra.Command{Use: "inspect", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
        return withGovernance(cmd.Context(), options, func(service *runtime.GovernanceService) error {
            resolution, err := service.InspectWorkspace(cmd.Context(), inspectRoot)
            if err != nil {
                return err
            }
            return writeJSON(cmd, workspaceOutput{Status: string(resolution.Status), ContinuityID: resolution.ContinuityID, RepoRoot: resolution.RepoRoot})
        })
    }}
    inspect.Flags().StringVar(&inspectRoot, "repo-root", "", "absolute workspace root")
    _ = inspect.MarkFlagRequired("repo-root")
    command.AddCommand(confirm, inspect)
    return command
}

func NewMemoryCommand() *cobra.Command {
    options := connectionOptions{}
    command := &cobra.Command{Use: "memory", Short: "Apply explicit local memory governance"}
    addConnectionFlags(command, &options)

    var inspectRoot string
    inspect := &cobra.Command{Use: "inspect", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
        return withGovernance(cmd.Context(), options, func(service *runtime.GovernanceService) error {
            resolution, memories, err := service.ListWorkspaceMemories(cmd.Context(), inspectRoot)
            if err != nil {
                return err
            }
            return writeJSON(cmd, memoryListOutput{ContinuityID: resolution.ContinuityID, RepoRoot: resolution.RepoRoot, Memories: memories})
        })
    }}
    inspect.Flags().StringVar(&inspectRoot, "repo-root", "", "absolute workspace root")
    _ = inspect.MarkFlagRequired("repo-root")

    var sourceRoot, sourceOperationID, sourceContent, sourceRef string
    addSource := &cobra.Command{Use: "add-source", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
        return withGovernance(cmd.Context(), options, func(service *runtime.GovernanceService) error {
            receipt, err := service.AddSource(cmd.Context(), sourceRoot, runtime.GovernanceWriteRequest{OperationID: sourceOperationID, Content: sourceContent, SourceRef: sourceRef})
            if err != nil {
                return err
            }
            return writeMutationJSON(cmd, service, sourceRoot, receipt)
        })
    }}
    addSource.Flags().StringVar(&sourceRoot, "repo-root", "", "absolute workspace root")
    addSource.Flags().StringVar(&sourceOperationID, "operation-id", "", "idempotency key")
    addSource.Flags().StringVar(&sourceContent, "content", "", "trusted source fact")
    addSource.Flags().StringVar(&sourceRef, "source-ref", "", "opaque source reference")
    markRequired(addSource, "repo-root", "operation-id", "content", "source-ref")

    var correctRoot, correctOperationID, correctMemoryID, correctContent string
    correct := &cobra.Command{Use: "correct", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
        return withGovernance(cmd.Context(), options, func(service *runtime.GovernanceService) error {
            receipt, err := service.Correct(cmd.Context(), correctRoot, correctMemoryID, runtime.GovernanceWriteRequest{OperationID: correctOperationID, Content: correctContent})
            if err != nil {
                return err
            }
            return writeMutationJSON(cmd, service, correctRoot, receipt)
        })
    }}
    correct.Flags().StringVar(&correctRoot, "repo-root", "", "absolute workspace root")
    correct.Flags().StringVar(&correctOperationID, "operation-id", "", "idempotency key")
    correct.Flags().StringVar(&correctMemoryID, "memory-id", "", "active memory to supersede")
    correct.Flags().StringVar(&correctContent, "content", "", "replacement fact")
    markRequired(correct, "repo-root", "operation-id", "memory-id", "content")

    var forgetRoot, forgetOperationID, forgetMemoryID string
    forget := &cobra.Command{Use: "forget", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
        return withGovernance(cmd.Context(), options, func(service *runtime.GovernanceService) error {
            receipt, err := service.Forget(cmd.Context(), forgetRoot, forgetMemoryID, forgetOperationID)
            if err != nil {
                return err
            }
            return writeMutationJSON(cmd, service, forgetRoot, receipt)
        })
    }}
    forget.Flags().StringVar(&forgetRoot, "repo-root", "", "absolute workspace root")
    forget.Flags().StringVar(&forgetOperationID, "operation-id", "", "idempotency key")
    forget.Flags().StringVar(&forgetMemoryID, "memory-id", "", "memory to redact")
    markRequired(forget, "repo-root", "operation-id", "memory-id")

    command.AddCommand(inspect, addSource, correct, forget)
    return command
}

func addConnectionFlags(command *cobra.Command, options *connectionOptions) {
    command.PersistentFlags().StringVar(&options.databaseURL, "database-url", "", "PostgreSQL connection URL")
    command.PersistentFlags().StringVar(&options.tenantID, "tenant-id", "", "server-owned tenant identifier")
    _ = command.MarkPersistentFlagRequired("database-url")
    _ = command.MarkPersistentFlagRequired("tenant-id")
}

func markRequired(command *cobra.Command, names ...string) {
    for _, name := range names {
        _ = command.MarkFlagRequired(name)
    }
}

Each parent command has persistent `--database-url` and `--tenant-id` flags,
and each leaf command has a required `--repo-root`. `add-source` also marks
`--operation-id`, `--content`, and `--source-ref` required. `correct` marks
`--operation-id`, `--memory-id`, and `--content` required. `forget` marks
`--operation-id` and `--memory-id` required; it deliberately has no free-text
flag. Use one helper that rejects blank connection settings, opens and
migrates the store, closes it with `defer`, and passes a
`runtime.NewGovernanceService(store, options.tenantID)` to the leaf action:

```go
func withGovernance(ctx context.Context, options connectionOptions, run func(*runtime.GovernanceService) error) error {
    if strings.TrimSpace(options.databaseURL) == "" {
        return fmt.Errorf("--database-url is required")
    }
    if strings.TrimSpace(options.tenantID) == "" {
        return fmt.Errorf("--tenant-id is required")
    }
    store, err := runtime.OpenStore(ctx, options.databaseURL)
    if err != nil {
        return err
    }
    defer store.Close()
    if err := store.Migrate(ctx); err != nil {
        return err
    }
    return run(runtime.NewGovernanceService(store, options.tenantID))
}

func writeJSON(cmd *cobra.Command, value any) error {
    return json.NewEncoder(cmd.OutOrStdout()).Encode(value)
}

func writeMutationJSON(cmd *cobra.Command, service *runtime.GovernanceService, repoRoot string, receipt runtime.GovernedObservationReceipt) error {
    resolution, err := service.InspectWorkspace(cmd.Context(), repoRoot)
    if err != nil {
        return err
    }
    return writeJSON(cmd, mutationOutput{
        ContinuityID:  resolution.ContinuityID,
        ObservationID: receipt.Observation.ObservationID,
        MemoryID:      receipt.Memory.MemoryID,
        MemoryStatus:  receipt.Memory.Status,
        Replayed:      receipt.Observation.Replayed || receipt.Memory.Replayed,
    })
}
```

In `cmd/vermory/main.go`, import `vermory/internal/operatorcli` and register
the two parents without changing `mcp-stdio`:

```go
rootCmd.AddCommand(operatorcli.NewWorkspaceCommand())
rootCmd.AddCommand(operatorcli.NewMemoryCommand())
```

- [x] **Step 4: Run the command, runtime, and MCP tests to verify GREEN**

Run:

```bash
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' go test -p 1 -count=1 ./internal/runtime ./internal/operatorcli ./internal/mcpserver ./cmd/vermory
```

Expected: PASS. The command test proves the full source/correct/forget flow;
the MCP package still proves that only two unprivileged tools are advertised.

- [x] **Step 5: Commit the CLI surface**

```bash
git add internal/operatorcli/command.go internal/operatorcli/command_test.go cmd/vermory/main.go cmd/vermory/main_test.go
git commit -m "feat: add local workspace governance cli"
```

### Task 3: Document and Rehearse the Local Operator Flow

**Files:**
- Create: `docs/integrations/local-operator-workspace-slice.md`
- Modify: `README.zh-CN.md`

**Interfaces:**
- Consumes: the command names and JSON receipt fields implemented in Task 2 and the existing `vermory mcp-stdio` transport.
- Produces: an exact local replay guide that distinguishes scripted contract evidence from a real client run.

- [x] **Step 1: Add the replay guide and concise README entry**

Create `docs/integrations/local-operator-workspace-slice.md` with this
replay, substituting a dedicated disposable local database and a
non-sensitive fixture root:

```bash
go build -o ./bin/vermory ./cmd/vermory

./bin/vermory workspace confirm \
  --database-url 'postgresql:///vermory_w03?host=/tmp' \
  --tenant-id local-w03 \
  --repo-root /fixtures/vermory-w03

./bin/vermory memory add-source \
  --database-url 'postgresql:///vermory_w03?host=/tmp' \
  --tenant-id local-w03 \
  --repo-root /fixtures/vermory-w03 \
  --operation-id w03-source-v1 \
  --source-ref fixture:W03:source-v1 \
  --content 'Use checkout_eta_v1 for the staged checkout release.'

./bin/vermory memory inspect \
  --database-url 'postgresql:///vermory_w03?host=/tmp' \
  --tenant-id local-w03 \
  --repo-root /fixtures/vermory-w03
```

The guide then uses the returned v1 `memory_id` in `memory correct`, uses the
returned v2 `memory_id` in `memory forget`, and gives the exact
`mcp-stdio` registration and `prepare_context`/`commit_observation` evidence
requirements. It must state all of the following:

- scripted Go tests prove the command and lifecycle contract, not a real
  coding client run;
- a real replay needs both a `memory_deliveries` row and an `agent_result`
  observation plus a repository artifact and its verification output;
- no actual source text, credentials, private paths, or unredacted client
  transcript belongs in the public fixture or Git history;
- a Codex usage-limit failure is retained as unavailable evidence and is not
  replaced by a scripted pass or a Grok result.

Add one short Chinese README section that links to this guide and explains
that operator commands are local, explicit governance controls rather than
ordinary AI tools.

- [x] **Step 2: Run the full verification suite and a local command replay**

Run:

```bash
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' go test -p 1 -count=1 ./...
VERMORY_TEST_DATABASE_URL='postgresql:///vermory_test?host=/tmp' go test -race -p 1 ./internal/runtime ./internal/operatorcli ./internal/mcpserver -count=1
go vet ./...
go mod tidy -diff
go build -o /tmp/vermory-operator-check ./cmd/vermory
git diff --check
```

Then follow the guide against a disposable database and fixture root. Preserve
only redacted runtime receipts under ignored `artifacts/runtime/W03/`; do not
commit generated database state, tool transcripts, user paths, or credentials.

Expected: all commands succeed, test suites pass, `memory forget` exposes no
free-text flag, and a freshly rebuilt projection contains no deleted W03 fact.

- [x] **Step 3: Commit documentation and evidence-safe replay assets**

```bash
git add docs/integrations/local-operator-workspace-slice.md README.zh-CN.md cmd/vermory/main_test.go
git commit -m "docs: add local governance replay guide"
```

## Plan Self-Review

- Spec coverage: Task 1 implements tenant-owned confirmation, inspection,
  scoped memory listing, explicit source/correction/forget actions, atomic
  store reuse, and no-free-text deletion. Task 2 exposes exactly the six
  approved local commands with JSON receipts while leaving MCP unchanged.
  Task 3 supplies the public runbook, a direct command-boundary test, and a
  real replay protocol.
- Placeholder scan: no task relies on unspecified command names, schema
  changes, generic policy engines, or later-defined interfaces.
- Type consistency: Task 1 defines every runtime type used by Task 2;
  Task 2 defines every command consumed by Task 3. Mutation output maps
  directly to `GovernedObservationReceipt` and never exposes a new authority
  path to MCP.
- Scope: bridge actions, conversation/global continuity, HTTP, UI,
  authentication, and new retrieval backends remain explicitly excluded.
