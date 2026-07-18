package app

import (
	"context"
	"path/filepath"
	"testing"

	"vermory/internal/reality"
	"vermory/internal/utilityeval"
)

func TestEvalUtilityComparisonUsesBundleAndRecomputesResults(t *testing.T) {
	profilePath := filepath.Join("..", "..", "runtime", "cases", "W27-real-utility-comparison", "case.json")
	profile, err := utilityeval.LoadProfile(profilePath)
	if err != nil {
		t.Fatal(err)
	}
	inputs := make([]utilityeval.CaseInput, 0, len(profile.RealityCaseIDs))
	for _, caseID := range profile.RealityCaseIDs {
		contexts := make(map[utilityeval.ConditionID]string, len(utilityeval.FrozenConditions))
		for _, condition := range utilityeval.FrozenConditions {
			contexts[condition] = "alpha"
		}
		input := utilityeval.CaseInput{
			ID:   caseID,
			Task: "return alpha",
			Checks: reality.DownstreamTask{DeterministicChecks: []string{
				"contains:alpha",
				"not_contains:forbidden",
			}},
			Context: make(map[utilityeval.ConditionID]utilityeval.ContextEvidence, len(contexts)),
		}
		for condition, body := range contexts {
			input.Context[condition] = utilityeval.NewContextEvidence(body, string(condition))
		}
		inputs = append(inputs, input)
	}
	bundlePath := filepath.Join(t.TempDir(), "bundle.json")
	if err := utilityeval.WriteContextBundle(bundlePath, utilityeval.ContextBundle{Version: "1", ProfileID: profile.ID, Inputs: inputs}); err != nil {
		t.Fatal(err)
	}
	report, err := EvalUtilityComparison(context.Background(), UtilityComparisonOptions{
		ProfilePath:  profilePath,
		BundlePath:   bundlePath,
		ArtifactRoot: t.TempDir(),
		Provider:     "mock",
		RunID:        "utility-app-test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Results) != len(profile.RealityCaseIDs)*len(utilityeval.FrozenConditions) {
		t.Fatalf("unexpected result count: %d", len(report.Results))
	}
	if report.Aggregates[utilityeval.ConditionVermoryNative].Successful != len(profile.RealityCaseIDs) {
		t.Fatalf("native aggregate was not recomputed from calls: %#v", report.Aggregates[utilityeval.ConditionVermoryNative])
	}
}
