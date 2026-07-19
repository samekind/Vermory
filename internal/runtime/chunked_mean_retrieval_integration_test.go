package runtime

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"vermory/internal/memorybackend"
)

type chunkedMeanRecordingEmbedder struct {
	mu     sync.Mutex
	inputs []string
}

type chunkedMeanBoundedLiveEmbedder struct {
	delegate      Embedder
	batchDelegate BatchEmbedder
	mu            sync.Mutex
	items         int
	maxInputBytes int
}

func (embedder *chunkedMeanBoundedLiveEmbedder) Embed(ctx context.Context, input string) ([]float32, error) {
	if err := embedder.observe(input); err != nil {
		return nil, err
	}
	return embedder.delegate.Embed(ctx, input)
}

func (embedder *chunkedMeanBoundedLiveEmbedder) EmbedBatch(ctx context.Context, inputs []string) ([][]float32, error) {
	for _, input := range inputs {
		if err := embedder.observe(input); err != nil {
			return nil, err
		}
	}
	return embedder.batchDelegate.EmbedBatch(ctx, inputs)
}

func (embedder *chunkedMeanBoundedLiveEmbedder) observe(input string) error {
	if !utf8.ValidString(input) || len(input) == 0 || len(input) > 7500 {
		return fmt.Errorf("live embedding input violates the registered byte contract")
	}
	embedder.mu.Lock()
	defer embedder.mu.Unlock()
	embedder.items++
	if len(input) > embedder.maxInputBytes {
		embedder.maxInputBytes = len(input)
	}
	return nil
}

func (embedder *chunkedMeanBoundedLiveEmbedder) evidence() (int, int) {
	embedder.mu.Lock()
	defer embedder.mu.Unlock()
	return embedder.items, embedder.maxInputBytes
}

func (embedder *chunkedMeanRecordingEmbedder) Embed(_ context.Context, input string) ([]float32, error) {
	embedder.record([]string{input})
	return testVectorWithFirstValue(1), nil
}

func (embedder *chunkedMeanRecordingEmbedder) EmbedBatch(_ context.Context, inputs []string) ([][]float32, error) {
	embedder.record(inputs)
	vectors := make([][]float32, len(inputs))
	for index := range vectors {
		vectors[index] = testVectorWithFirstValue(1)
	}
	return vectors, nil
}

func (embedder *chunkedMeanRecordingEmbedder) record(inputs []string) {
	embedder.mu.Lock()
	defer embedder.mu.Unlock()
	embedder.inputs = append(embedder.inputs, inputs...)
}

func (embedder *chunkedMeanRecordingEmbedder) takeInputs() []string {
	embedder.mu.Lock()
	defer embedder.mu.Unlock()
	inputs := append([]string(nil), embedder.inputs...)
	embedder.inputs = nil
	return inputs
}

