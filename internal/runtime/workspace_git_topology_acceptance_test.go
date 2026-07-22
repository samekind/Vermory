package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"vermory/internal/resolver"
)

type workspaceGitTopologyCase struct {
	Version                      string   `json:"version"`
	ID                           string   `json:"id"`
	RealityCaseID                string   `json:"reality_case_id"`
	TenantID                     string   `json:"tenant_id"`
	OtherTenantID                string   `json:"other_tenant_id"`
	FilesystemNamespace          string   `json:"filesystem_namespace"`
	AlternateFilesystemNamespace string   `json:"alternate_filesystem_namespace"`
	CurrentFact                  string   `json:"current_fact"`
	OtherTenantFact              string   `json:"other_tenant_fact"`
	HardGateCount                int      `json:"hard_gate_count"`
	HardGates                    []string `json:"hard_gates"`
}

type workspaceGitTopology struct {
	primaryRoot  string
	nestedCWD    string
	worktreeRoot string
	sameNameRoot string
	cloneRoot    string
	movedRoot    string
}

func TestW31RealGitWorkspaceTopologyCaseIsFrozen(t *testing.T) {
	manifest := loadWorkspaceGitTopologyCase(t)
	if manifest.Version != "1" || manifest.ID != "W31-real-git-workspace-topology" ||
		manifest.RealityCaseID != "W05-trusted-workspace-attachment" {
		t.Fatalf("unexpected W31 identity: %#v", manifest)
	}
	if manifest.TenantID == "" || manifest.OtherTenantID == "" || manifest.TenantID == manifest.OtherTenantID {
		t.Fatalf("W31 tenant isolation contract is invalid: %#v", manifest)
	}
	if manifest.FilesystemNamespace == "" || manifest.AlternateFilesystemNamespace == "" ||
		manifest.FilesystemNamespace == manifest.AlternateFilesystemNamespace {
		t.Fatalf("W31 filesystem namespace contract is invalid: %#v", manifest)
	}
	if manifest.HardGateCount != 18 || len(manifest.HardGates) != manifest.HardGateCount {
		t.Fatalf("W31 hard-gate count drifted: count=%d gates=%d", manifest.HardGateCount, len(manifest.HardGates))
	}
	seen := make(map[string]struct{}, len(manifest.HardGates))
	for _, gate := range manifest.HardGates {
		if strings.TrimSpace(gate) == "" {
			t.Fatal("W31 hard gate cannot be empty")
		}
		if _, exists := seen[gate]; exists {
			t.Fatalf("W31 hard gate is duplicated: %q", gate)
		}
		seen[gate] = struct{}{}
	}
}

