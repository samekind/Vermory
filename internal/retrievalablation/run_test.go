package retrievalablation

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"vermory/internal/runtime"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestRunValidatesExternalDependencies(t *testing.T) {
	tests := []struct {
		name    string
		options Options
		want    string
	}{
		{name: "database", options: Options{}, want: "database URL"},
		{name: "corpus", options: Options{DatabaseURL: "postgres://example"}, want: "corpus path"},
		{name: "run", options: Options{DatabaseURL: "postgres://example", CorpusPath: "corpus.json"}, want: "run_id"},
		{name: "embedding base", options: Options{DatabaseURL: "postgres://example", CorpusPath: "corpus.json", RunID: "run"}, want: "embedding base URL"},
		{name: "embedding key", options: Options{DatabaseURL: "postgres://example", CorpusPath: "corpus.json", RunID: "run", EmbeddingBaseURL: "https://example.com"}, want: "embedding API key"},
		{name: "embedding model", options: Options{DatabaseURL: "postgres://example", CorpusPath: "corpus.json", RunID: "run", EmbeddingBaseURL: "https://example.com", EmbeddingAPIKey: "secret"}, want: "embedding model"},
		{name: "dimensions", options: Options{DatabaseURL: "postgres://example", CorpusPath: "corpus.json", RunID: "run", EmbeddingBaseURL: "https://example.com", EmbeddingAPIKey: "secret", EmbeddingModel: "model"}, want: "embedding dimensions"},
		{name: "revision", options: Options{DatabaseURL: "postgres://example", CorpusPath: "corpus.json", RunID: "run", EmbeddingBaseURL: "https://example.com", EmbeddingAPIKey: "secret", EmbeddingModel: "model", EmbeddingDimensions: 3}, want: "implementation revision"},
		{name: "base url credentials", options: Options{DatabaseURL: "postgres://example", CorpusPath: "corpus.json", RunID: "run", EmbeddingBaseURL: "https://user:secret@example.com/v1", EmbeddingAPIKey: "secret", EmbeddingModel: "model", EmbeddingDimensions: 3, ImplementationRevision: "rev"}, want: "must not include credentials"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := Run(context.Background(), test.options); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Run error=%v, want substring %q", err, test.want)
			}
		})
	}
}

