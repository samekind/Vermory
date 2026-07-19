package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	vermoryruntime "vermory/internal/runtime"
)

func TestLiveLongMemEvalVectorProfileBatch(t *testing.T) {
	if os.Getenv("VERMORY_W28_LIVE_EMBEDDING") != "1" {
		t.Skip("VERMORY_W28_LIVE_EMBEDDING=1 is required")
	}
	apiKey := os.Getenv("VERMORY_LIVE_EMBEDDING_API_KEY")
	if apiKey == "" {
		t.Skip("VERMORY_LIVE_EMBEDDING_API_KEY is required")
	}
	profile := validLongMemEvalVectorProfile(t)
	embedder, err := configureLongMemEvalVectorEmbedder(LongMemEvalRetrievalOptions{EmbeddingAPIKey: apiKey}, profile)
	if err != nil {
		t.Fatal(err)
	}
	vectors, err := embedder.EmbedBatch(context.Background(), []string{
		"The current deployment region is Singapore.",
		"The maintenance window is Friday at 22:30.",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(vectors) != 2 || len(vectors[0]) != profile.Dimensions || len(vectors[1]) != profile.Dimensions {
		t.Fatalf("unexpected live batch dimensions: %d / %d / %d", len(vectors), len(vectors[0]), len(vectors[1]))
	}
	stats := embedder.stats()
	if stats.LogicalOperations != 1 || stats.ProviderAttempts < 1 || stats.SuccessfulItems != 2 || stats.TerminalFailures != 0 {
		t.Fatalf("unexpected live batch accounting: %#v", stats)
	}
}

func TestFrozenLongMemEvalVectorProfileMatchesRegisteredProductionPath(t *testing.T) {
	root, err := projectRoot()
	if err != nil {
		t.Fatal(err)
	}
	profile, digest, err := loadLongMemEvalVectorProfile(filepath.Join(root, "casebook/benchmarks/profiles/longmemeval-s-vector-retrieval-v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	if digest == "" || profile.ID != "w28-longmemeval-s-vector-retrieval-v1" {
		t.Fatalf("unexpected frozen vector profile: digest=%q profile=%#v", digest, profile)
	}
	if profile.RetrievalProfile != vermoryruntime.ProductionRetrievalProfileID || profile.Provider != "siliconflow-direct" || profile.EmbeddingBatchSize != 16 || profile.WorkerBatchSize != 256 {
		t.Fatalf("frozen production vector path drifted: %#v", profile)
	}
	if len(profile.Conditions) != 3 || profile.Conditions[2] != longMemEvalRetrievalVector {
		t.Fatalf("unexpected W28 conditions: %#v", profile.Conditions)
	}
}

func TestLongMemEvalVectorProfileRejectsRuntimeRegistryDrift(t *testing.T) {
	profile := validLongMemEvalVectorProfile(t)
	profile.Model = "unregistered-model"
	if err := profile.Validate(); err == nil {
		t.Fatal("vector profile accepted a model outside the runtime registry")
	}
	profile = validLongMemEvalVectorProfile(t)
	profile.Conditions = []string{longMemEvalRetrievalBaseline, longMemEvalRetrievalVermory}
	if err := profile.Validate(); err == nil {
		t.Fatal("vector profile accepted the legacy two-condition contract")
	}
}

func validLongMemEvalVectorProfile(t *testing.T) LongMemEvalVectorProfile {
	t.Helper()
	root, err := projectRoot()
	if err != nil {
		t.Fatal(err)
	}
	profile, _, err := loadLongMemEvalVectorProfile(filepath.Join(root, "casebook/benchmarks/profiles/longmemeval-s-vector-retrieval-v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	return profile
}
