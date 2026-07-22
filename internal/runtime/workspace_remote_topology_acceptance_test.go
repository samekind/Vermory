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

type workspaceRemoteTopologyCase struct {
	Version                     string   `json:"version"`
	ID                          string   `json:"id"`
	RealityCaseID               string   `json:"reality_case_id"`
	TenantID                    string   `json:"tenant_id"`
	OtherTenantID               string   `json:"other_tenant_id"`
	FilesystemNamespace         string   `json:"filesystem_namespace"`
	MigratedFilesystemNamespace string   `json:"migrated_filesystem_namespace"`
	CurrentFact                 string   `json:"current_fact"`
	OtherTenantFact             string   `json:"other_tenant_fact"`
	HardGateCount               int      `json:"hard_gate_count"`
	HardGates                   []string `json:"hard_gates"`
}

type workspaceRemoteTopology struct {
	upstreamBare string
	mirrorBare   string
	forkBare     string
	primaryRoot  string
	mirrorRoot   string
	forkRoot     string
	migratedRoot string
}

func TestW33RemoteGitWorkspaceTopologyCaseIsFrozen(t *testing.T) {
	manifest := loadWorkspaceRemoteTopologyCase(t)
	if manifest.Version != "1" || manifest.ID != "W33-remote-git-workspace-topology" ||
		manifest.RealityCaseID != "W05-trusted-workspace-attachment" {
		t.Fatalf("unexpected W33 identity: %#v", manifest)
	}
	if manifest.TenantID == "" || manifest.OtherTenantID == "" || manifest.TenantID == manifest.OtherTenantID {
		t.Fatalf("W33 tenant isolation contract is invalid: %#v", manifest)
	}
	if manifest.FilesystemNamespace == "" || manifest.MigratedFilesystemNamespace == "" ||
		manifest.FilesystemNamespace == manifest.MigratedFilesystemNamespace {
		t.Fatalf("W33 filesystem namespace contract is invalid: %#v", manifest)
	}
	if manifest.HardGateCount != 24 || len(manifest.HardGates) != manifest.HardGateCount {
		t.Fatalf("W33 hard-gate count drifted: count=%d gates=%d", manifest.HardGateCount, len(manifest.HardGates))
	}
	seen := make(map[string]struct{}, len(manifest.HardGates))
	for _, gate := range manifest.HardGates {
		if strings.TrimSpace(gate) == "" {
			t.Fatal("W33 hard gate cannot be empty")
		}
		if _, exists := seen[gate]; exists {
			t.Fatalf("W33 hard gate is duplicated: %q", gate)
		}
		seen[gate] = struct{}{}
	}
}

