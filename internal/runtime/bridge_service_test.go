package runtime

import (
	"context"
	"strings"
	"testing"
)

func TestBridgePromoteCopiesOnlySelectedActiveConversationMemoryAndReverses(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	service := NewBridgeService(store, "local")
	sourceAnchor := ConversationAnchor{Channel: "web_chat", ThreadID: "release-planning"}
	sourceResolution, selectedMemoryID := confirmConversationMemoryForBridge(t, store, "local", sourceAnchor, "promote-selected", "Use checkout_eta_v2 for the staged checkout release.")
	_, _ = confirmConversationMemoryForBridge(t, store, "local", sourceAnchor, "promote-noise", "Run the checkout smoke suite before increasing rollout percentage.")
	proposed, err := store.CommitGovernedObservation(ctx, "local", sourceResolution.ContinuityID, CommitObservationRequest{
		OperationID: "promote-proposed",
		Kind:        ObservationKindAgentResult,
		Content:     "Rename the internal mascot before launch.",
	})
	requireNoError(t, err)
	workspaceContinuityID, err := store.ConfirmWorkspaceBinding(ctx, "local", "/fixtures/checkout-workspace")
	requireNoError(t, err)

	promoted, err := service.PromoteConversationToWorkspace(ctx, PromoteConversationToWorkspaceRequest{
		OperationID:    "bridge-promote-release-flag",
		Source:         sourceAnchor,
		TargetRepoRoot: "/fixtures/checkout-workspace",
		MemoryIDs:      []string{selectedMemoryID},
	})
	requireNoError(t, err)
	if promoted.Action != BridgeActionPromote || promoted.Status != BridgeStatusActive || len(promoted.MemoryEffects) != 1 || promoted.MemoryEffects[0].SourceMemoryID != selectedMemoryID || promoted.MemoryEffects[0].TargetMemoryID == "" {
		t.Fatalf("unexpected promote receipt: %#v", promoted)
	}

	workspace := NewService(store, "local")
	prepared, err := workspace.PrepareContext(ctx, PrepareContextRequest{
		OperationID: "bridge-promote-workspace-consume",
		Workspace:   WorkspaceAnchor{RepoRoot: "/fixtures/checkout-workspace"},
		Task:        "Which checkout flag should the staged release use?",
	})
	requireNoError(t, err)
	requireContains(t, prepared.Context, "checkout_eta_v2")
	requireNotContains(t, prepared.Context, "smoke suite")
	requireNotContains(t, prepared.Context, "mascot")
	if deliveryContinuityID, err := store.DeliveryContinuity(ctx, "local", prepared.DeliveryID); err != nil || deliveryContinuityID != workspaceContinuityID {
		t.Fatalf("promoted delivery escaped target workspace: continuity=%s err=%v", deliveryContinuityID, err)
	}

	replay, err := service.PromoteConversationToWorkspace(ctx, PromoteConversationToWorkspaceRequest{
		OperationID:    "bridge-promote-release-flag",
		Source:         sourceAnchor,
		TargetRepoRoot: "/fixtures/checkout-workspace",
		MemoryIDs:      []string{selectedMemoryID},
	})
	requireNoError(t, err)
	if !replay.Replayed || replay.ID != promoted.ID || replay.MemoryEffects[0].TargetMemoryID != promoted.MemoryEffects[0].TargetMemoryID {
		t.Fatalf("promote replay changed effects: first=%#v replay=%#v", promoted, replay)
	}

	_, err = service.PromoteConversationToWorkspace(ctx, PromoteConversationToWorkspaceRequest{
		OperationID:    "bridge-promote-reject-proposed",
		Source:         sourceAnchor,
		TargetRepoRoot: "/fixtures/checkout-workspace",
		MemoryIDs:      []string{proposed.Memory.MemoryID},
	})
	if err == nil || !strings.Contains(err.Error(), "active") {
		t.Fatalf("non-active memory %s was promoted: %v", proposed.Memory.MemoryID, err)
	}

	reversed, err := service.Reverse(ctx, ReverseBridgeRequest{OperationID: "bridge-promote-reverse", BridgeID: promoted.ID})
	requireNoError(t, err)
	if reversed.Status != BridgeStatusReversed {
		t.Fatalf("promotion was not reversed: %#v", reversed)
	}
	requireNoError(t, store.RebuildProjection(ctx, "local", workspaceContinuityID))
	prepared, err = workspace.PrepareContext(ctx, PrepareContextRequest{
		OperationID: "bridge-promote-workspace-after-reverse",
		Workspace:   WorkspaceAnchor{RepoRoot: "/fixtures/checkout-workspace"},
		Task:        "Which checkout flag should the staged release use now?",
	})
	requireNoError(t, err)
	requireNotContains(t, prepared.Context, "checkout_eta_v2")
	if sourceMatches, err := store.SearchActiveMemory(ctx, "local", sourceResolution.ContinuityID, "checkout_eta_v2", 5); err != nil || len(sourceMatches) != 1 || sourceMatches[0].ID != selectedMemoryID {
		t.Fatalf("promotion reversal altered source memory: matches=%#v err=%v", sourceMatches, err)
	}
}

