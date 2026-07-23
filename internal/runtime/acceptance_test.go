package runtime

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

type workspaceSliceCase struct {
	ID              string `json:"id"`
	EvidenceLevel   string `json:"evidence_level"`
	Fixture         string `json:"fixture"`
	CurrentFact     string `json:"current_fact"`
	StaleFact       string `json:"stale_fact"`
	DistractorFact  string `json:"distractor_fact"`
	CorrectionFact  string `json:"correction_fact"`
	DeletionProbe   string `json:"deletion_probe"`
	ParaphraseProbe string `json:"paraphrase_probe"`
}

func TestWorkspaceSliceAcceptance(t *testing.T) {
	caseDir := filepath.Join("..", "..", "runtime", "cases", "W02-codex-workspace-slice")
	fixture := loadWorkspaceSliceCase(t, filepath.Join(caseDir, "case.json"))
	if fixture.ID != "W02-codex-workspace-slice" || fixture.EvidenceLevel != "public" {
		t.Fatalf("unexpected fixture identity: %#v", fixture)
	}

	store := openTestStore(t)
	seed, err := os.ReadFile(filepath.Join(caseDir, fixture.Fixture))
	requireNoError(t, err)
	_, err = store.pool.Exec(context.Background(), string(seed))
	requireNoError(t, err)

	ctx := context.Background()
	service := NewService(store, "local")
	web := mustResolveWorkspace(t, store, "/fixtures/web-checkout")
	ops := mustResolveWorkspace(t, store, "/fixtures/ops-console")
	requireNoError(t, store.RebuildProjection(ctx, "local", web))
	requireNoError(t, store.RebuildProjection(ctx, "local", ops))

	first, err := service.PrepareContext(ctx, PrepareContextRequest{
		OperationID: "w02-prepare-current",
		Workspace:   WorkspaceAnchor{RepoRoot: "/fixtures/web-checkout"},
		Task:        "Continue the staged checkout release.",
	})
	requireNoError(t, err)
	requireContains(t, first.Context, fixture.CurrentFact)
	requireNotContains(t, first.Context, fixture.StaleFact)
	requireNotContains(t, first.Context, fixture.DistractorFact)

	var currentMemoryID string
	err = store.pool.QueryRow(ctx, `
SELECT id::text FROM governed_memories
WHERE tenant_id = 'local' AND continuity_id = $1::uuid AND content = $2`, web, fixture.CurrentFact).Scan(&currentMemoryID)
	requireNoError(t, err)
	correction, err := service.CommitObservation(ctx, CommitObservationRequest{
		OperationID:        "w02-source-correction",
		DeliveryID:         first.DeliveryID,
		Kind:               ObservationKindUserCorrection,
		Content:            fixture.CorrectionFact,
		SourceRef:          "fixture:W02:trusted-correction",
		SupersedesMemoryID: currentMemoryID,
	})
	requireNoError(t, err)
	if correction.MemoryStatus != "active" {
		t.Fatalf("trusted correction is not active: %#v", correction)
	}

	second, err := service.PrepareContext(ctx, PrepareContextRequest{
		OperationID: "w02-prepare-corrected",
		Workspace:   WorkspaceAnchor{RepoRoot: "/fixtures/web-checkout"},
		Task:        "Which checkout flag should the release use now?",
	})
	requireNoError(t, err)
	requireContains(t, second.Context, fixture.CorrectionFact)
	requireNotContains(t, second.Context, fixture.CurrentFact)

	forgotten, err := service.CommitObservation(ctx, CommitObservationRequest{
		OperationID:    "w02-forget-correction",
		DeliveryID:     second.DeliveryID,
		Kind:           ObservationKindForgetRequest,
		Content:        "Forget the current checkout flag.",
		TargetMemoryID: correction.MemoryID,
	})
	requireNoError(t, err)
	if forgotten.MemoryStatus != "deleted" {
		t.Fatalf("deletion did not complete: %#v", forgotten)
	}
	requireNoError(t, store.RebuildProjection(ctx, "local", web))
	for _, probe := range []string{fixture.DeletionProbe, fixture.ParaphraseProbe} {
		if matches := mustSearch(t, store, web, probe); len(matches) != 0 {
			t.Fatalf("deleted fact returned for probe %q: %#v", probe, matches)
		}
	}
}

func loadWorkspaceSliceCase(t *testing.T, path string) workspaceSliceCase {
	t.Helper()
	data, err := os.ReadFile(path)
	requireNoError(t, err)
	var fixture workspaceSliceCase
	requireNoError(t, json.Unmarshal(data, &fixture))
	return fixture
}

func mustResolveWorkspace(t *testing.T, store *Store, repoRoot string) string {
	t.Helper()
	resolution, err := store.ResolveWorkspace(context.Background(), "local", WorkspaceAnchor{RepoRoot: repoRoot})
	requireNoError(t, err)
	if resolution.Status != ResolutionResolved {
		t.Fatalf("expected resolved workspace %q, got %#v", repoRoot, resolution)
	}
	return resolution.ContinuityID
}