func TestW31RealGitWorkspaceTopologyAcceptance(t *testing.T) {
	manifest := loadWorkspaceGitTopologyCase(t)
	topology := createWorkspaceGitTopology(t)
	ctx := context.Background()

	primary := probeWorkspace(t, topology.nestedCWD, manifest.FilesystemNamespace)
	worktree := probeWorkspace(t, topology.worktreeRoot, manifest.FilesystemNamespace)
	sameName := probeWorkspace(t, topology.sameNameRoot, manifest.FilesystemNamespace)
	clone := probeWorkspace(t, topology.cloneRoot, manifest.FilesystemNamespace)
	if primary.RepoRoot != topology.primaryRoot || primary.CWD != topology.nestedCWD {
		t.Fatalf("nested cwd did not resolve to the primary root: %#v", primary)
	}
	if primary.RepoRoot == worktree.RepoRoot || primary.Fingerprint == worktree.Fingerprint {
		t.Fatalf("worktree collapsed into the primary attachment: primary=%#v worktree=%#v", primary, worktree)
	}
	if primary.GitCommonFingerprint != worktree.GitCommonFingerprint {
		t.Fatalf("linked worktree lost common Git evidence: primary=%#v worktree=%#v", primary, worktree)
	}
	for label, attachment := range map[string]resolver.WorkspaceAttachment{"same-name": sameName, "clone": clone} {
		if attachment.RepoRoot == primary.RepoRoot || attachment.Fingerprint == primary.Fingerprint ||
			attachment.GitCommonFingerprint == primary.GitCommonFingerprint {
			t.Fatalf("%s repository was treated as the primary Git identity: primary=%#v candidate=%#v", label, primary, attachment)
		}
	}

	store := openTestStore(t)
	primaryAnchor := WorkspaceAnchor{RepoRoot: primary.RepoRoot, FilesystemNamespace: manifest.FilesystemNamespace}
	primaryGovernance := NewGovernanceServiceWithNamespace(store, manifest.TenantID, manifest.FilesystemNamespace)
	primaryResolution, err := primaryGovernance.ConfirmWorkspaceAnchor(ctx, primaryAnchor)
	requireNoError(t, err)
	seed, err := primaryGovernance.AddSource(ctx, primary.RepoRoot, GovernanceWriteRequest{
		OperationID: "w31-seed-primary",
		MemoryKey:   "continuation_marker",
		Content:     manifest.CurrentFact,
		SourceRef:   "fixture:W05:real-git-topology",
	})
	requireNoError(t, err)
	if seed.Memory.Status != "active" {
		t.Fatalf("W31 current fact is not active: %#v", seed)
	}

	targetService := NewService(store, manifest.TenantID)
	assertWorkspaceAbstains(t, targetService, "w31-unadopted-worktree", WorkspaceAnchor{
		RepoRoot: worktree.RepoRoot, FilesystemNamespace: manifest.FilesystemNamespace,
	})
	assertWorkspaceAbstains(t, targetService, "w31-same-name", WorkspaceAnchor{
		RepoRoot: sameName.RepoRoot, FilesystemNamespace: manifest.FilesystemNamespace,
	})
	assertWorkspaceAbstains(t, targetService, "w31-clone", WorkspaceAnchor{
		RepoRoot: clone.RepoRoot, FilesystemNamespace: manifest.FilesystemNamespace,
	})
	assertWorkspaceAbstains(t, targetService, "w31-other-namespace", WorkspaceAnchor{
		RepoRoot: primary.RepoRoot, FilesystemNamespace: manifest.AlternateFilesystemNamespace,
	})

	otherGovernance := NewGovernanceServiceWithNamespace(store, manifest.OtherTenantID, manifest.FilesystemNamespace)
	_, err = otherGovernance.ConfirmWorkspaceAnchor(ctx, primaryAnchor)
	requireNoError(t, err)
	_, err = otherGovernance.AddSource(ctx, primary.RepoRoot, GovernanceWriteRequest{
		OperationID: "w31-seed-other-tenant",
		MemoryKey:   "continuation_marker",
		Content:     manifest.OtherTenantFact,
		SourceRef:   "fixture:W05:other-tenant",
	})
	requireNoError(t, err)
	otherPrepared, err := NewService(store, manifest.OtherTenantID).PrepareContext(ctx, PrepareContextRequest{
		OperationID: "w31-other-tenant-prepare",
		Workspace:   primaryAnchor,
		Task:        "Which continuation marker is current?",
	})
	requireNoError(t, err)
	requireContains(t, otherPrepared.Context, manifest.OtherTenantFact)
	requireNotContains(t, otherPrepared.Context, manifest.CurrentFact)

	bridges := NewBridgeService(store, manifest.TenantID)
	adopted, err := bridges.AdoptWorkspaceAnchor(ctx, AdoptWorkspaceAnchorRequest{
		OperationID:         "w31-adopt-worktree",
		ExistingRepoRoot:    primary.RepoRoot,
		NewRepoRoot:         worktree.RepoRoot,
		ExistingNamespaceID: manifest.FilesystemNamespace,
		NewNamespaceID:      manifest.FilesystemNamespace,
	})
	requireNoError(t, err)
	if adopted.Action != BridgeActionAdopt || adopted.SourceContinuityID != primaryResolution.ContinuityID ||
		adopted.TargetContinuityID != primaryResolution.ContinuityID {
		t.Fatalf("worktree adopt changed continuity: %#v", adopted)
	}
	worktreePrepared := requireWorkspaceFact(t, targetService, "w31-worktree-prepare", WorkspaceAnchor{
		RepoRoot: worktree.RepoRoot, FilesystemNamespace: manifest.FilesystemNamespace,
	}, manifest.CurrentFact)
	requireNotContains(t, worktreePrepared.Context, manifest.OtherTenantFact)

	adoptReplay, err := bridges.AdoptWorkspaceAnchor(ctx, AdoptWorkspaceAnchorRequest{
		OperationID:         "w31-adopt-worktree",
		ExistingRepoRoot:    primary.RepoRoot,
		NewRepoRoot:         worktree.RepoRoot,
		ExistingNamespaceID: manifest.FilesystemNamespace,
		NewNamespaceID:      manifest.FilesystemNamespace,
	})
	requireNoError(t, err)
	if !adoptReplay.Replayed || adoptReplay.ID != adopted.ID {
		t.Fatalf("worktree adopt replay changed operation: first=%#v replay=%#v", adopted, adoptReplay)
	}

	reversedAdopt, err := bridges.Reverse(ctx, ReverseBridgeRequest{
		OperationID: "w31-reverse-adopt", BridgeID: adopted.ID,
	})
	requireNoError(t, err)
	if reversedAdopt.Status != BridgeStatusReversed {
		t.Fatalf("worktree adopt did not reverse: %#v", reversedAdopt)
	}
	assertWorkspaceAbstains(t, targetService, "w31-worktree-after-reverse", WorkspaceAnchor{
		RepoRoot: worktree.RepoRoot, FilesystemNamespace: manifest.FilesystemNamespace,
	})
	requireWorkspaceFact(t, targetService, "w31-primary-after-adopt-reverse", primaryAnchor, manifest.CurrentFact)

	removeGitWorktree(t, topology.primaryRoot, topology.worktreeRoot)
	if err := os.MkdirAll(filepath.Dir(topology.movedRoot), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(topology.primaryRoot, topology.movedRoot); err != nil {
		t.Fatalf("move primary checkout: %v", err)
	}
	moved := probeWorkspace(t, topology.movedRoot, manifest.FilesystemNamespace)
	assertWorkspaceAbstains(t, targetService, "w31-moved-before-rebind", WorkspaceAnchor{
		RepoRoot: moved.RepoRoot, FilesystemNamespace: manifest.FilesystemNamespace,
	})

	rebound, err := bridges.RebindWorkspace(ctx, RebindWorkspaceRequest{
		OperationID:    "w31-rebind-moved-checkout",
		OldRepoRoot:    primary.RepoRoot,
		NewRepoRoot:    moved.RepoRoot,
		OldNamespaceID: manifest.FilesystemNamespace,
		NewNamespaceID: manifest.FilesystemNamespace,
	})
	requireNoError(t, err)
	if rebound.Action != BridgeActionRebind || rebound.SourceContinuityID != primaryResolution.ContinuityID ||
		rebound.TargetContinuityID != primaryResolution.ContinuityID {
		t.Fatalf("moved checkout rebind changed continuity: %#v", rebound)
	}
	requireWorkspaceFact(t, targetService, "w31-moved-after-rebind", WorkspaceAnchor{
		RepoRoot: moved.RepoRoot, FilesystemNamespace: manifest.FilesystemNamespace,
	}, manifest.CurrentFact)
	assertWorkspaceAbstains(t, targetService, "w31-old-after-rebind", primaryAnchor)

	rebindReplay, err := bridges.RebindWorkspace(ctx, RebindWorkspaceRequest{
		OperationID:    "w31-rebind-moved-checkout",
		OldRepoRoot:    primary.RepoRoot,
		NewRepoRoot:    moved.RepoRoot,
		OldNamespaceID: manifest.FilesystemNamespace,
		NewNamespaceID: manifest.FilesystemNamespace,
	})
	requireNoError(t, err)
	if !rebindReplay.Replayed || rebindReplay.ID != rebound.ID {
		t.Fatalf("moved checkout rebind replay changed operation: first=%#v replay=%#v", rebound, rebindReplay)
	}

	reversedRebind, err := bridges.Reverse(ctx, ReverseBridgeRequest{
		OperationID: "w31-reverse-rebind", BridgeID: rebound.ID,
	})
	requireNoError(t, err)
	if reversedRebind.Status != BridgeStatusReversed {
		t.Fatalf("moved checkout rebind did not reverse: %#v", reversedRebind)
	}
	requireWorkspaceFact(t, targetService, "w31-old-after-rebind-reverse", primaryAnchor, manifest.CurrentFact)
	assertWorkspaceAbstains(t, targetService, "w31-new-after-rebind-reverse", WorkspaceAnchor{
		RepoRoot: moved.RepoRoot, FilesystemNamespace: manifest.FilesystemNamespace,
	})

	finalRebind, err := bridges.RebindWorkspace(ctx, RebindWorkspaceRequest{
		OperationID:    "w31-final-rebind",
		OldRepoRoot:    primary.RepoRoot,
		NewRepoRoot:    moved.RepoRoot,
		OldNamespaceID: manifest.FilesystemNamespace,
		NewNamespaceID: manifest.FilesystemNamespace,
	})
	requireNoError(t, err)
	if finalRebind.ID == rebound.ID || finalRebind.Replayed {
		t.Fatalf("final rebind reused the reversed operation: first=%#v final=%#v", rebound, finalRebind)
	}
	requireWorkspaceFact(t, targetService, "w31-final-moved-prepare", WorkspaceAnchor{
		RepoRoot: moved.RepoRoot, FilesystemNamespace: manifest.FilesystemNamespace,
	}, manifest.CurrentFact)
}

func loadWorkspaceGitTopologyCase(t *testing.T) workspaceGitTopologyCase {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	payload, err := os.ReadFile(filepath.Join(root, "runtime", "cases", "W31-real-git-workspace-topology", "case.json"))
	if err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	var manifest workspaceGitTopologyCase
	if err := decoder.Decode(&manifest); err != nil {
		t.Fatal(err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		t.Fatalf("W31 case contains trailing JSON data: %v", err)
	}
	return manifest
}

func createWorkspaceGitTopology(t *testing.T) workspaceGitTopology {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Fatalf("W31 requires the real git executable: %v", err)
	}
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	primaryRoot := filepath.Join(root, "primary", "Vermory")
	initWorkspaceGitRepository(t, primaryRoot, "primary\n")
	nestedCWD := filepath.Join(primaryRoot, "internal", "runtime")
	if err := os.MkdirAll(nestedCWD, 0o755); err != nil {
		t.Fatal(err)
	}
	worktreeRoot := filepath.Join(root, "worktrees", "Vermory")
	runWorkspaceGit(t, primaryRoot, "worktree", "add", "-b", "w31-worktree", worktreeRoot)

	sameNameRoot := filepath.Join(root, "unrelated", "Vermory")
	initWorkspaceGitRepository(t, sameNameRoot, "unrelated\n")
	cloneRoot := filepath.Join(root, "clone", "Vermory")
	runWorkspaceGit(t, root, "clone", primaryRoot, cloneRoot)
	return workspaceGitTopology{
		primaryRoot:  primaryRoot,
		nestedCWD:    nestedCWD,
		worktreeRoot: worktreeRoot,
		sameNameRoot: sameNameRoot,
		cloneRoot:    cloneRoot,
		movedRoot:    filepath.Join(root, "moved", "Vermory"),
	}
}

func initWorkspaceGitRepository(t *testing.T, root, readme string) {
	t.Helper()
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	runWorkspaceGit(t, root, "init")
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte(readme), 0o644); err != nil {
		t.Fatal(err)
	}
	runWorkspaceGit(t, root, "add", "README.md")
	runWorkspaceGit(t, root, "-c", "user.name=Vermory Test", "-c", "user.email=test@example.invalid", "commit", "-m", "initial")
}