func TestBridgeExportIsBoundedDurableAndRevocable(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	governance := NewGovernanceService(store, "local")
	_, err := governance.ConfirmWorkspace(ctx, "/fixtures/export-workspace")
	requireNoError(t, err)
	flag, err := governance.AddSource(ctx, "/fixtures/export-workspace", GovernanceWriteRequest{
		OperationID: "export-source-flag",
		Content:     "Use checkout_eta_v2 for the staged checkout release.",
		SourceRef:   "fixture:B01:flag",
	})
	requireNoError(t, err)
	smoke, err := governance.AddSource(ctx, "/fixtures/export-workspace", GovernanceWriteRequest{
		OperationID: "export-source-smoke",
		Content:     "Run the checkout smoke suite before increasing rollout percentage.",
		SourceRef:   "fixture:B01:smoke",
	})
	requireNoError(t, err)
	_, err = governance.AddSource(ctx, "/fixtures/export-workspace", GovernanceWriteRequest{
		OperationID: "export-source-noise",
		Content:     "Rename the internal mascot before launch.",
		SourceRef:   "fixture:B01:noise",
	})
	requireNoError(t, err)

	service := NewBridgeService(store, "local")
	exported, err := service.ExportWorkspace(ctx, ExportWorkspaceRequest{
		OperationID:   "bridge-export-release-handoff",
		RepoRoot:      "/fixtures/export-workspace",
		MemoryIDs:     []string{flag.Memory.MemoryID, smoke.Memory.MemoryID},
		Title:         "Release handoff",
		TargetProfile: "team_handoff",
	})
	requireNoError(t, err)
	if exported.Action != BridgeActionExport || exported.TargetProfile != "team_handoff" || len(exported.MemoryEffects) != 2 {
		t.Fatalf("unexpected export receipt: %#v", exported)
	}
	for _, expected := range []string{"Release handoff", "checkout_eta_v2", "smoke suite"} {
		requireContains(t, exported.ExportBody, expected)
	}
	requireNotContains(t, exported.ExportBody, "mascot")
	requireNotContains(t, exported.ExportBody, flag.Memory.MemoryID)

	replay, err := service.ExportWorkspace(ctx, ExportWorkspaceRequest{
		OperationID:   "bridge-export-release-handoff",
		RepoRoot:      "/fixtures/export-workspace",
		MemoryIDs:     []string{flag.Memory.MemoryID, smoke.Memory.MemoryID},
		Title:         "Release handoff",
		TargetProfile: "team_handoff",
	})
	requireNoError(t, err)
	if !replay.Replayed || replay.ID != exported.ID || replay.ExportBody != exported.ExportBody {
		t.Fatalf("export replay changed artifact: first=%#v replay=%#v", exported, replay)
	}

	revoked, err := service.Reverse(ctx, ReverseBridgeRequest{OperationID: "bridge-export-revoke", BridgeID: exported.ID})
	requireNoError(t, err)
	if revoked.Status != BridgeStatusRevoked || revoked.ExportBody != "[revoked]" {
		t.Fatalf("export was not revoked and redacted: %#v", revoked)
	}
}