func TestW33RemoteGitWorkspaceTopologyAcceptance(t *testing.T) {
	manifest := loadWorkspaceRemoteTopologyCase(t)
	topology := createWorkspaceRemoteTopology(t)
	ctx := context.Background()

	if gitOutput(t, topology.mirrorBare, "rev-parse", "--is-bare-repository") != "true" {
		t.Fatal("mirror fixture is not a real bare repository")
	}
	if _, err := resolver.ProbeGitWorkspace(ctx, topology.mirrorBare, manifest.FilesystemNamespace); err == nil {
		t.Fatal("bare mirror was accepted as a workspace attachment")
	}

	primaryBefore := probeWorkspace(t, topology.primaryRoot, manifest.FilesystemNamespace)
	mirror := probeWorkspace(t, topology.mirrorRoot, manifest.FilesystemNamespace)
	fork := probeWorkspace(t, topology.forkRoot, manifest.FilesystemNamespace)
	migrated := probeWorkspace(t, topology.migratedRoot, manifest.MigratedFilesystemNamespace)
	wantHead := gitOutput(t, topology.primaryRoot, "rev-parse", "HEAD")
	for label, root := range map[string]string{
		"mirror":   topology.mirrorRoot,
		"fork":     topology.forkRoot,
		"migrated": topology.migratedRoot,
	} {
		if got := gitOutput(t, root, "rev-parse", "HEAD"); got != wantHead {
			t.Fatalf("%s fixture does not share the primary commit graph: got=%s want=%s", label, got, wantHead)
		}
	}
	for label, attachment := range map[string]resolver.WorkspaceAttachment{
		"mirror": mirror, "fork": fork, "migrated": migrated,
	} {
		if attachment.RepoRoot == primaryBefore.RepoRoot || attachment.Fingerprint == primaryBefore.Fingerprint ||
			attachment.GitCommonFingerprint == primaryBefore.GitCommonFingerprint {
			t.Fatalf("%s topology collapsed into the primary attachment: primary=%#v candidate=%#v", label, primaryBefore, attachment)
		}
	}

	runWorkspaceGit(t, topology.primaryRoot, "remote", "add", "mirror", topology.mirrorBare)
	runWorkspaceGit(t, topology.primaryRoot, "remote", "add", "fork", topology.forkBare)
	runWorkspaceGit(t, topology.primaryRoot, "remote", "set-url", "origin", topology.forkBare)
	primaryAfter := probeWorkspace(t, topology.primaryRoot, manifest.FilesystemNamespace)
	if primaryAfter != primaryBefore {
		t.Fatalf("remote configuration changed trusted attachment identity: before=%#v after=%#v", primaryBefore, primaryAfter)
	}

	store := openTestStore(t)
	primaryAnchor := WorkspaceAnchor{
		RepoRoot: primaryBefore.RepoRoot, FilesystemNamespace: manifest.FilesystemNamespace,
	}
	governance := NewGovernanceServiceWithNamespace(store, manifest.TenantID, manifest.FilesystemNamespace)
	primaryResolution, err := governance.ConfirmWorkspaceAnchor(ctx, primaryAnchor)
	requireNoError(t, err)
	seed, err := governance.AddSource(ctx, primaryBefore.RepoRoot, GovernanceWriteRequest{
		OperationID: "w33-seed-primary",
		MemoryKey:   "continuation_marker",
		Content:     manifest.CurrentFact,
		SourceRef:   "fixture:W05:remote-git-topology",
	})
	requireNoError(t, err)
	if seed.Memory.Status != "active" {
		t.Fatalf("W33 current fact is not active: %#v", seed)
	}

	service := NewService(store, manifest.TenantID)
	mirrorAnchor := WorkspaceAnchor{RepoRoot: mirror.RepoRoot, FilesystemNamespace: manifest.FilesystemNamespace}
	forkAnchor := WorkspaceAnchor{RepoRoot: fork.RepoRoot, FilesystemNamespace: manifest.FilesystemNamespace}
	migratedAnchor := WorkspaceAnchor{
		RepoRoot: migrated.RepoRoot, FilesystemNamespace: manifest.MigratedFilesystemNamespace,
	}
	requireWorkspaceFact(t, service, "w33-primary-after-remote-change", primaryAnchor, manifest.CurrentFact)
	assertWorkspaceAbstains(t, service, "w33-mirror-before-adopt", mirrorAnchor)
	assertWorkspaceAbstains(t, service, "w33-fork-before-adopt", forkAnchor)
	assertWorkspaceAbstains(t, service, "w33-migrated-before-rebind", migratedAnchor)

	otherGovernance := NewGovernanceServiceWithNamespace(store, manifest.OtherTenantID, manifest.MigratedFilesystemNamespace)
	_, err = otherGovernance.ConfirmWorkspaceAnchor(ctx, migratedAnchor)
	requireNoError(t, err)
	_, err = otherGovernance.AddSource(ctx, migrated.RepoRoot, GovernanceWriteRequest{
		OperationID: "w33-seed-other-tenant",
		MemoryKey:   "continuation_marker",
		Content:     manifest.OtherTenantFact,
		SourceRef:   "fixture:W05:remote-git-topology:other-tenant",
	})
	requireNoError(t, err)
	otherService := NewService(store, manifest.OtherTenantID)
	otherPrepared := requireWorkspaceFact(t, otherService, "w33-other-tenant-before-rebind", migratedAnchor, manifest.OtherTenantFact)
	requireNotContains(t, otherPrepared.Context, manifest.CurrentFact)

	bridges := NewBridgeService(store, manifest.TenantID)
	mirrorAdopt, err := bridges.AdoptWorkspaceAnchor(ctx, AdoptWorkspaceAnchorRequest{
		OperationID:         "w33-adopt-mirror-checkout",
		ExistingRepoRoot:    primaryBefore.RepoRoot,
		NewRepoRoot:         mirror.RepoRoot,
		ExistingNamespaceID: manifest.FilesystemNamespace,
		NewNamespaceID:      manifest.FilesystemNamespace,
	})
	requireNoError(t, err)
	if mirrorAdopt.Action != BridgeActionAdopt || mirrorAdopt.SourceContinuityID != primaryResolution.ContinuityID ||
		mirrorAdopt.TargetContinuityID != primaryResolution.ContinuityID {
		t.Fatalf("mirror adopt changed continuity: %#v", mirrorAdopt)
	}
	mirrorPrepared := requireWorkspaceFact(t, service, "w33-mirror-after-adopt", mirrorAnchor, manifest.CurrentFact)
	requireNotContains(t, mirrorPrepared.Context, manifest.OtherTenantFact)
	mirrorReplay, err := bridges.AdoptWorkspaceAnchor(ctx, AdoptWorkspaceAnchorRequest{
		OperationID:         "w33-adopt-mirror-checkout",
		ExistingRepoRoot:    primaryBefore.RepoRoot,
		NewRepoRoot:         mirror.RepoRoot,
		ExistingNamespaceID: manifest.FilesystemNamespace,
		NewNamespaceID:      manifest.FilesystemNamespace,
	})
	requireNoError(t, err)
	if !mirrorReplay.Replayed || mirrorReplay.ID != mirrorAdopt.ID {
		t.Fatalf("mirror adopt replay changed operation: first=%#v replay=%#v", mirrorAdopt, mirrorReplay)
	}
	reversedMirror, err := bridges.Reverse(ctx, ReverseBridgeRequest{
		OperationID: "w33-reverse-mirror-adopt", BridgeID: mirrorAdopt.ID,
	})
	requireNoError(t, err)
	if reversedMirror.Status != BridgeStatusReversed {
		t.Fatalf("mirror adopt did not reverse: %#v", reversedMirror)
	}
	assertWorkspaceAbstains(t, service, "w33-mirror-after-reverse", mirrorAnchor)
	requireWorkspaceFact(t, service, "w33-primary-after-mirror-reverse", primaryAnchor, manifest.CurrentFact)

	forkAdopt, err := bridges.AdoptWorkspaceAnchor(ctx, AdoptWorkspaceAnchorRequest{
		OperationID:         "w33-adopt-fork-checkout",
		ExistingRepoRoot:    primaryBefore.RepoRoot,
		NewRepoRoot:         fork.RepoRoot,
		ExistingNamespaceID: manifest.FilesystemNamespace,
		NewNamespaceID:      manifest.FilesystemNamespace,
	})
	requireNoError(t, err)
	if forkAdopt.SourceContinuityID != primaryResolution.ContinuityID || forkAdopt.TargetContinuityID != primaryResolution.ContinuityID {
		t.Fatalf("fork adopt changed continuity: %#v", forkAdopt)
	}
	forkPrepared := requireWorkspaceFact(t, service, "w33-fork-after-adopt", forkAnchor, manifest.CurrentFact)
	requireNotContains(t, forkPrepared.Context, manifest.OtherTenantFact)
	reversedFork, err := bridges.Reverse(ctx, ReverseBridgeRequest{
		OperationID: "w33-reverse-fork-adopt", BridgeID: forkAdopt.ID,
	})
	requireNoError(t, err)
	if reversedFork.Status != BridgeStatusReversed {
		t.Fatalf("fork adopt did not reverse: %#v", reversedFork)
	}
	assertWorkspaceAbstains(t, service, "w33-fork-after-reverse", forkAnchor)

	rebound, err := bridges.RebindWorkspace(ctx, RebindWorkspaceRequest{
		OperationID:    "w33-rebind-device-migration",
		OldRepoRoot:    primaryBefore.RepoRoot,
		NewRepoRoot:    migrated.RepoRoot,
		OldNamespaceID: manifest.FilesystemNamespace,
		NewNamespaceID: manifest.MigratedFilesystemNamespace,
	})
	requireNoError(t, err)
	if rebound.Action != BridgeActionRebind || rebound.SourceContinuityID != primaryResolution.ContinuityID ||
		rebound.TargetContinuityID != primaryResolution.ContinuityID {
		t.Fatalf("cross-namespace rebind changed continuity: %#v", rebound)
	}
	migratedPrepared := requireWorkspaceFact(t, service, "w33-migrated-after-rebind", migratedAnchor, manifest.CurrentFact)
	requireNotContains(t, migratedPrepared.Context, manifest.OtherTenantFact)
	assertWorkspaceAbstains(t, service, "w33-primary-after-rebind", primaryAnchor)
	otherAfterRebind := requireWorkspaceFact(t, otherService, "w33-other-tenant-after-rebind", migratedAnchor, manifest.OtherTenantFact)
	requireNotContains(t, otherAfterRebind.Context, manifest.CurrentFact)

	rebindReplay, err := bridges.RebindWorkspace(ctx, RebindWorkspaceRequest{
		OperationID:    "w33-rebind-device-migration",
		OldRepoRoot:    primaryBefore.RepoRoot,
		NewRepoRoot:    migrated.RepoRoot,
		OldNamespaceID: manifest.FilesystemNamespace,
		NewNamespaceID: manifest.MigratedFilesystemNamespace,
	})
	requireNoError(t, err)
	if !rebindReplay.Replayed || rebindReplay.ID != rebound.ID {
		t.Fatalf("device rebind replay changed operation: first=%#v replay=%#v", rebound, rebindReplay)
	}
	reversedRebind, err := bridges.Reverse(ctx, ReverseBridgeRequest{
		OperationID: "w33-reverse-device-rebind", BridgeID: rebound.ID,
	})
	requireNoError(t, err)
	if reversedRebind.Status != BridgeStatusReversed {
		t.Fatalf("device rebind did not reverse: %#v", reversedRebind)
	}
	requireWorkspaceFact(t, service, "w33-primary-after-rebind-reverse", primaryAnchor, manifest.CurrentFact)
	assertWorkspaceAbstains(t, service, "w33-migrated-after-rebind-reverse", migratedAnchor)

	finalRebind, err := bridges.RebindWorkspace(ctx, RebindWorkspaceRequest{
		OperationID:    "w33-final-device-rebind",
		OldRepoRoot:    primaryBefore.RepoRoot,
		NewRepoRoot:    migrated.RepoRoot,
		OldNamespaceID: manifest.FilesystemNamespace,
		NewNamespaceID: manifest.MigratedFilesystemNamespace,
	})
	requireNoError(t, err)
	if finalRebind.ID == rebound.ID || finalRebind.Replayed {
		t.Fatalf("final device rebind reused the reversed operation: first=%#v final=%#v", rebound, finalRebind)
	}
	finalPrepared := requireWorkspaceFact(t, service, "w33-final-migrated-prepare", migratedAnchor, manifest.CurrentFact)
	requireNotContains(t, finalPrepared.Context, manifest.OtherTenantFact)
	assertWorkspaceAbstains(t, service, "w33-final-primary-prepare", primaryAnchor)
}

