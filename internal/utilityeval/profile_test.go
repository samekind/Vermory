package utilityeval

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadW27Profile(t *testing.T) {
	profile, err := LoadProfile(filepath.Join("..", "..", "runtime", "cases", "W27-real-utility-comparison", "case.json"))
	if err != nil {
		t.Fatal(err)
	}
	if profile.CallsPerCompleteLane != 24 || len(profile.ProviderLanes) != 2 {
		t.Fatalf("unexpected W27 profile: %#v", profile)
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
	bundle := ContextBundle{Version: "1", ProfileID: "W27-real-utility-comparison", Inputs: []CaseInput{input}}
	if err := bundle.Validate(); err == nil {
		t.Fatal("expected changed context evidence to be rejected")
	}
}