func TestBridgeAdoptAddsReversibleWorkspaceAlias(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	governance := NewGovernanceService(store, "local")
	original, err := governance.ConfirmWorkspace(ctx, "/fixtures/adopt-original")
	requireNoError(t, err)
	seed, err := governance.AddSource(ctx, "/fixtures/adopt-original", GovernanceWriteRequest{
		OperationID: "adopt-source",
		Content:     "Build the final PDF with make final-pdf.",
		SourceRef:   "fixture:B02:adopt",
	})
	requireNoError(t, err)
	if seed.Memory.Status != "active" {
		t.Fatalf("workspace seed is not active: %#v", seed)
	}

	service := NewBridgeService(store, "local")
	adopted, err := service.AdoptWorkspaceAnchor(ctx, AdoptWorkspaceAnchorRequest{
		OperationID:      "bridge-adopt-worktree",
		ExistingRepoRoot: "/fixtures/adopt-original",
		NewRepoRoot:      "/fixtures/adopt-worktree",
	})
	requireNoError(t, err)
	if adopted.Action != BridgeActionAdopt || adopted.SourceContinuityID != original.ContinuityID || adopted.TargetContinuityID != original.ContinuityID {
		t.Fatalf("unexpected adopt receipt: %#v", adopted)
	}
	for _, root := range []string{"/fixtures/adopt-original", "/fixtures/adopt-worktree"} {
		resolution, err := store.ResolveWorkspace(ctx, "local", WorkspaceAnchor{RepoRoot: root})
		requireNoError(t, err)
		if resolution.Status != ResolutionResolved || resolution.ContinuityID != original.ContinuityID {
			t.Fatalf("adopted root %s did not resolve to original continuity: %#v", root, resolution)
		}
	}
	prepared, err := NewService(store, "local").PrepareContext(ctx, PrepareContextRequest{
		OperationID: "bridge-adopt-consume",
		Workspace:   WorkspaceAnchor{RepoRoot: "/fixtures/adopt-worktree"},
		Task:        "How is the final PDF built?",
	})
	requireNoError(t, err)
	requireContains(t, prepared.Context, "make final-pdf")

	_, err = governance.ConfirmWorkspace(ctx, "/fixtures/adopt-conflict")
	requireNoError(t, err)
	_, err = service.AdoptWorkspaceAnchor(ctx, AdoptWorkspaceAnchorRequest{
		OperationID:      "bridge-adopt-conflict",
		ExistingRepoRoot: "/fixtures/adopt-original",
		NewRepoRoot:      "/fixtures/adopt-conflict",
	})
	if err == nil || !strings.Contains(err.Error(), "already") {
		t.Fatalf("adopt accepted a root owned by another continuity: %v", err)
	}

	reversed, err := service.Reverse(ctx, ReverseBridgeRequest{OperationID: "bridge-adopt-reverse", BridgeID: adopted.ID})
	requireNoError(t, err)
	if reversed.Status != BridgeStatusReversed {
		t.Fatalf("adopt was not reversed: %#v", reversed)
	}
	alias, err := store.ResolveWorkspace(ctx, "local", WorkspaceAnchor{RepoRoot: "/fixtures/adopt-worktree"})
	requireNoError(t, err)
	if alias.Status != ResolutionNeedsConfirmation {
		t.Fatalf("reversed alias still resolves: %#v", alias)
	}
	stillOriginal, err := store.ResolveWorkspace(ctx, "local", WorkspaceAnchor{RepoRoot: "/fixtures/adopt-original"})
	requireNoError(t, err)
	if stillOriginal.Status != ResolutionResolved || stillOriginal.ContinuityID != original.ContinuityID {
		t.Fatalf("adopt reversal altered original binding: %#v", stillOriginal)
	}
}

