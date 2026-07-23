package utilityeval

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"vermory/internal/reality"
)

func TestLoadW27Profile(t *testing.T) {
	profile, err := LoadProfile(filepath.Join("..", "..", "runtime", "cases", "W27-real-utility-comparison", "case.json"))
	if err != nil {
		t.Fatal(err)
	}
	if profile.CallsPerCompleteLane != 24 || len(profile.ProviderLanes) != 2 {
		t.Fatalf("unexpected W27 profile: %#v", profile)
	}
	if profile.Version != "2" || profile.ScorerVersion != ScorerVersion || profile.Workers != 2 || len(profile.ScoringAliases) != 3 || len(profile.SHA256) != 64 {
		t.Fatalf("unexpected W27 scoring contract: %#v", profile)
	}
	if !profile.ProviderLanes[0].DisableThinking || profile.ProviderLanes[1].DisableThinking {
		t.Fatalf("unexpected W27 thinking modes: %#v", profile.ProviderLanes)
	}
}

func TestW27ProfileSemanticPredicatesMatchPositiveRelationsOnly(t *testing.T) {
	profile, err := LoadProfile(filepath.Join("..", "..", "runtime", "cases", "W27-real-utility-comparison", "case.json"))
	if err != nil {
		t.Fatal(err)
	}
	g01 := scoreTask(
		reality.DownstreamTask{DeterministicChecks: []string{"contains:local-scope"}},
		profile.ScoringAliases["G01-language-default-local-override"],
		"英语的临时规定仅作用于任务本身，不会改变全局默认。",
	)
	if !g01.Success {
		t.Fatalf("expected positive G01 relation to pass: %#v", g01)
	}
	s01 := scoreTask(
		reality.DownstreamTask{DeterministicChecks: []string{"contains:rotated after use"}},
		profile.ScoringAliases["S01-deletion-and-source-injection"],
		"After a recovery code is used for authentication, it should be immediately rotated.",
	)
	if !s01.Success {
		t.Fatalf("expected positive S01 relation to pass: %#v", s01)
	}
	negated := scoreTask(
		reality.DownstreamTask{DeterministicChecks: []string{"contains:rotated after use"}},
		profile.ScoringAliases["S01-deletion-and-source-injection"],
		"After a recovery code is used, it should not be rotated.",
	)
	if negated.Success {
		t.Fatalf("negated S01 relation matched: %#v", negated)
	}
}

func TestLoadProfileRejectsUnknownFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "profile.json")
	if err := os.WriteFile(path, []byte(`{"version":"1","unexpected":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadProfile(path); err == nil {
		t.Fatal("expected unknown profile field to be rejected")
	}
}

func TestContextBundleRejectsChangedEvidenceDigest(t *testing.T) {
	input := testInput(t)
	input.Context[ConditionVermoryNative] = ContextEvidence{Body: "changed", Source: "native", SHA256: "wrong", ByteSize: 7}
	bundle := ContextBundle{Version: "2", ProfileID: "W27-real-utility-comparison", ProfileSHA256: strings.Repeat("a", 64), ScorerVersion: ScorerVersion, Inputs: []CaseInput{input}}
	if err := bundle.Validate(); err == nil {
		t.Fatal("expected changed context evidence to be rejected")
	}
}
