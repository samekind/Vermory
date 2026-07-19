package memorybackend

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

var errInvalidEmbeddingBatch = errors.New("invalid embedding batch response")

type openAIEmbedder struct {
	remote     remoteClient
	model      string
	dimensions int
}

type Embedder interface {
	Embed(context.Context, string) ([]float32, error)
}

func NewOpenAIEmbedder(baseURL, apiKey, model string, dimensions int, client *http.Client) (Embedder, error) {
	return newOpenAIEmbedder(baseURL, apiKey, model, dimensions, client)
}

func newOpenAIEmbedder(baseURL, apiKey, model string, dimensions int, client *http.Client) (*openAIEmbedder, error) {
	if strings.TrimSpace(model) == "" {
		return nil, fmt.Errorf("embedding model is required")
	}
	if dimensions <= 0 {
		return nil, fmt.Errorf("embedding dimensions must be positive")
	}
	headers := map[string]string{}
	if apiKey != "" {
		headers["Authorization"] = "Bearer " + apiKey
	}
	remote, err := newRemoteClient(baseURL, client, headers)
	if err != nil {
		return nil, err
	}
	return &openAIEmbedder{remote: remote, model: model, dimensions: dimensions}, nil
}

func (e *openAIEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	vectors, err := e.embed(ctx, text, 1)
	if err != nil {
		return nil, err
	}
	return vectors[0], nil
}

func (e *openAIEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return [][]float32{}, nil
	}
	return e.embed(ctx, texts, len(texts))
}

func (e *openAIEmbedder) embed(ctx context.Context, input any, expected int) ([][]float32, error) {
	body := map[string]any{"model": e.model, "input": input}
	var response struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
			Index     *int      `json:"index"`
		} `json:"data"`
	}
	if err := e.remote.doJSON(ctx, http.MethodPost, "/embeddings", body, &response); err != nil {
		return nil, err
	}
	if len(response.Data) != expected {
		return nil, fmt.Errorf("%w: embedding provider returned %d vectors, want %d", errInvalidEmbeddingBatch, len(response.Data), expected)
	}
	vectors := make([][]float32, expected)
	seen := make([]bool, expected)
	for _, item := range response.Data {
		if item.Index == nil || *item.Index < 0 || *item.Index >= expected {
			return nil, fmt.Errorf("%w: embedding provider returned an out-of-range index", errInvalidEmbeddingBatch)
		}
		if seen[*item.Index] {
			return nil, fmt.Errorf("%w: embedding provider returned duplicate index %d", errInvalidEmbeddingBatch, *item.Index)
		}
		if len(item.Embedding) != e.dimensions {
			return nil, fmt.Errorf("%w: embedding provider returned %d dimensions, want %d", errInvalidEmbeddingBatch, len(item.Embedding), e.dimensions)
		}
		seen[*item.Index] = true
		vectors[*item.Index] = item.Embedding
	}
	return vectors, nil
}

func vectorLiteral(vector []float32) string {
	parts := make([]string, len(vector))
	for index, value := range vector {
		parts[index] = strconv.FormatFloat(float64(value), 'g', -1, 32)
	}
	return "[" + strings.Join(parts, ",") + "]"
}