func TestBridgeRebindMovesWorkspaceContinuityAndReverses(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	governance := NewGovernanceService(store, "local")
	original, err := governance.ConfirmWorkspace(ctx, "/fixtures/thesis-old")
	requireNoError(t, err)
	_, err = governance.AddSource(ctx, "/fixtures/thesis-old", GovernanceWriteRequest{
		OperationID: "rebind-source",
		Content:     "Build the final PDF with make final-pdf.",
		SourceRef:   "fixture:B02:rebind",
	})
	requireNoError(t, err)

	service := NewBridgeService(store, "local")
	rebound, err := service.RebindWorkspace(ctx, RebindWorkspaceRequest{
		OperationID: "bridge-rebind-thesis",
		OldRepoRoot: "/fixtures/thesis-old",
		NewRepoRoot: "/fixtures/thesis-new",
	})
	requireNoError(t, err)
	if rebound.Action != BridgeActionRebind || rebound.SourceContinuityID != original.ContinuityID || rebound.TargetContinuityID != original.ContinuityID {
		t.Fatalf("unexpected rebind receipt: %#v", rebound)
	}
	oldResolution, err := store.ResolveWorkspace(ctx, "local", WorkspaceAnchor{RepoRoot: "/fixtures/thesis-old"})
	requireNoError(t, err)
	newResolution, err := store.ResolveWorkspace(ctx, "local", WorkspaceAnchor{RepoRoot: "/fixtures/thesis-new"})
	requireNoError(t, err)
	if oldResolution.Status != ResolutionNeedsConfirmation || newResolution.Status != ResolutionResolved || newResolution.ContinuityID != original.ContinuityID {
		t.Fatalf("rebind did not move the exact anchor: old=%#v new=%#v", oldResolution, newResolution)
	}
	prepared, err := NewService(store, "local").PrepareContext(ctx, PrepareContextRequest{
		OperationID: "bridge-rebind-consume",
		Workspace:   WorkspaceAnchor{RepoRoot: "/fixtures/thesis-new"},
		Task:        "How is the final PDF built?",
	})
	requireNoError(t, err)
	requireContains(t, prepared.Context, "make final-pdf")

	replay, err := service.RebindWorkspace(ctx, RebindWorkspaceRequest{
		OperationID: "bridge-rebind-thesis",
		OldRepoRoot: "/fixtures/thesis-old",
		NewRepoRoot: "/fixtures/thesis-new",
	})
	requireNoError(t, err)
	if !replay.Replayed || replay.ID != rebound.ID {
		t.Fatalf("rebind replay changed operation: first=%#v replay=%#v", rebound, replay)
	}

	reversed, err := service.Reverse(ctx, ReverseBridgeRequest{OperationID: "bridge-rebind-reverse", BridgeID: rebound.ID})
	requireNoError(t, err)
	if reversed.Status != BridgeStatusReversed {
		t.Fatalf("rebind was not reversed: %#v", reversed)
	}
	oldResolution, err = store.ResolveWorkspace(ctx, "local", WorkspaceAnchor{RepoRoot: "/fixtures/thesis-old"})
	requireNoError(t, err)
	newResolution, err = store.ResolveWorkspace(ctx, "local", WorkspaceAnchor{RepoRoot: "/fixtures/thesis-new"})
	requireNoError(t, err)
	if oldResolution.Status != ResolutionResolved || oldResolution.ContinuityID != original.ContinuityID || newResolution.Status != ResolutionNeedsConfirmation {
		t.Fatalf("rebind reversal did not restore exact state: old=%#v new=%#v", oldResolution, newResolution)
	}
}