func loadWorkspaceRemoteTopologyCase(t *testing.T) workspaceRemoteTopologyCase {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	payload, err := os.ReadFile(filepath.Join(root, "runtime", "cases", "W33-remote-git-workspace-topology", "case.json"))
	if err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	var manifest workspaceRemoteTopologyCase
	if err := decoder.Decode(&manifest); err != nil {
		t.Fatal(err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		t.Fatalf("W33 case contains trailing JSON data: %v", err)
	}
	return manifest
}

func createWorkspaceRemoteTopology(t *testing.T) workspaceRemoteTopology {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Fatalf("W33 requires the real git executable: %v", err)
	}
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	remoteRoot := filepath.Join(root, "remotes")
	if err := os.MkdirAll(remoteRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	seedRoot := filepath.Join(root, "seed", "Vermory")
	initWorkspaceGitRepository(t, seedRoot, "remote topology seed\n")
	runWorkspaceGit(t, seedRoot, "branch", "-M", "main")

	upstreamBare := filepath.Join(remoteRoot, "upstream.git")
	runWorkspaceGit(t, root, "init", "--bare", upstreamBare)
	runWorkspaceGit(t, seedRoot, "remote", "add", "origin", upstreamBare)
	runWorkspaceGit(t, seedRoot, "push", "-u", "origin", "main")
	runWorkspaceGit(t, upstreamBare, "symbolic-ref", "HEAD", "refs/heads/main")

	primaryRoot := filepath.Join(root, "device-alpha", "Vermory")
	runWorkspaceGit(t, root, "clone", upstreamBare, primaryRoot)
	mirrorBare := filepath.Join(remoteRoot, "mirror.git")
	runWorkspaceGit(t, root, "clone", "--mirror", upstreamBare, mirrorBare)
	mirrorRoot := filepath.Join(root, "mirror-checkout", "Vermory")
	runWorkspaceGit(t, root, "clone", mirrorBare, mirrorRoot)
	forkBare := filepath.Join(remoteRoot, "fork.git")
	runWorkspaceGit(t, root, "clone", "--bare", upstreamBare, forkBare)
	forkRoot := filepath.Join(root, "fork-checkout", "Vermory")
	runWorkspaceGit(t, root, "clone", forkBare, forkRoot)
	migratedRoot := filepath.Join(root, "device-beta", "Vermory")
	runWorkspaceGit(t, root, "clone", upstreamBare, migratedRoot)

	return workspaceRemoteTopology{
		upstreamBare: upstreamBare,
		mirrorBare:   mirrorBare,
		forkBare:     forkBare,
		primaryRoot:  primaryRoot,
		mirrorRoot:   mirrorRoot,
		forkRoot:     forkRoot,
		migratedRoot: migratedRoot,
	}
}

func gitOutput(t *testing.T, cwd string, args ...string) string {
	t.Helper()
	commandArgs := append([]string{"-C", cwd}, args...)
	output, err := exec.Command("git", commandArgs...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, output)
	}
	return strings.TrimSpace(string(output))
}
