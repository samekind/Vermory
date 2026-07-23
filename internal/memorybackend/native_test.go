package memorybackend

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOpenAIEmbedderUsesConfiguredModel(t *testing.T) {
	var request map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/embeddings" {
			http.Error(w, "unexpected path", http.StatusNotFound)
			return
		}
		defer r.Body.Close()
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"embedding":[0.1,0.2,0.3],"index":0}],"model":"bge-m3"}`))
	}))
	defer server.Close()

	embedder, err := newOpenAIEmbedder(server.URL, "test-key", "bge-m3", 3, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	vector, err := embedder.Embed(context.Background(), "中文 mixed identifier checkout_eta_v2")
	if err != nil {
		t.Fatal(err)
	}
	if len(vector) != 3 || vector[2] != 0.3 {
		t.Fatalf("unexpected vector: %#v", vector)
	}
	if request["model"] != "bge-m3" || request["input"] != "中文 mixed identifier checkout_eta_v2" {
		t.Fatalf("unexpected embedding request: %#v", request)
	}
	if _, exists := request["dimensions"]; exists {
		t.Fatalf("fixed-dimension embedding request must not send optional dimensions: %#v", request)
	}
}

func TestOpenAIEmbedderBatchUsesIndexesAndValidatesCompleteResponse(t *testing.T) {
	t.Run("reorders vectors by provider index", func(t *testing.T) {
		var input []string
		server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
			var body struct {
				Model string   `json:"model"`
				Input []string `json:"input"`
			}
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			input = body.Input
			_ = json.NewEncoder(response).Encode(map[string]any{
				"data": []any{
					map[string]any{"embedding": []float32{0.4, 0.5, 0.6}, "index": 1},
					map[string]any{"embedding": []float32{0.1, 0.2, 0.3}, "index": 0},
				},
			})
		}))
		defer server.Close()

		embedder, err := newOpenAIEmbedder(server.URL, "test-key", "bge-m3", 3, server.Client())
		if err != nil {
			t.Fatal(err)
		}
		vectors, err := embedder.EmbedBatch(context.Background(), []string{"first", "second"})
		if err != nil {
			t.Fatal(err)
		}
		if len(input) != 2 || input[0] != "first" || input[1] != "second" {
			t.Fatalf("unexpected batch input: %#v", input)
		}
		if len(vectors) != 2 || vectors[0][0] != 0.1 || vectors[1][0] != 0.4 {
			t.Fatalf("provider indexes were not honored: %#v", vectors)
		}
	})

	for _, test := range []struct {
		name string
		data []map[string]any
		want string
	}{
		{name: "missing vector", data: []map[string]any{{"embedding": []float32{0.1, 0.2, 0.3}, "index": 0}}, want: "returned 1 vectors"},
		{name: "duplicate index", data: []map[string]any{{"embedding": []float32{0.1, 0.2, 0.3}, "index": 0}, {"embedding": []float32{0.4, 0.5, 0.6}, "index": 0}}, want: "duplicate index"},
		{name: "out of range", data: []map[string]any{{"embedding": []float32{0.1, 0.2, 0.3}, "index": 0}, {"embedding": []float32{0.4, 0.5, 0.6}, "index": 2}}, want: "out-of-range index"},
		{name: "wrong dimensions", data: []map[string]any{{"embedding": []float32{0.1, 0.2, 0.3}, "index": 0}, {"embedding": []float32{0.4}, "index": 1}}, want: "dimensions"},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
				_ = json.NewEncoder(response).Encode(map[string]any{"data": test.data})
			}))
			defer server.Close()
			embedder, err := newOpenAIEmbedder(server.URL, "test-key", "bge-m3", 3, server.Client())
			if err != nil {
				t.Fatal(err)
			}
			_, err = embedder.EmbedBatch(context.Background(), []string{"first", "second"})
			if err == nil || !errors.Is(err, errInvalidEmbeddingBatch) || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("unexpected batch validation error: %v", err)
			}
		})
	}
}

func TestExportedOpenAIEmbedderUsesTheValidatedRequestPath(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if _, exists := body["dimensions"]; exists {
			t.Fatalf("exported embedder sent unsupported dimensions: %#v", body)
		}
		_ = json.NewEncoder(response).Encode(map[string]any{
			"data": []any{map[string]any{"embedding": []float32{0.1, 0.2, 0.3}, "index": 0}},
		})
	}))
	defer server.Close()

	embedder, err := NewOpenAIEmbedder(server.URL, "test-key", "test-model", 3, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	vector, err := embedder.Embed(context.Background(), "exported request")
	if err != nil {
		t.Fatal(err)
	}
	if len(vector) != 3 {
		t.Fatalf("unexpected exported vector: %#v", vector)
	}
}

func TestVectorLiteral(t *testing.T) {
	if got := vectorLiteral([]float32{0.25, -1, 3.5}); got != "[0.25,-1,3.5]" {
		t.Fatalf("unexpected vector literal %q", got)
	}
}
