package benchmark

import (
	"math"
	"strings"
	"testing"
)

func TestEvaluateSessionRetrievalMatchesUpstreamRecallAndNDCG(t *testing.T) {
	ranked := []string{"a", "b", "c"}
	relevant := []string{"a", "c"}

	atTwo, err := EvaluateSessionRetrieval(ranked, relevant, 2)
	if err != nil {
		t.Fatal(err)
	}
	if atTwo.RecallAny != 1 || atTwo.RecallAll != 0 || atTwo.FirstRelevantRank != 1 || atTwo.ReciprocalRank != 1 {
		t.Fatalf("unexpected K=2 metrics: %#v", atTwo)
	}
	if math.Abs(atTwo.NDCG-0.5) > 1e-9 {
		t.Fatalf("unexpected K=2 NDCG: %#v", atTwo)
	}

	atThree, err := EvaluateSessionRetrieval(ranked, relevant, 3)
	if err != nil {
		t.Fatal(err)
	}
	wantNDCG := (1 + 1/math.Log2(3)) / 2
	if atThree.RecallAny != 1 || atThree.RecallAll != 1 || math.Abs(atThree.NDCG-wantNDCG) > 1e-9 {
		t.Fatalf("unexpected K=3 metrics: got=%#v want_ndcg=%f", atThree, wantNDCG)
	}
}

func TestEvaluateSessionRetrievalRejectsInvalidRankings(t *testing.T) {
	tests := []struct {
		name     string
		ranked   []string
		relevant []string
		k        int
		contains string
	}{
		{name: "empty ranked", ranked: []string{""}, relevant: []string{"a"}, k: 1, contains: "empty"},
		{name: "no relevant", ranked: []string{"a"}, relevant: nil, k: 1, contains: "relevant session"},
		{name: "invalid k", ranked: []string{"a"}, relevant: []string{"a"}, k: 0, contains: "positive"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := EvaluateSessionRetrieval(test.ranked, test.relevant, test.k); err == nil || !strings.Contains(err.Error(), test.contains) {
				t.Fatalf("expected %q error, got %v", test.contains, err)
			}
		})
	}
}

func TestEvaluateSessionRetrievalPreservesRepeatedCorpusPositions(t *testing.T) {
	metric, err := EvaluateSessionRetrieval([]string{"noise", "noise", "answer"}, []string{"answer", "answer"}, 2)
	if err != nil {
		t.Fatal(err)
	}
	if metric.RecallAny != 0 || metric.RecallAll != 0 {
		t.Fatalf("repeated distractor positions were collapsed: %#v", metric)
	}
	metric, err = EvaluateSessionRetrieval([]string{"noise", "noise", "answer"}, []string{"answer", "answer"}, 3)
	if err != nil {
		t.Fatal(err)
	}
	if metric.RecallAny != 1 || metric.RecallAll != 1 || metric.FirstRelevantRank != 3 {
		t.Fatalf("repeated corpus positions were not retained: %#v", metric)
	}
}

func TestEvaluateSessionRetrievalReportsMissesAndAggregateMeans(t *testing.T) {
	miss, err := EvaluateSessionRetrieval([]string{"noise"}, []string{"answer"}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if miss.RecallAny != 0 || miss.RecallAll != 0 || miss.NDCG != 0 || miss.FirstRelevantRank != 0 || miss.ReciprocalRank != 0 {
		t.Fatalf("unexpected miss metrics: %#v", miss)
	}
	hit, err := EvaluateSessionRetrieval([]string{"answer"}, []string{"answer"}, 1)
	if err != nil {
		t.Fatal(err)
	}
	aggregate := AggregateSessionRetrieval([]SessionRetrievalMetric{miss, hit})
	if aggregate.Count != 2 || aggregate.MeanRecallAny != 0.5 || aggregate.MeanRecallAll != 0.5 || aggregate.MeanNDCG != 0.5 || aggregate.MeanMRR != 0.5 {
		t.Fatalf("unexpected aggregate: %#v", aggregate)
	}
}