func TestChunkedMeanProfilePreservesOneLogicalVectorAcrossLifecycle(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	tenantID := "chunked-mean-lifecycle"
	repoRoot := "/fixtures/chunked-mean-lifecycle"
	governance := NewGovernanceService(store, tenantID)
	if _, err := governance.ConfirmWorkspace(ctx, repoRoot); err != nil {
		t.Fatal(err)
	}
	continuityID := mustWorkspaceContinuity(t, store, tenantID, repoRoot)

	longContent := "Current deployment evidence: " + strings.Repeat("阶段-alpha-rollback-证据;", 3000)
	if len(longContent) <= 43406 {
		t.Fatalf("long fixture bytes=%d must exceed the rejected provider input", len(longContent))
	}
	created, err := governance.AddSource(ctx, repoRoot, GovernanceWriteRequest{
		OperationID: "chunked-mean-create",
		MemoryKey:   "deployment.current.evidence",
		Content:     longContent,
		SourceRef:   "fixture:chunked-mean-create",
	})
	if err != nil {
		t.Fatal(err)
	}

	embedder := &chunkedMeanRecordingEmbedder{}
	worker, err := NewProjectionWorker(store, embedder, ProjectionWorkerOptions{
		TenantID:           tenantID,
		Profile:            chunkedMeanRetrievalProfile(t),
		BatchSize:          1,
		EmbeddingBatchSize: 16,
		SnapshotPageSize:   16,
	})
	if err != nil {
		t.Fatal(err)
	}
	runProjectionUntilCurrent(t, worker)
	assertChunkedPhysicalInputs(t, embedder.takeInputs(), true)
	assertProfileVectorPresence(t, store, ChunkedMeanRetrievalProfileID, created.Memory.MemoryID, true)
	beforeHash, beforeVector := chunkedVectorDocument(t, store, tenantID, created.Memory.MemoryID)
	assertChunkedVectorCount(t, store, tenantID, 1)

	rebuild, err := worker.RebuildCurrent(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if rebuild.Scanned != 1 || rebuild.Projected != 1 || rebuild.Lag != 0 || rebuild.Status != "idle" {
		t.Fatalf("unexpected chunked snapshot rebuild: %#v", rebuild)
	}
	assertChunkedPhysicalInputs(t, embedder.takeInputs(), true)
	afterHash, afterVector := chunkedVectorDocument(t, store, tenantID, created.Memory.MemoryID)
	if afterHash != beforeHash || afterVector != beforeVector {
		t.Fatalf("snapshot rebuild changed deterministic logical vector: hash=%q/%q", beforeHash, afterHash)
	}
	assertChunkedVectorCount(t, store, tenantID, 1)

	revisedContent := "Revised deployment evidence: " + strings.Repeat("阶段-beta-three-maintainers-证据;", 2600)
	revised, err := governance.ReviseSource(ctx, repoRoot, created.Memory.MemoryID, GovernanceWriteRequest{
		OperationID: "chunked-mean-revise",
		MemoryKey:   "deployment.current.evidence",
		Content:     revisedContent,
		SourceRef:   "fixture:chunked-mean-revise",
	})
	if err != nil {
		t.Fatal(err)
	}
	runProjectionUntilCurrent(t, worker)
	assertChunkedPhysicalInputs(t, embedder.takeInputs(), true)
	assertProfileVectorPresence(t, store, ChunkedMeanRetrievalProfileID, created.Memory.MemoryID, false)
	assertProfileVectorPresence(t, store, ChunkedMeanRetrievalProfileID, revised.Memory.MemoryID, true)
	revisedHash, _ := chunkedVectorDocument(t, store, tenantID, revised.Memory.MemoryID)
	if revisedHash == beforeHash {
		t.Fatal("revision retained the superseded content hash")
	}
	assertChunkedVectorCount(t, store, tenantID, 1)

	longQuery := "Which deployment evidence is current? " + strings.Repeat("beta three maintainers continuity ", 1600)
	if len(longQuery) <= 43406 {
		t.Fatalf("long query bytes=%d must exceed the rejected provider input", len(longQuery))
	}
	coordinator, err := NewRetrievalCoordinator(store, embedder, chunkedMeanRetrievalProfile(t))
	if err != nil {
		t.Fatal(err)
	}
	result, err := coordinator.Retrieve(ctx, RetrievalRequest{
		OperationID:   "chunked-mean-long-query",
		TenantID:      tenantID,
		ContinuityIDs: []string{continuityID},
		Query:         longQuery,
		Limit:         1,
		Mode:          RetrievalVector,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Effective != RetrievalVector || result.Degraded || len(result.Memories) != 1 || result.Memories[0].ID != revised.Memory.MemoryID {
		t.Fatalf("long chunked query was not effective vector retrieval: %#v", result)
	}
	assertChunkedPhysicalInputs(t, embedder.takeInputs(), true)

	if _, err := governance.Forget(ctx, repoRoot, revised.Memory.MemoryID, "chunked-mean-forget"); err != nil {
		t.Fatal(err)
	}
	runProjectionUntilCurrent(t, worker)
	if inputs := embedder.takeInputs(); len(inputs) != 0 {
		t.Fatalf("forget/redact unnecessarily embedded %d inputs", len(inputs))
	}
	assertProfileVectorPresence(t, store, ChunkedMeanRetrievalProfileID, revised.Memory.MemoryID, false)
	assertChunkedVectorCount(t, store, tenantID, 0)
}

func TestLiveChunkedMeanProfileProjectsAndRetrievesOversizeInput(t *testing.T) {
	if os.Getenv("VERMORY_W28_LIVE_EMBEDDING") != "1" {
		t.Skip("VERMORY_W28_LIVE_EMBEDDING=1 is required")
	}
	apiKey := os.Getenv("VERMORY_LIVE_EMBEDDING_API_KEY")
	databaseURL := os.Getenv("VERMORY_LIVE_MIGRATION_DATABASE_URL")
	if apiKey == "" || databaseURL == "" {
		t.Skip("live embedding key and database URL are required")
	}

	ctx := context.Background()
	store, err := OpenStore(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if err := store.ResetForTest(ctx); err != nil {
		t.Fatal(err)
	}

	tenantID := "chunked-mean-live"
	repoRoot := "/fixtures/chunked-mean-live"
	governance := NewGovernanceService(store, tenantID)
	if _, err := governance.ConfirmWorkspace(ctx, repoRoot); err != nil {
		t.Fatal(err)
	}
	continuityID := mustWorkspaceContinuity(t, store, tenantID, repoRoot)
	content := "Live current deployment evidence: " + strings.Repeat("阶段-gamma-signed-artifact-证据;", 3000)
	created, err := governance.AddSource(ctx, repoRoot, GovernanceWriteRequest{
		OperationID: "chunked-mean-live-create",
		MemoryKey:   "deployment.live.current",
		Content:     content,
		SourceRef:   "fixture:chunked-mean-live",
	})
	if err != nil {
		t.Fatal(err)
	}

	spec, _ := SupportedRetrievalProfile(ChunkedMeanRetrievalProfileID)
	configured, err := memorybackend.NewOpenAIEmbedder(
		spec.BaseURL, apiKey, spec.Model, spec.Dimensions, &http.Client{Timeout: 2 * time.Minute},
	)
	if err != nil {
		t.Fatal(err)
	}
	batch, ok := any(configured).(BatchEmbedder)
	if !ok {
		t.Fatal("live SiliconFlow embedder does not support batch requests")
	}
	embedder := &chunkedMeanBoundedLiveEmbedder{delegate: configured, batchDelegate: batch}
	worker, err := NewProjectionWorker(store, embedder, ProjectionWorkerOptions{
		TenantID: tenantID, Profile: chunkedMeanRetrievalProfile(t),
		BatchSize: 1, EmbeddingBatchSize: 16,
	})
	if err != nil {
		t.Fatal(err)
	}
	runProjectionUntilCurrent(t, worker)
	assertProfileVectorPresence(t, store, ChunkedMeanRetrievalProfileID, created.Memory.MemoryID, true)
	assertChunkedVectorCount(t, store, tenantID, 1)

	coordinator, err := NewRetrievalCoordinator(store, embedder, chunkedMeanRetrievalProfile(t))
	if err != nil {
		t.Fatal(err)
	}
	query := "Which signed artifact evidence is current? " + strings.Repeat("gamma signed artifact continuity ", 1600)
	result, err := coordinator.Retrieve(ctx, RetrievalRequest{
		OperationID: "chunked-mean-live-query", TenantID: tenantID,
		ContinuityIDs: []string{continuityID}, Query: query, Limit: 1, Mode: RetrievalVector,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Effective != RetrievalVector || result.Degraded || len(result.Memories) != 1 || result.Memories[0].ID != created.Memory.MemoryID {
		t.Fatalf("live oversized retrieval was not effective vector: %#v", result)
	}
	items, maxBytes := embedder.evidence()
	if items < 4 || maxBytes <= 0 || maxBytes > 7500 {
		t.Fatalf("unexpected live physical-input evidence: items=%d max_bytes=%d", items, maxBytes)
	}
}

func chunkedMeanRetrievalProfile(t *testing.T) RetrievalProfile {
	t.Helper()
	spec, ok := SupportedRetrievalProfile(ChunkedMeanRetrievalProfileID)
	if !ok {
		t.Fatal("chunked mean retrieval profile is not registered")
	}
	return RetrievalProfile{
		ID: spec.ID, BaseURL: spec.BaseURL, Model: spec.Model,
		Dimensions: spec.Dimensions, ProjectionClass: spec.ProjectionClass,
		InputPolicy: spec.InputPolicy,
	}
}

func assertChunkedPhysicalInputs(t *testing.T, inputs []string, wantMultiple bool) {
	t.Helper()
	if len(inputs) == 0 || (wantMultiple && len(inputs) < 2) {
		t.Fatalf("physical embedding inputs=%d want multiple", len(inputs))
	}
	for index, input := range inputs {
		if !utf8.ValidString(input) || len(input) == 0 || len(input) > 7500 {
			t.Fatalf("physical input %d bytes=%d valid=%t", index, len(input), utf8.ValidString(input))
		}
	}
}

func assertChunkedVectorCount(t *testing.T, store *Store, tenantID string, want int) {
	t.Helper()
	var count int
	if err := store.pool.QueryRow(context.Background(), `
SELECT count(*)
FROM memory_vector_documents
WHERE tenant_id = $1 AND profile_id = $2`, tenantID, ChunkedMeanRetrievalProfileID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != want {
		t.Fatalf("chunked logical vector documents=%d want %d", count, want)
	}
}

func chunkedVectorDocument(t *testing.T, store *Store, tenantID, memoryID string) (string, string) {
	t.Helper()
	var hash, vector string
	if err := store.pool.QueryRow(context.Background(), `
SELECT content_sha256, embedding::text
FROM memory_vector_documents
WHERE tenant_id = $1 AND profile_id = $2 AND memory_id = $3::uuid`,
		tenantID, ChunkedMeanRetrievalProfileID, memoryID,
	).Scan(&hash, &vector); err != nil {
		t.Fatal(err)
	}
	return hash, vector
}
