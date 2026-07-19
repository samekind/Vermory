package runtime

import (
	"context"
	"math"
	"strings"
	"testing"
	"unicode/utf8"
)

type embeddingPolicyTestEmbedder struct {
	inputs       [][]string
	next         int
	logicalItems int
}

func (embedder *embeddingPolicyTestEmbedder) RecordLogicalEmbeddingSuccess(items int) {
	embedder.logicalItems += items
}

func (embedder *embeddingPolicyTestEmbedder) Embed(context.Context, string) ([]float32, error) {
	panic("embedding policy test must use batch requests")
}

func (embedder *embeddingPolicyTestEmbedder) EmbedBatch(_ context.Context, texts []string) ([][]float32, error) {
	embedder.inputs = append(embedder.inputs, append([]string(nil), texts...))
	vectors := make([][]float32, len(texts))
	fixtures := [][]float32{{1, 0}, {0, 1}, {1, 1}}
	for index := range texts {
		vectors[index] = append([]float32(nil), fixtures[embedder.next]...)
		embedder.next++
	}
	return vectors, nil
}

func TestEmbeddingInputPolicyChunksUTF8Deterministically(t *testing.T) {
	policy := EmbeddingInputPolicy{
		ID:                EmbeddingInputPolicyUTF8ChunkedMeanV1,
		MaxChunkBytes:     10,
		ChunkOverlapBytes: 3,
		Pooling:           EmbeddingPoolingNormalizedMean,
	}
	input := "alpha-中文-beta-中文-gamma"
	first, err := splitEmbeddingInput(input, policy)
	if err != nil {
		t.Fatal(err)
	}
	second, err := splitEmbeddingInput(input, policy)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) < 3 || strings.Join(first, "\x00") != strings.Join(second, "\x00") {
		t.Fatalf("chunking is not deterministic: first=%q second=%q", first, second)
	}
	if first[0] != input[:len(first[0])] || !strings.HasSuffix(input, first[len(first)-1]) {
		t.Fatalf("chunks do not preserve input boundaries: %#v", first)
	}
	for index, chunk := range first {
		if !utf8.ValidString(chunk) || len(chunk) > policy.MaxChunkBytes || chunk == "" {
			t.Fatalf("invalid chunk %d: bytes=%d valid=%t value=%q", index, len(chunk), utf8.ValidString(chunk), chunk)
		}
	}
	profile := RetrievalProfile{InputPolicy: policy}
	count, err := EmbeddingInputCount(profile, input)
	if err != nil || count != len(first) {
		t.Fatalf("physical input count=%d/%v want %d", count, err, len(first))
	}
}

func TestEmbeddingInputPolicyAllowsZeroOverlap(t *testing.T) {
	policy := EmbeddingInputPolicy{
		ID:            EmbeddingInputPolicyUTF8ChunkedMeanV1,
		MaxChunkBytes: 5,
		Pooling:       EmbeddingPoolingNormalizedMean,
	}
	chunks, err := splitEmbeddingInput("abcdefghij", policy)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(chunks, "") != "abcdefghij" || len(chunks) != 2 {
		t.Fatalf("zero-overlap chunks=%q", chunks)
	}
}

func TestEmbeddingInputPolicyPoolsPhysicalChunksIntoOneLogicalVector(t *testing.T) {
	policy := EmbeddingInputPolicy{
		ID:                EmbeddingInputPolicyUTF8ChunkedMeanV1,
		MaxChunkBytes:     6,
		ChunkOverlapBytes: 2,
		Pooling:           EmbeddingPoolingNormalizedMean,
	}
	embedder := &embeddingPolicyTestEmbedder{}
	vectors, physicalItems, err := embedLogicalTexts(
		context.Background(), embedder, []string{"abcdefghijk"}, 2, 2, policy,
	)
	if err != nil {
		t.Fatal(err)
	}
	if physicalItems != 3 || len(vectors) != 1 || len(embedder.inputs) != 2 || len(embedder.inputs[0]) != 2 || len(embedder.inputs[1]) != 1 || embedder.logicalItems != 1 {
		t.Fatalf("unexpected physical embedding plan: items=%d vectors=%#v calls=%#v", physicalItems, vectors, embedder.inputs)
	}
	want := float32(1 / math.Sqrt2)
	if math.Abs(float64(vectors[0][0]-want)) > 1e-6 || math.Abs(float64(vectors[0][1]-want)) > 1e-6 {
		t.Fatalf("unexpected normalized mean vector: %#v want [%f %f]", vectors[0], want, want)
	}
}

func TestChunkedMeanRetrievalProfileIsIndependentCandidate(t *testing.T) {
	legacy, ok := SupportedRetrievalProfile(ProductionRetrievalProfileID)
	if !ok || legacy.InputPolicy != (EmbeddingInputPolicy{}) || legacy.Status != "active" {
		t.Fatalf("legacy production profile changed: %#v", legacy)
	}
	chunked, ok := SupportedRetrievalProfile(ChunkedMeanRetrievalProfileID)
	if !ok || chunked.Status != "candidate" || chunked.Model != legacy.Model ||
		chunked.InputPolicy.ID != EmbeddingInputPolicyUTF8ChunkedMeanV1 ||
		chunked.InputPolicy.MaxChunkBytes != 7500 || chunked.InputPolicy.ChunkOverlapBytes != 500 ||
		chunked.InputPolicy.Pooling != EmbeddingPoolingNormalizedMean {
		t.Fatalf("unexpected chunked candidate profile: %#v", chunked)
	}
}
