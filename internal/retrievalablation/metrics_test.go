package retrievalablation

import (
	"math"
	"testing"
	"time"
)

func TestScoreQueryComputesRankingAndViolationMetrics(t *testing.T) {
	query := Query{
		ID:                 "multi",
		RelevantRecordIDs:  []string{"relevant-a", "relevant-b"},
		ForbiddenRecordIDs: []string{"forbidden"},
	}
	results := []RankedResult{
		{RecordID: "distractor", Eligible: true},
		{RecordID: "relevant-a", Eligible: true},
		{RecordID: "forbidden", Eligible: true},
		{RecordID: "relevant-b", Eligible: true},
		{RecordID: "ineligible", Eligible: false},
	}

	metrics := ScoreQuery(query, results)
	if metrics.HitAt1 != 0 || metrics.RecallAtK != 1 || math.Abs(metrics.MRR-0.5) > 1e-12 {
		t.Fatalf("ranking metrics mismatch: %#v", metrics)
	}
	wantNDCG := (1/math.Log2(3) + 1/math.Log2(5)) / (1 + 1/math.Log2(3))
	if math.Abs(metrics.NDCGAtK-wantNDCG) > 1e-12 {
		t.Fatalf("ndcg=%v, want %v", metrics.NDCGAtK, wantNDCG)
	}
	if metrics.ForbiddenCount != 1 || metrics.IneligibleCount != 1 {
		t.Fatalf("violation metrics mismatch: %#v", metrics)
	}
}

func TestScoreQueryHandlesNoResults(t *testing.T) {
	metrics := ScoreQuery(Query{RelevantRecordIDs: []string{"relevant"}}, nil)
	if metrics.HitAt1 != 0 || metrics.RecallAtK != 0 || metrics.MRR != 0 || metrics.NDCGAtK != 0 {
		t.Fatalf("empty result metrics mismatch: %#v", metrics)
	}
}

func TestScoreQueryNDCGPenalizesMissingRelevantResults(t *testing.T) {
	metrics := ScoreQuery(Query{Limit: 5, RelevantRecordIDs: []string{"a", "b"}}, []RankedResult{{RecordID: "a", Eligible: true}})
	want := 1 / (1 + 1/math.Log2(3))
	if math.Abs(metrics.NDCGAtK-want) > 1e-12 {
		t.Fatalf("ndcg=%v, want %v", metrics.NDCGAtK, want)
	}
}

func TestScoreQueryDoesNotDoubleCountDuplicateRelevantResults(t *testing.T) {
	query := Query{Limit: 3, RelevantRecordIDs: []string{"a"}}
	metrics := ScoreQuery(query, []RankedResult{
		{RecordID: "a", Eligible: true},
		{RecordID: "a", Eligible: true},
	})
	if metrics.RecallAtK != 1 || metrics.MRR != 1 || metrics.NDCGAtK != 1 {
		t.Fatalf("duplicate relevant result inflated metrics: %#v", metrics)
	}
}

func TestAggregateMetricsBuildsConditionAndCohortViews(t *testing.T) {
	reports := []QueryReport{
		{
			QueryID:  "q1",
			Cohorts:  []string{"semantic", "mixed"},
			Duration: 10 * time.Millisecond,
			Metrics:  QueryMetrics{HitAt1: 1, RecallAtK: 1, MRR: 1, NDCGAtK: 1},
		},
		{
			QueryID:  "q2",
			Cohorts:  []string{"semantic"},
			Duration: 30 * time.Millisecond,
			Metrics:  QueryMetrics{HitAt1: 0, RecallAtK: 0.5, MRR: 0.5, NDCGAtK: 0.6, ForbiddenCount: 1, IneligibleCount: 2},
		},
	}

	aggregate := AggregateMetrics(reports)
	if aggregate.QueryCount != 2 || aggregate.HitAt1 != 0.5 || aggregate.RecallAtK != 0.75 || aggregate.MRR != 0.75 || aggregate.NDCGAtK != 0.8 {
		t.Fatalf("aggregate mismatch: %#v", aggregate)
	}
	if aggregate.ForbiddenCount != 1 || aggregate.IneligibleCount != 2 || aggregate.SearchP50 != 10*time.Millisecond || aggregate.SearchP95 != 10*time.Millisecond {
		t.Fatalf("aggregate counts/latency mismatch: %#v", aggregate)
	}
	cohorts := AggregateCohorts(reports)
	if semantic := cohorts["semantic"]; semantic.QueryCount != 2 || semantic.MRR != 0.75 {
		t.Fatalf("semantic cohort mismatch: %#v", semantic)
	}
	if mixed := cohorts["mixed"]; mixed.QueryCount != 1 || mixed.MRR != 1 {
		t.Fatalf("mixed cohort mismatch: %#v", mixed)
	}
}
