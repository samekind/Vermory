package app

import (
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"vermory/internal/reality"
	"vermory/internal/utilityeval"
)

func TestValidateUtilityWritebackInputsRequiresCompleteFrozenMatrix(t *testing.T) {
	profile, bundle, report := utilityWritebackFixture(t)
	writes, err := validateUtilityWritebackInputs(profile, bundle, report)
	if err != nil {
		t.Fatal(err)
	}
	if len(writes) != len(profile.RealityCaseIDs) {
		t.Fatalf("write count=%d, want %d", len(writes), len(profile.RealityCaseIDs))
	}

	report.Results = report.Results[:len(report.Results)-1]
	if _, err := validateUtilityWritebackInputs(profile, bundle, report); err == nil {
		t.Fatal("expected incomplete report matrix to be rejected")
	}
}

func TestValidateUtilityWritebackInputsRejectsChangedOutput(t *testing.T) {
	profile, bundle, report := utilityWritebackFixture(t)
	native := firstNativeUtilityResult(t, report)
	parsed, err := url.Parse(native.OutputURI)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(parsed.Path, []byte("changed output"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := validateUtilityWritebackInputs(profile, bundle, report); err == nil {
		t.Fatal("expected changed native output to be rejected")
	}
}

func TestValidateUtilityWritebackInputsRejectsFailedNativeResult(t *testing.T) {
	profile, bundle, report := utilityWritebackFixture(t)
	for index := range report.Results {
		if report.Results[index].Condition == utilityeval.ConditionVermoryNative {
			report.Results[index].Score.Success = false
			break
		}
	}
	if _, err := validateUtilityWritebackInputs(profile, bundle, report); err == nil {
		t.Fatal("expected unsuccessful native result to be rejected")
	}
}

func utilityWritebackFixture(t *testing.T) (utilityeval.Profile, utilityeval.ContextBundle, utilityeval.Report) {
	t.Helper()
	profile, err := utilityeval.LoadProfile(filepath.Join("..", "..", "runtime", "cases", "W27-real-utility-comparison", "case.json"))
	if err != nil {
		t.Fatal(err)
	}
	inputs := make([]utilityeval.CaseInput, 0, len(profile.RealityCaseIDs))
	results := make([]utilityeval.CallResult, 0, profile.CallsPerCompleteLane)
	for _, caseID := range profile.RealityCaseIDs {
		input := utilityeval.CaseInput{
			ID:             caseID,
			Task:           "test task",
			Checks:         reality.DownstreamTask{DeterministicChecks: scoringAliasChecks(profile.ScoringAliases[caseID])},
			ScoringAliases: profile.ScoringAliases[caseID],
			Context:        make(map[utilityeval.ConditionID]utilityeval.ContextEvidence, len(utilityeval.FrozenConditions)),
		}
		for _, condition := range utilityeval.FrozenConditions {
			body := "context " + string(condition)
			if condition == utilityeval.ConditionNoContext {
				body = ""
			}
			evidence := utilityeval.NewContextEvidence(body, string(condition))
			if condition == utilityeval.ConditionVermoryNative {
				evidence.DeliveryID = "11111111-1111-1111-1111-111111111111"
			}
			input.Context[condition] = evidence
			result := utilityeval.CallResult{
				CaseID:       caseID,
				Condition:    condition,
				ProviderName: "siliconflow",
				Model:        "deepseek-ai/DeepSeek-V4-Flash",
				Status:       "completed",
				Context:      evidence,
			}
			if condition == utilityeval.ConditionVermoryNative {
				output := []byte("valid native output for " + caseID)
				outputPath := filepath.Join(t.TempDir(), "output.md")
				if err := os.WriteFile(outputPath, output, 0o600); err != nil {
					t.Fatal(err)
				}
				result.Score.Success = true
				result.OutputURI = (&url.URL{Scheme: "file", Path: outputPath}).String()
				result.OutputSHA256 = sha256Bytes(output)
			}
			results = append(results, result)
		}
		if err := input.Validate(); err != nil {
			t.Fatal(err)
		}
		inputs = append(inputs, input)
	}
	lane := profile.ProviderLanes[0]
	return profile, utilityeval.ContextBundle{
			Version:       "2",
			ProfileID:     profile.ID,
			ProfileSHA256: profile.SHA256,
			ScorerVersion: profile.ScorerVersion,
			Inputs:        inputs,
		}, utilityeval.Report{
			RunID:           "utility-writeback-test",
			ProfileID:       profile.ID,
			ProfileSHA256:   profile.SHA256,
			ProviderName:    lane.Provider,
			ProviderMode:    "real",
			Model:           lane.Model,
			Scorer:          profile.ScorerVersion,
			Workers:         profile.Workers,
			DisableThinking: lane.DisableThinking,
			Temperature:     lane.Temperature,
			Results:         results,
		}
}

func scoringAliasChecks(aliases map[string][]string) []string {
	checks := make([]string, 0, len(aliases))
	for canonical := range aliases {
		checks = append(checks, "contains:"+canonical)
	}
	return checks
}

func firstNativeUtilityResult(t *testing.T, report utilityeval.Report) utilityeval.CallResult {
	t.Helper()
	for _, result := range report.Results {
		if result.Condition == utilityeval.ConditionVermoryNative {
			return result
		}
	}
	t.Fatal("no native result")
	return utilityeval.CallResult{}
}
