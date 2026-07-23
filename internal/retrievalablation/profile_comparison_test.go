package retrievalablation

import (
	"context"
	"strings"
	"testing"
	"time"

	"vermory/internal/runtime"
)

func TestRunProfileComparisonUsesSharedAuthorityAndIndependentProductionProfiles(t *testing.T) {
	store := openRetrievalTestStore(t)
	corpus := seedTestCorpus()
	corpus.Records = append(corpus.Records, Record{
		ID: "distractor", ScopeID: "workspace-a", MemoryKey: "release.region",
		Content: "The release region is ap-southeast-1.", Lifecycle: "active",
		ProvenanceCase: "101-workspace-parallel-repos",
	})

	incumbent := profileTestEmbedder{queryNeedle: "current command", preferredContent: "current locked command"}
	candidate := profileTestEmbedder{queryNeedle: "current command", preferredContent: "current locked command"}
	report, err := RunProfileComparisonWithDependencies(context.Background(), ProfileComparisonOptions{
		RunID:                  "profile-comparison-pass",
		Corpus:                 corpus,
		ImplementationRevision: "test-revision",
		SchemaVersion:          15,
	}, store, map[string]runtime.Embedder{
		runtime.ProductionRetrievalProfileID: incumbent,
		runtime.MigrationRetrievalProfileID:  candidate,
	})
	if err != nil {
		t.Fatal(err)
	}

	if !report.HardGates.Pass || len(report.Profiles) != 2 {
		t.Fatalf("comparison gates mismatch: %#v", report)
	}
	if report.AuthorityFingerprint == "" || report.CorpusSHA256 == "" || report.RequestFingerprint == "" {
		t.Fatalf("comparison identity is incomplete: %#v", report)
	}
	for _, profileID := range []string{runtime.ProductionRetrievalProfileID, runtime.MigrationRetrievalProfileID} {
		profile := profileComparisonReport(t, report, profileID)
		if !profile.HardGates.Pass || !profile.ProjectionRebuildEquivalent || profile.ProjectionVectorCount != 4 {
			t.Fatalf("profile %s did not qualify: %#v", profileID, profile)
		}
		if profile.Metrics.QueryCount != 1 || profile.Metrics.RecallAtK != 1 || profile.Metrics.MRR != 1 {
			t.Fatalf("profile %s metrics mismatch: %#v", profileID, profile.Metrics)
		}
		if got := profile.Queries[0].Results[0].RecordID; got != "current" {
			t.Fatalf("profile %s top result=%q, want current", profileID, got)
		}
		if profile.EmbeddingRequestCount == 0 || profile.ProjectionBuildDuration <= 0 {
			t.Fatalf("profile %s request/build evidence missing: %#v", profileID, profile)
		}
	}
	if report.Decision.CandidateProfileID != runtime.MigrationRetrievalProfileID {
		t.Fatalf("candidate decision mismatch: %#v", report.Decision)
	}
}

func TestEvaluateProfilePromotionRequiresSafetyQualityLatencyAndClearBenefit(t *testing.T) {
	policy := DefaultProfilePromotionPolicy()
	incumbent := ProfileComparisonReport{
		ProfileID: runtime.ProductionRetrievalProfileID,
		HardGates: HardGateReport{Pass: true}, ProjectionRebuildEquivalent: true,
		Metrics: Aggregate{QueryCount: 20, HitAt1: 0.90, RecallAtK: 1, MRR: 0.92, NDCGAtK: 0.93, SearchP95: 200 * time.Millisecond},
	}

	tests := []struct {
		name      string
		candidate ProfileComparisonReport
		promote   bool
		reason    string
	}{
		{
			name: "clear quality gain",
			candidate: ProfileComparisonReport{
				ProfileID: runtime.MigrationRetrievalProfileID,
				HardGates: HardGateReport{Pass: true}, ProjectionRebuildEquivalent: true,
				Metrics: Aggregate{QueryCount: 20, HitAt1: 0.95, RecallAtK: 1, MRR: 0.95, NDCGAtK: 0.95, SearchP95: 210 * time.Millisecond},
			},
			promote: true,
		},
		{
			name: "equivalent without benefit",
			candidate: ProfileComparisonReport{
				ProfileID: runtime.MigrationRetrievalProfileID,
				HardGates: HardGateReport{Pass: true}, ProjectionRebuildEquivalent: true,
				Metrics: Aggregate{QueryCount: 20, HitAt1: 0.90, RecallAtK: 1, MRR: 0.92, NDCGAtK: 0.93, SearchP95: 205 * time.Millisecond},
			},
			reason: "no_clear_benefit",
		},
		{
			name: "quality regression",
			candidate: ProfileComparisonReport{
				ProfileID: runtime.MigrationRetrievalProfileID,
				HardGates: HardGateReport{Pass: true}, ProjectionRebuildEquivalent: true,
				Metrics: Aggregate{QueryCount: 20, HitAt1: 0.90, RecallAtK: 0.95, MRR: 0.90, NDCGAtK: 0.91, SearchP95: 150 * time.Millisecond},
			},
			reason: "quality_regression",
		},
		{
			name: "unsafe result",
			candidate: ProfileComparisonReport{
				ProfileID: runtime.MigrationRetrievalProfileID,
				HardGates: HardGateReport{Pass: false, ForbiddenCount: 1}, ProjectionRebuildEquivalent: true,
				Metrics: Aggregate{QueryCount: 20, HitAt1: 1, RecallAtK: 1, MRR: 1, NDCGAtK: 1, SearchP95: 100 * time.Millisecond},
			},
			reason: "hard_gate_failed",
		},
		{
			name: "latency regression",
			candidate: ProfileComparisonReport{
				ProfileID: runtime.MigrationRetrievalProfileID,
				HardGates: HardGateReport{Pass: true}, ProjectionRebuildEquivalent: true,
				Metrics: Aggregate{QueryCount: 20, HitAt1: 0.95, RecallAtK: 1, MRR: 0.95, NDCGAtK: 0.95, SearchP95: 400 * time.Millisecond},
			},
			reason: "latency_regression",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			decision := EvaluateProfilePromotion(policy, incumbent, test.candidate)
			if decision.Promote != test.promote {
				t.Fatalf("promote=%t, want %t: %#v", decision.Promote, test.promote, decision)
			}
			if test.reason != "" && !containsProfileReason(decision.ReasonCodes, test.reason) {
				t.Fatalf("reason codes %v do not contain %q", decision.ReasonCodes, test.reason)
			}
		})
	}
}

func containsProfileReason(reasons []string, want string) bool {
	for _, reason := range reasons {
		if reason == want {
			return true
		}
	}
	return false
}

func profileComparisonReport(t *testing.T, report ProfileComparison, profileID string) ProfileComparisonReport {
	t.Helper()
	for _, profile := range report.Profiles {
		if profile.ProfileID == profileID {
			return profile
		}
	}
	t.Fatalf("missing profile %q", profileID)
	return ProfileComparisonReport{}
}

type profileTestEmbedder struct {
	queryNeedle      string
	preferredContent string
}

func (embedder profileTestEmbedder) Embed(_ context.Context, content string) ([]float32, error) {
	vector := make([]float32, 1024)
	switch {
	case strings.Contains(content, embedder.queryNeedle):
		vector[0] = 1
	case strings.Contains(content, embedder.preferredContent):
		vector[0] = 1
	default:
		vector[1] = 1
	}
	return vector, nil
}
