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
		contextBody := "alpha Chinese local-scope 82 percent 中文"
		checks := []string{"contains:alpha", "not_contains:forbidden"}
		for canonical := range profile.ScoringAliases[caseID] {
			checks = append(checks, "contains:"+canonical)
			contextBody += " " + canonical
		}
		for _, condition := range utilityeval.FrozenConditions {
			contexts[condition] = contextBody
		}
		input := utilityeval.CaseInput{
			ID:             caseID,
			Task:           "return alpha",
			Checks:         reality.DownstreamTask{DeterministicChecks: checks},
			ScoringAliases: profile.ScoringAliases[caseID],
			Context:        make(map[utilityeval.ConditionID]utilityeval.ContextEvidence, len(contexts)),
		}
		for condition, body := range contexts {
			input.Context[condition] = utilityeval.NewContextEvidence(body, string(condition))
		}
		inputs = append(inputs, input)
	}
	bundlePath := filepath.Join(t.TempDir(), "bundle.json")
	if err := utilityeval.WriteContextBundle(bundlePath, utilityeval.ContextBundle{Version: "2", ProfileID: profile.ID, ProfileSHA256: profile.SHA256, ScorerVersion: profile.ScorerVersion, Inputs: inputs}); err != nil {
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

func TestValidateBundleScoringAliasesRejectsProfileDrift(t *testing.T) {
	inputs := []utilityeval.CaseInput{{
		ID:             "case-1",
		ScoringAliases: map[string][]string{"Chinese": {"中文"}},
	}}
	expected := map[string]map[string][]string{
		"case-1": {"Chinese": {"汉语"}},
	}
	if err := validateBundleScoringAliases(expected, inputs); err == nil {
		t.Fatal("expected changed scoring aliases to be rejected")
	}
}

func TestUtilityProviderLaneRequiresDeclaredRealLane(t *testing.T) {
	profile, err := utilityeval.LoadProfile(filepath.Join("..", "..", "runtime", "cases", "W27-real-utility-comparison", "case.json"))
	if err != nil {
		t.Fatal(err)
	}
	lane, err := utilityProviderLane(profile, "siliconflow", "deepseek-ai/DeepSeek-V4-Flash")
	if err != nil || !lane.DisableThinking || lane.Temperature != 0 {
		t.Fatalf("expected declared deterministic non-thinking lane: lane=%#v err=%v", lane, err)
	}
	if _, err := utilityProviderLane(profile, "siliconflow", "undeclared-model"); err == nil {
		t.Fatal("expected undeclared real lane to be rejected")
	}
}