func probeWorkspace(t *testing.T, cwd, namespace string) resolver.WorkspaceAttachment {
	t.Helper()
	attachment, err := resolver.ProbeGitWorkspace(context.Background(), cwd, namespace)
	if err != nil {
		t.Fatal(err)
	}
	return attachment
}

func assertWorkspaceAbstains(t *testing.T, service *Service, operationID string, anchor WorkspaceAnchor) {
	t.Helper()
	prepared, err := service.PrepareContext(context.Background(), PrepareContextRequest{
		OperationID: operationID,
		Workspace:   anchor,
		Task:        "Which continuation marker is current?",
	})
	requireNoError(t, err)
	if prepared.Status != ResolutionNeedsConfirmation || prepared.DeliveryID != "" || prepared.Context != "" {
		t.Fatalf("unknown workspace did not abstain: operation=%s result=%#v", operationID, prepared)
	}
}

func requireWorkspaceFact(t *testing.T, service *Service, operationID string, anchor WorkspaceAnchor, fact string) PrepareContextResponse {
	t.Helper()
	prepared, err := service.PrepareContext(context.Background(), PrepareContextRequest{
		OperationID: operationID,
		Workspace:   anchor,
		Task:        "Which continuation marker is current?",
	})
	requireNoError(t, err)
	if prepared.Status != ResolutionResolved || prepared.DeliveryID == "" {
		t.Fatalf("workspace did not resolve: operation=%s result=%#v", operationID, prepared)
	}
	requireContains(t, prepared.Context, fact)
	return prepared
}

func removeGitWorktree(t *testing.T, primaryRoot, worktreeRoot string) {
	t.Helper()
	runWorkspaceGit(t, primaryRoot, "worktree", "remove", "--force", worktreeRoot)
}

func runWorkspaceGit(t *testing.T, cwd string, args ...string) {
	t.Helper()
	commandArgs := append([]string{"-C", cwd}, args...)
	output, err := exec.Command("git", commandArgs...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, output)
	}
}
