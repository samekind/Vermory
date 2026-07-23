package retrievalablation

import (
	"math"
	"reflect"
	"testing"
)

func TestFuseRRFUsesExactGuardAndDeterministicRanks(t *testing.T) {
	lexical := []RankedResult{
		{MemoryID: "memory-a", RecordID: "a", Content: "Semantic release guidance.", Score: 99},
		{MemoryID: "memory-b", RecordID: "b", Content: "Use workspace_route_v3 for routing."},
	}
	vector := []RankedResult{
		{MemoryID: "memory-b", RecordID: "b", Content: "Use workspace_route_v3 for routing."},
		{MemoryID: "memory-a", RecordID: "a", Content: "Semantic release guidance."},
		{MemoryID: "memory-c", RecordID: "c", Content: "A third result."},
	}

	got := FuseRRF("workspace_route_v3", 3, lexical, vector)
	if ids := rankedRecordIDs(got); !reflect.DeepEqual(ids, []string{"b", "a", "c"}) {
		t.Fatalf("unexpected fused order: %v", ids)
	}
	if !got[0].Exact || got[0].LexicalRank != 2 || got[0].VectorRank != 1 {
		t.Fatalf("exact result metadata mismatch: %#v", got[0])
	}
	wantScore := 1.0/float64(RRFK+2) + 1.0/float64(RRFK+1)
	if math.Abs(got[0].Score-wantScore) > 1e-12 {
		t.Fatalf("rrf score=%v, want %v", got[0].Score, wantScore)
	}
	if got[2].LexicalRank != 0 || got[2].VectorRank != 3 {
		t.Fatalf("missing-rank metadata mismatch: %#v", got[2])
	}
	wantA := 1.0/float64(RRFK+1) + 1.0/float64(RRFK+2)
	if math.Abs(got[1].Score-wantA) > 1e-12 {
		t.Fatalf("input backend score leaked into RRF: score=%v want=%v", got[1].Score, wantA)
	}
}

func TestFuseRRFDeduplicatesAndUsesStableTieBreaks(t *testing.T) {
	lexical := []RankedResult{
		{MemoryID: "memory-b", RecordID: "b", Content: "B"},
		{MemoryID: "memory-a", RecordID: "a", Content: "A"},
		{MemoryID: "memory-b", RecordID: "b", Content: "B duplicate"},
	}
	vector := []RankedResult{
		{MemoryID: "memory-a", RecordID: "a", Content: "A"},
		{MemoryID: "memory-b", RecordID: "b", Content: "B"},
		{MemoryID: "memory-c", RecordID: "c", Content: "C"},
	}

	got := FuseRRF("unmatched", 2, lexical, vector)
	if ids := rankedRecordIDs(got); !reflect.DeepEqual(ids, []string{"b", "a"}) {
		t.Fatalf("unexpected deterministic order: %v", ids)
	}
	if len(got) != 2 {
		t.Fatalf("limit not enforced: %d", len(got))
	}
}

func TestFallbackToLexicalPreservesIDsAndOrder(t *testing.T) {
	lexical := []RankedResult{
		{MemoryID: "memory-c", RecordID: "c", Content: "C", Score: 0.9},
		{MemoryID: "memory-a", RecordID: "a", Content: "A", Score: 0.8},
		{MemoryID: "memory-b", RecordID: "b", Content: "B", Score: 0.7},
	}
	got := FallbackToLexical(lexical, 2)
	if ids := rankedRecordIDs(got); !reflect.DeepEqual(ids, []string{"c", "a"}) {
		t.Fatalf("fallback order changed: %v", ids)
	}
	if got[0].LexicalRank != 1 || got[1].LexicalRank != 2 {
		t.Fatalf("fallback ranks missing: %#v", got)
	}
	got[0].RecordID = "mutated"
	if lexical[0].RecordID != "c" {
		t.Fatal("fallback returned aliases into lexical input")
	}
}

func rankedRecordIDs(results []RankedResult) []string {
	ids := make([]string, len(results))
	for index, result := range results {
		ids[index] = result.RecordID
	}
	return ids
}
