package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	vermoryruntime "vermory/internal/runtime"
)

type longMemEvalFlakyBatchEmbedder struct {
	failures int
	attempts int
}

func (embedder *longMemEvalFlakyBatchEmbedder) Embed(context.Context, string) ([]float32, error) {
	return make([]float32, 1024), nil
}

func (embedder *longMemEvalFlakyBatchEmbedder) EmbedBatch(_ context.Context, texts []string) ([][]float32, error) {
	embedder.attempts++
	if embedder.attempts <= embedder.failures {
		return nil, errors.New("transient provider outage")
	}
	vectors := make([][]float32, len(texts))
	for index := range vectors {
		vectors[index] = make([]float32, 1024)
	}
	return vectors, nil
}

func TestLongMemEvalVectorProfileV2UsesBoundedLinearRecovery(t *testing.T) {
	root, err := projectRoot()
	if err != nil {
		t.Fatal(err)
	}
	profile, digest, err := loadLongMemEvalVectorProfile(filepath.Join(root, "casebook/benchmarks/profiles/longmemeval-s-vector-retrieval-v2.json"))
	if err != nil {
		t.Fatal(err)
	}
	if digest == "" || profile.ID != "w28-longmemeval-s-vector-retrieval-v2" || profile.MaxAttempts != 5 || profile.RetryBackoff != "linear" {
		t.Fatalf("unexpected v2 retry profile: digest=%q profile=%#v", digest, profile)
	}
	delegate := &longMemEvalFlakyBatchEmbedder{failures: 3}
	embedder, err := newLongMemEvalRetryingEmbedder(delegate, profile)
	if err != nil {
		t.Fatal(err)
	}
	var delays []time.Duration
	embedder.sleeper = func(_ context.Context, delay time.Duration) error {
		delays = append(delays, delay)
		return nil
	}
	vectors, err := embedder.EmbedBatch(context.Background(), []string{"first", "second"})
	if err != nil {
		t.Fatal(err)
	}
	if len(vectors) != 2 || delegate.attempts != 4 {
		t.Fatalf("transient batch was not recovered: vectors=%d attempts=%d", len(vectors), delegate.attempts)
	}
	wantDelays := []time.Duration{2 * time.Second, 4 * time.Second, 6 * time.Second}
	if len(delays) != len(wantDelays) {
		t.Fatalf("unexpected retry delays: %#v", delays)
	}
	for index := range delays {
		if delays[index] != wantDelays[index] {
			t.Fatalf("retry delay %d=%s want %s", index, delays[index], wantDelays[index])
		}
	}
	stats := embedder.stats()
	if stats.LogicalOperations != 1 || stats.ProviderAttempts != 4 || stats.FailedAttempts != 3 || stats.RetriedOperations != 1 || stats.SuccessfulItems != 2 || stats.TerminalFailures != 0 {
		t.Fatalf("unexpected retry evidence: %#v", stats)
	}
}

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
