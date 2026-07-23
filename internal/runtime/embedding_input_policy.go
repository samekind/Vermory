package runtime

import (
	"context"
	"fmt"
	"math"
	"unicode/utf8"
)

const (
	EmbeddingInputPolicyUTF8ChunkedMeanV1 = "utf8-byte-chunks-v1"
	EmbeddingPoolingNormalizedMean        = "normalized_mean"
	semanticQueryEmbeddingBatchSize       = 16
)

type EmbeddingInputPolicy struct {
	ID                string
	MaxChunkBytes     int
	ChunkOverlapBytes int
	Pooling           string
}

func (policy EmbeddingInputPolicy) validate() error {
	if policy.ID == "" {
		if policy != (EmbeddingInputPolicy{}) {
			return fmt.Errorf("embedding input policy without an ID must be empty")
		}
		return nil
	}
	if policy.ID != EmbeddingInputPolicyUTF8ChunkedMeanV1 {
		return fmt.Errorf("unsupported embedding input policy %q", policy.ID)
	}
	if policy.MaxChunkBytes <= 0 {
		return fmt.Errorf("embedding maximum chunk bytes must be positive")
	}
	if policy.ChunkOverlapBytes < 0 || policy.ChunkOverlapBytes >= policy.MaxChunkBytes {
		return fmt.Errorf("embedding chunk overlap must be non-negative and smaller than the maximum chunk size")
	}
	if policy.Pooling != EmbeddingPoolingNormalizedMean {
		return fmt.Errorf("unsupported embedding pooling %q", policy.Pooling)
	}
	return nil
}

func splitEmbeddingInput(input string, policy EmbeddingInputPolicy) ([]string, error) {
	if err := policy.validate(); err != nil {
		return nil, err
	}
	if policy.ID == "" {
		return []string{input}, nil
	}
	if !utf8.ValidString(input) {
		return nil, fmt.Errorf("embedding input must be valid UTF-8")
	}
	if len(input) <= policy.MaxChunkBytes {
		return []string{input}, nil
	}

	chunks := make([]string, 0, len(input)/policy.MaxChunkBytes+1)
	for start := 0; start < len(input); {
		end := start + policy.MaxChunkBytes
		if end >= len(input) {
			chunks = append(chunks, input[start:])
			break
		}
		for end > start && !utf8.RuneStart(input[end]) {
			end--
		}
		if end == start {
			return nil, fmt.Errorf("embedding maximum chunk bytes cannot contain one UTF-8 code point")
		}
		chunks = append(chunks, input[start:end])

		next := end - policy.ChunkOverlapBytes
		if next <= start {
			next = end
		}
		for next < end && !utf8.RuneStart(input[next]) {
			next++
		}
		if next <= start || next > end {
			return nil, fmt.Errorf("embedding chunk policy cannot make progress")
		}
		start = next
	}
	return chunks, nil
}

func EmbeddingInputCount(profile RetrievalProfile, input string) (int, error) {
	chunks, err := splitEmbeddingInput(input, profile.InputPolicy)
	if err != nil {
		return 0, err
	}
	return len(chunks), nil
}

type logicalEmbeddingRange struct {
	start int
	end   int
}

type embeddingDimensionError struct {
	got  int
	want int
}

func (err embeddingDimensionError) Error() string {
	return fmt.Sprintf("embedding returned %d dimensions, want %d", err.got, err.want)
}

func embedLogicalTexts(
	ctx context.Context,
	embedder Embedder,
	texts []string,
	batchSize int,
	dimensions int,
	policy EmbeddingInputPolicy,
) ([][]float32, int, error) {
	if embedder == nil {
		return nil, 0, fmt.Errorf("embedding provider is required")
	}
	if batchSize <= 0 {
		return nil, 0, fmt.Errorf("embedding batch size must be positive")
	}
	if dimensions <= 0 {
		return nil, 0, fmt.Errorf("embedding dimensions must be positive")
	}
	if err := policy.validate(); err != nil {
		return nil, 0, err
	}
	if len(texts) == 0 {
		return [][]float32{}, 0, nil
	}

	physical := make([]string, 0, len(texts))
	ranges := make([]logicalEmbeddingRange, len(texts))
	for index, input := range texts {
		chunks, err := splitEmbeddingInput(input, policy)
		if err != nil {
			return nil, 0, err
		}
		ranges[index] = logicalEmbeddingRange{start: len(physical), end: len(physical) + len(chunks)}
		physical = append(physical, chunks...)
	}

	physicalVectors := make([][]float32, 0, len(physical))
	batchEmbedder, batchCapable := embedder.(BatchEmbedder)
	for start := 0; start < len(physical); start += batchSize {
		end := start + batchSize
		if end > len(physical) {
			end = len(physical)
		}
		if batchCapable && batchSize > 1 {
			vectors, err := batchEmbedder.EmbedBatch(ctx, physical[start:end])
			if err != nil {
				return nil, 0, err
			}
			if len(vectors) != end-start {
				return nil, 0, fmt.Errorf("embedding batch returned %d vectors, want %d", len(vectors), end-start)
			}
			for _, vector := range vectors {
				if len(vector) != dimensions {
					return nil, 0, embeddingDimensionError{got: len(vector), want: dimensions}
				}
			}
			physicalVectors = append(physicalVectors, vectors...)
			continue
		}
		for _, input := range physical[start:end] {
			vector, err := embedder.Embed(ctx, input)
			if err != nil {
				return nil, 0, err
			}
			if len(vector) != dimensions {
				return nil, 0, embeddingDimensionError{got: len(vector), want: dimensions}
			}
			physicalVectors = append(physicalVectors, vector)
		}
	}

	logicalVectors := make([][]float32, len(ranges))
	for index, itemRange := range ranges {
		vectors := physicalVectors[itemRange.start:itemRange.end]
		if len(vectors) == 1 {
			logicalVectors[index] = vectors[0]
			continue
		}
		means := make([]float64, dimensions)
		var squaredNorm float64
		for dimension := 0; dimension < dimensions; dimension++ {
			var sum float64
			for _, vector := range vectors {
				sum += float64(vector[dimension])
			}
			mean := sum / float64(len(vectors))
			means[dimension] = mean
			squaredNorm += mean * mean
		}
		if squaredNorm == 0 || math.IsNaN(squaredNorm) || math.IsInf(squaredNorm, 0) {
			return nil, 0, fmt.Errorf("embedding pooled vector has invalid norm")
		}
		norm := math.Sqrt(squaredNorm)
		pooled := make([]float32, dimensions)
		for dimension := range means {
			pooled[dimension] = float32(means[dimension] / norm)
		}
		logicalVectors[index] = pooled
	}
	if observer, ok := embedder.(LogicalEmbeddingObserver); ok {
		observer.RecordLogicalEmbeddingSuccess(len(logicalVectors))
	}
	return logicalVectors, len(physical), nil
}