func TestNamespacedWorkspaceBridgePreservesIdentityAcrossAdoptAndRebind(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	const tenantID = "w05-namespaced"
	oldRoot := "/work/Vermory"
	worktreeRoot := "/worktrees/Vermory-resolver"
	newRoot := "/srv/Vermory"
	continuityID, err := store.ConfirmWorkspaceAnchorBinding(ctx, tenantID, WorkspaceAnchor{
		RepoRoot: oldRoot, FilesystemNamespace: "workstation-alpha",
	})
	if err != nil {
		t.Fatal(err)
	}
	service := NewBridgeService(store, tenantID)
	adopted, err := service.AdoptWorkspaceAnchor(ctx, AdoptWorkspaceAnchorRequest{
		OperationID:         "w05-adopt-worktree",
		ExistingRepoRoot:    oldRoot,
		NewRepoRoot:         worktreeRoot,
		ExistingNamespaceID: "workstation-alpha",
		NewNamespaceID:      "workstation-alpha",
	})
	if err != nil {
		t.Fatal(err)
	}
	if adopted.TargetNamespaceID != "workstation-alpha" {
		t.Fatalf("adopt receipt lost target namespace: %#v", adopted)
	}
	worktree, err := store.ResolveWorkspace(ctx, tenantID, WorkspaceAnchor{RepoRoot: worktreeRoot, FilesystemNamespace: "workstation-alpha"})
	if err != nil || worktree.Status != ResolutionResolved || worktree.ContinuityID != continuityID {
		t.Fatalf("adopted namespaced worktree did not reconnect: resolution=%#v err=%v", worktree, err)
	}

	rebound, err := service.RebindWorkspace(ctx, RebindWorkspaceRequest{
		OperationID:    "w05-rebind-primary",
		OldRepoRoot:    oldRoot,
		NewRepoRoot:    newRoot,
		OldNamespaceID: "workstation-alpha",
		NewNamespaceID: "workstation-beta",
	})
	if err != nil {
		t.Fatal(err)
	}
	if rebound.SourceNamespaceID != "workstation-alpha" || rebound.TargetNamespaceID != "workstation-beta" {
		t.Fatalf("rebind receipt lost namespace transition: %#v", rebound)
	}
	newResolution, err := store.ResolveWorkspace(ctx, tenantID, WorkspaceAnchor{RepoRoot: newRoot, FilesystemNamespace: "workstation-beta"})
	if err != nil || newResolution.Status != ResolutionResolved || newResolution.ContinuityID != continuityID {
		t.Fatalf("rebound namespaced workspace did not reconnect: resolution=%#v err=%v", newResolution, err)
	}
	oldResolution, err := store.ResolveWorkspace(ctx, tenantID, WorkspaceAnchor{RepoRoot: oldRoot, FilesystemNamespace: "workstation-alpha"})
	if err != nil || oldResolution.Status != ResolutionNeedsConfirmation {
		t.Fatalf("retired namespaced root remained attached: resolution=%#v err=%v", oldResolution, err)
	}

	if _, err := service.Reverse(ctx, ReverseBridgeRequest{OperationID: "w05-reverse-rebind", BridgeID: rebound.ID}); err != nil {
		t.Fatal(err)
	}
	restored, err := store.ResolveWorkspace(ctx, tenantID, WorkspaceAnchor{RepoRoot: oldRoot, FilesystemNamespace: "workstation-alpha"})
	if err != nil || restored.Status != ResolutionResolved || restored.ContinuityID != continuityID {
		t.Fatalf("reversed namespaced rebind did not restore source: resolution=%#v err=%v", restored, err)
	}
	newResolution, err = store.ResolveWorkspace(ctx, tenantID, WorkspaceAnchor{RepoRoot: newRoot, FilesystemNamespace: "workstation-beta"})
	if err != nil || newResolution.Status != ResolutionNeedsConfirmation {
		t.Fatalf("reversed namespaced rebind retained target: resolution=%#v err=%v", newResolution, err)
	}
}

func confirmConversationMemoryForBridge(t *testing.T, store *Store, tenantID string, anchor ConversationAnchor, operationPrefix, content string) (ConversationResolution, string) {
	t.Helper()
	ctx := context.Background()
	resolution, err := store.ResolveOrCreateConversation(ctx, tenantID, anchor)
	requireNoError(t, err)
	observation, err := store.CommitObservation(ctx, tenantID, resolution.ContinuityID, CommitObservationRequest{
		OperationID: operationPrefix + ":message",
		Kind:        ObservationKindUserMessage,
		Content:     content,
		SourceRef:   conversationUserSourceRef,
	})
	requireNoError(t, err)
	memory, err := store.ConfirmConversationObservation(ctx, tenantID, resolution.ContinuityID, observation.ObservationID, operationPrefix+":confirm")
	requireNoError(t, err)
	return resolution, memory.MemoryID
}
