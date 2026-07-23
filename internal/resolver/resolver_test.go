package resolver

import (
	"testing"

	"vermory/internal/domain"
)

func TestResolveWorkspacePrefersExplicitBinding(t *testing.T) {
	result := ResolveWorkspace(WorkspaceInput{
		CWD:                "/tmp/random-copy",
		ExplicitBindingID:  "workspace:contextmesh",
		CandidateRepoRoot:  "/repo/contextmesh",
		KnownWorkspaceName: "ContextMesh",
		Candidates: []WorkspaceCandidate{
			{ID: "workspace:contextmesh", Path: "/repo/contextmesh"},
		},
	})

	if result.Status != ResolutionResolved {
		t.Fatalf("expected resolved status, got %q", result.Status)
	}
	if result.Line != domain.ContinuityLineWorkspace {
		t.Fatalf("expected workspace line, got %q", result.Line)
	}
	if result.SpaceID != "workspace:contextmesh" {
		t.Fatalf("expected explicit binding id, got %q", result.SpaceID)
	}
	if result.AnchorStrength != domain.AnchorStrengthStrong {
		t.Fatalf("expected strong anchor, got %q", result.AnchorStrength)
	}
}

func TestResolveWorkspaceRequiresConfirmationForAmbiguousParent(t *testing.T) {
	result := ResolveWorkspace(WorkspaceInput{
		CWD: "/workspaces",
		Candidates: []WorkspaceCandidate{
			{ID: "workspace:a", Path: "/workspaces/A"},
			{ID: "workspace:b", Path: "/workspaces/B"},
		},
	})

	if result.Status != ResolutionNeedsConfirmation {
		t.Fatalf("expected needs confirmation, got %q", result.Status)
	}
	if len(result.Candidates) != 2 {
		t.Fatalf("expected two candidates, got %d", len(result.Candidates))
	}
}

func TestResolveWorkspaceDoesNotDeriveIdentityFromRepositoryBasename(t *testing.T) {
	result := ResolveWorkspace(WorkspaceInput{CandidateRepoRoot: "/archive/Vermory"})
	if result.Status != ResolutionNeedsConfirmation {
		t.Fatalf("basename-derived workspace identity was accepted: %#v", result)
	}
	if result.SpaceID != "" {
		t.Fatalf("unbound workspace received a guessed space id: %#v", result)
	}
}

func TestResolveConversationUsesThreadButSuggestsLinks(t *testing.T) {
	result := ResolveConversation(ConversationInput{
		ThreadID: "thread-123",
		Channel:  "openclaw",
		SuggestedLinks: []ConversationCandidate{
			{ID: "housing-search", Label: "Seattle housing search", Score: 0.78},
		},
	})

	if result.Status != ResolutionResolved {
		t.Fatalf("expected resolved thread continuity, got %q", result.Status)
	}
	if result.SpaceID != "thread:openclaw:thread-123" {
		t.Fatalf("unexpected space id %q", result.SpaceID)
	}
	if len(result.SuggestedLinks) != 1 {
		t.Fatalf("expected one suggested link, got %d", len(result.SuggestedLinks))
	}
}

func TestResolveGlobalDefaultsRejectsTaskSpecificFacts(t *testing.T) {
	taskSpecific := domain.Claim{
		Type:    domain.ClaimTypeProgress,
		Content: "The checkout task is halfway done.",
	}
	stablePreference := domain.Claim{
		Type:    domain.ClaimTypePreference,
		Content: "Prefer concise Chinese progress reports.",
	}

	if AllowsGlobalDefault(taskSpecific) {
		t.Fatalf("expected task-specific progress to be rejected from global defaults")
	}
	if !AllowsGlobalDefault(stablePreference) {
		t.Fatalf("expected stable preference to be allowed as global default")
	}
}