func TestRunUsesNativePostgreSQLVectorBackend(t *testing.T) {
	databaseURL := os.Getenv("VERMORY_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("VERMORY_TEST_DATABASE_URL is not set")
	}
	resetRetrievalRunDatabase(t, databaseURL)
	corpusPath := writeNativeRunCorpus(t)

	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/embeddings" || request.Header.Get("Authorization") != "Bearer test-key" {
			http.Error(response, "invalid request", http.StatusUnauthorized)
			return
		}
		calls.Add(1)
		var body struct {
			Input string `json:"input"`
		}
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			http.Error(response, err.Error(), http.StatusBadRequest)
			return
		}
		vector := []float64{0, 1, 0}
		if strings.Contains(body.Input, "current") {
			vector = []float64{1, 0, 0}
		}
		_ = json.NewEncoder(response).Encode(map[string]any{"data": []any{map[string]any{"embedding": vector, "index": 0}}, "model": "test-embedding"})
	}))
	defer server.Close()

	report, err := Run(context.Background(), Options{
		DatabaseURL: databaseURL, CorpusPath: corpusPath, RunID: "native-run",
		EmbeddingBaseURL: server.URL, EmbeddingAPIKey: "test-key", EmbeddingModel: "test-embedding",
		EmbeddingDimensions: 3, ImplementationRevision: "test-revision",
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.SchemaVersion != runtime.MaximumSupportedSchemaVersion || !report.HardGates.Pass || !report.ProjectionRebuildEquivalent {
		t.Fatalf("native run gates mismatch: %#v", report)
	}
	if vector := conditionReport(t, report, ConditionVector); vector.Metrics.RecallAtK != 1 {
		t.Fatalf("native vector report mismatch: %#v", vector)
	}
	if calls.Load() < 5 {
		t.Fatalf("embedding calls=%d, expected seed, query, rebuild, and replay calls", calls.Load())
	}
	if report.EmbeddingRequestCount != calls.Load() {
		t.Fatalf("reported embedding requests=%d, observed=%d", report.EmbeddingRequestCount, calls.Load())
	}
}

func TestRunNativeVectorOutageDegradesToLexical(t *testing.T) {
	databaseURL := os.Getenv("VERMORY_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("VERMORY_TEST_DATABASE_URL is not set")
	}
	resetRetrievalRunDatabase(t, databaseURL)
	corpusPath := writeNativeRunCorpus(t)
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		call := calls.Add(1)
		if call == 3 || call == 6 {
			http.Error(response, "forced embedding outage", http.StatusServiceUnavailable)
			return
		}
		_ = json.NewEncoder(response).Encode(map[string]any{"data": []any{map[string]any{"embedding": []float64{1, 0, 0}, "index": 0}}, "model": "test-embedding"})
	}))
	defer server.Close()

	report, err := Run(context.Background(), Options{
		DatabaseURL: databaseURL, CorpusPath: corpusPath, RunID: "native-outage",
		EmbeddingBaseURL: server.URL, EmbeddingAPIKey: "test-key", EmbeddingModel: "test-embedding",
		EmbeddingDimensions: 3, ImplementationRevision: "test-revision",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !report.HardGates.Pass || !report.ProjectionRebuildEquivalent || len(report.Failures) != 1 {
		t.Fatalf("outage report mismatch: %#v", report)
	}
	lexical := conditionReport(t, report, ConditionLexical).Queries[0]
	hybrid := conditionReport(t, report, ConditionHybrid).Queries[0]
	if !hybrid.DegradedToLexical || !reflect.DeepEqual(recordIDs(lexical.Results), recordIDs(hybrid.Results)) {
		t.Fatalf("outage did not preserve lexical order: lexical=%#v hybrid=%#v", lexical, hybrid)
	}
}

func resetRetrievalRunDatabase(t *testing.T, databaseURL string) {
	t.Helper()
	store, err := runtime.OpenStore(context.Background(), databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Migrate(context.Background()); err != nil {
		store.Close()
		t.Fatal(err)
	}
	if err := store.ResetForTest(context.Background()); err != nil {
		store.Close()
		t.Fatal(err)
	}
	store.Close()
	pool, err := pgxpool.New(context.Background(), databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(context.Background(), `DROP TABLE IF EXISTS contextmesh_memory_index`); err != nil {
		pool.Close()
		t.Fatal(err)
	}
	pool.Close()
	t.Cleanup(func() {
		pool, err := pgxpool.New(context.Background(), databaseURL)
		if err == nil {
			_, _ = pool.Exec(context.Background(), `DROP TABLE IF EXISTS contextmesh_memory_index`)
			pool.Close()
		}
	})
}

func writeNativeRunCorpus(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	caseDir := filepath.Join(root, "casebook", "cases", "101-example")
	if err := os.MkdirAll(caseDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(caseDir, "source.md"), []byte("fixture"), 0o644); err != nil {
		t.Fatal(err)
	}
	corpusDir := filepath.Join(root, "runtime", "cases", "W08-test")
	if err := os.MkdirAll(corpusDir, 0o755); err != nil {
		t.Fatal(err)
	}
	corpus := Corpus{
		Version: "1", Name: "native run",
		Scopes: []Scope{{ID: "workspace", TenantID: "run-local", Line: "workspace", Anchor: "/fixtures/run/workspace"}},
		Records: []Record{
			{ID: "current", ScopeID: "workspace", Content: "Use the current locked command.", Lifecycle: "active", ProvenanceCase: "101-example"},
			{ID: "distractor", ScopeID: "workspace", Content: "A gardening reminder about tomatoes.", Lifecycle: "active", ProvenanceCase: "101-example"},
		},
		Queries: []Query{{ID: "command", ScopeID: "workspace", Text: "current command", Limit: 1, RelevantRecordIDs: []string{"current"}, ForbiddenRecordIDs: []string{"distractor"}, TaskExcludedRecordIDs: []string{"distractor"}, Cohorts: []string{"semantic"}}},
	}
	payload, err := json.Marshal(corpus)
	if err != nil {
		t.Fatal(err)
	}
	corpusPath := filepath.Join(corpusDir, "corpus.json")
	if err := os.WriteFile(corpusPath, payload, 0o644); err != nil {
		t.Fatal(err)
	}
	return corpusPath
}
