package retrievalablation

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"vermory/internal/runtime"
)

func TestWriteProfileComparisonCreatesDeterministicArtifactsAndReplays(t *testing.T) {
	report := profileComparisonTestFixture()
	paths, replayed, err := WriteProfileComparison(t.TempDir(), report)
	if err != nil {
		t.Fatal(err)
	}
	if replayed {
		t.Fatal("first profile comparison write was marked replayed")
	}
	payload, err := os.ReadFile(paths.JSON)
	if err != nil {
		t.Fatal(err)
	}
	var decoded ProfileComparison
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.RequestFingerprint != report.RequestFingerprint || !decoded.HardGates.Pass {
		t.Fatalf("decoded profile comparison mismatch: %#v", decoded)
	}
	markdown, err := os.ReadFile(paths.Markdown)
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		"# Retrieval Profile Migration Comparison",
		runtime.ProductionRetrievalProfileID,
		runtime.MigrationRetrievalProfileID,
		"Decision: `keep_candidate`",
		"not a model ranking",
	} {
		if !strings.Contains(string(markdown), required) {
			t.Fatalf("profile comparison markdown missing %q:\n%s", required, markdown)
		}
	}

	changed := report
	changed.Profiles = append([]ProfileComparisonReport(nil), report.Profiles...)
	changed.Profiles[0].Queries = nil
	changed.Profiles[1].Queries = nil
	replayedPaths, replayed, err := WriteProfileComparison(filepath.Dir(paths.JSON), changed)
	if err != nil {
		t.Fatal(err)
	}
	if !replayed || replayedPaths != paths {
		t.Fatalf("profile comparison replay mismatch: paths=%#v replayed=%t", replayedPaths, replayed)
	}
}

func TestWriteProfileComparisonRejectsConflictingReplay(t *testing.T) {
	dir := t.TempDir()
	report := profileComparisonTestFixture()
	if _, _, err := WriteProfileComparison(dir, report); err != nil {
		t.Fatal(err)
	}
	conflict := report
	conflict.CorpusSHA256 = strings.Repeat("b", 64)
	conflict.RequestFingerprint = profileComparisonFingerprint(conflict)
	if _, _, err := WriteProfileComparison(dir, conflict); err == nil || !strings.Contains(err.Error(), "conflicting profile comparison replay") {
		t.Fatalf("conflicting replay error=%v", err)
	}
}

func profileComparisonTestFixture() ProfileComparison {
	policy := DefaultProfilePromotionPolicy()
	incumbent := ProfileComparisonReport{
		ProfileID: runtime.ProductionRetrievalProfileID, Model: "BAAI/bge-m3", Dimensions: 1024,
		LifecycleStatus: "active", EmbeddingRequestCount: 10, ProjectionBuildDuration: time.Second,
		ProjectionVectorCount: 8, Metrics: Aggregate{QueryCount: 2, HitAt1: 1, RecallAtK: 1, MRR: 1, NDCGAtK: 1, SearchP95: 150 * time.Millisecond},
		HardGates: HardGateReport{Pass: true}, ProjectionRebuildEquivalent: true,
	}
	candidate := ProfileComparisonReport{
		ProfileID: runtime.MigrationRetrievalProfileID, Model: "BAAI/bge-large-zh-v1.5", Dimensions: 1024,
		LifecycleStatus: "candidate", EmbeddingRequestCount: 10, ProjectionBuildDuration: time.Second,
		ProjectionVectorCount: 8, Metrics: Aggregate{QueryCount: 2, HitAt1: 1, RecallAtK: 1, MRR: 1, NDCGAtK: 1, SearchP95: 155 * time.Millisecond},
		HardGates: HardGateReport{Pass: true}, ProjectionRebuildEquivalent: true,
	}
	report := ProfileComparison{
		RunID: "profile-report", CorpusSHA256: strings.Repeat("a", 64),
		ImplementationRevision: "0123456789abcdef", SchemaVersion: 15,
		AuthorityFingerprint: strings.Repeat("c", 64), StartedAt: time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC),
		Duration: time.Second, Profiles: []ProfileComparisonReport{incumbent, candidate}, Policy: policy,
		Decision:  EvaluateProfilePromotion(policy, incumbent, candidate),
		HardGates: ProfileComparisonHardGates{Pass: true}, QualificationStatus: "measured",
		NonClaims: []string{"not a model ranking"},
	}
	report.RequestFingerprint = profileComparisonFingerprint(report)
	return report
}
