package retrievalablation

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"vermory/internal/memorybackend"
	"vermory/internal/runtime"
)

func TestSeedCorpusMaterializesGovernedLifecycleAndVectorProjection(t *testing.T) {
	store := openRetrievalTestStore(t)
	backend := newRecordingBackend()
	corpus := seedTestCorpus()

	seeded, err := SeedCorpus(context.Background(), store, backend, corpus, "seed-run")
	if err != nil {
		t.Fatal(err)
	}
	if len(seeded.Scopes) != 3 || len(seeded.Records) != len(corpus.Records) {
		t.Fatalf("seeded mapping mismatch: scopes=%d records=%d", len(seeded.Scopes), len(seeded.Records))
	}
	workspace := seeded.Scopes["workspace-a"]
	current := seeded.Records["current"]
	old := seeded.Records["old"]
	proposed := seeded.Records["proposed"]
	deleted := seeded.Records["deleted"]

	assertRuntimeSearchContains(t, store, workspace.TenantID, workspace.ContinuityID, "current locked command", current.MemoryID)
	assertRuntimeSearchExcludes(t, store, workspace.TenantID, workspace.ContinuityID, "legacy command", old.MemoryID)
	assertRuntimeSearchExcludes(t, store, workspace.TenantID, workspace.ContinuityID, "unsafe proposal", proposed.MemoryID)
	assertRuntimeSearchExcludes(t, store, workspace.TenantID, workspace.ContinuityID, "deleted marker", deleted.MemoryID)

	if old.Lifecycle != "superseded" || proposed.Lifecycle != "proposed" || deleted.Lifecycle != "deleted" {
		t.Fatalf("seeded lifecycle mismatch: old=%#v proposed=%#v deleted=%#v", old, proposed, deleted)
	}
	for recordID, memoryID := range map[string]string{
		"superseded": old.MemoryID,
		"proposed":   proposed.MemoryID,
		"deleted":    deleted.MemoryID,
	} {
		if backend.has(memoryID) {
			t.Fatalf("%s memory remained in active vector projection", recordID)
		}
	}
}

func TestSeedW08CorpusMaterializesAllRecords(t *testing.T) {
	store := openRetrievalTestStore(t)
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	corpus, err := LoadCorpus(filepath.Join(root, "runtime", "cases", "W08-production-retrieval-ablation", "corpus.json"))
	if err != nil {
		t.Fatal(err)
	}
	backend := newRecordingBackend()
	seeded, err := SeedCorpus(context.Background(), store, backend, corpus, "w08-full-seed")
	if err != nil {
		t.Fatal(err)
	}
	if len(seeded.Records) != 60 || len(backend.records) != 48 {
		t.Fatalf("full seed counts mismatch: authority=%d vector=%d", len(seeded.Records), len(backend.records))
	}
	for _, record := range corpus.Records {
		seededRecord, exists := seeded.Records[record.ID]
		if !exists || seededRecord.MemoryID == "" || seededRecord.Lifecycle != record.Lifecycle {
			t.Fatalf("record %q mapping mismatch: %#v", record.ID, seededRecord)
		}
	}
}

func openRetrievalTestStore(t *testing.T) *runtime.Store {
	t.Helper()
	databaseURL := os.Getenv("VERMORY_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("VERMORY_TEST_DATABASE_URL is not set")
	}
	store, err := runtime.OpenStore(context.Background(), databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(store.Close)
	if err := store.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := store.ResetForTest(context.Background()); err != nil {
		t.Fatal(err)
	}
	return store
}

func seedTestCorpus() Corpus {
	return Corpus{
		Version: "1",
		Name:    "seed test",
		Scopes: []Scope{
			{ID: "workspace-a", TenantID: "seed-local", Line: "workspace", Anchor: "/fixtures/seed/workspace-a"},
			{ID: "conversation-a", TenantID: "seed-local", Line: "conversation", Anchor: "webchat:seed-thread"},
			{ID: "workspace-other", TenantID: "seed-other", Line: "workspace", Anchor: "/fixtures/seed/workspace-a"},
		},
		Records: []Record{
			{ID: "current", ScopeID: "workspace-a", MemoryKey: "release.command", Content: "Use the current locked command.", Lifecycle: "active", ProvenanceCase: "106-workspace-source-revision"},
			{ID: "old", ScopeID: "workspace-a", MemoryKey: "release.command", Content: "Use the legacy command.", Lifecycle: "superseded", ReplacementID: "current", ProvenanceCase: "106-workspace-source-revision"},
			{ID: "proposed", ScopeID: "workspace-a", Content: "Use an unsafe proposal.", Lifecycle: "proposed", ProvenanceCase: "107-workspace-source-conflict-candidate"},
			{ID: "deleted", ScopeID: "workspace-a", Content: "Deleted marker.", Lifecycle: "deleted", ProvenanceCase: "109-workspace-multifact-document-formation"},
			{ID: "conversation", ScopeID: "conversation-a", Content: "Rotate the recovery code.", Lifecycle: "active", ProvenanceCase: "203-conversation-device-troubleshooting"},
			{ID: "other", ScopeID: "workspace-other", Content: "Other tenant command.", Lifecycle: "active", ProvenanceCase: "101-workspace-parallel-repos"},
		},
		Queries: []Query{
			{ID: "current", ScopeID: "workspace-a", Text: "current command", Limit: 4, RelevantRecordIDs: []string{"current"}, ForbiddenRecordIDs: []string{"old", "proposed", "deleted", "other"}, Cohorts: []string{"semantic"}},
		},
	}
}

func assertRuntimeSearchContains(t *testing.T, store *runtime.Store, tenantID, continuityID, query, want string) {
	t.Helper()
	memories, err := store.SearchActiveMemory(context.Background(), tenantID, continuityID, query, 12)
	if err != nil {
		t.Fatal(err)
	}
	for _, memory := range memories {
		if memory.ID == want {
			return
		}
	}
	t.Fatalf("search %q did not contain %s: %#v", query, want, memories)
}

func assertRuntimeSearchExcludes(t *testing.T, store *runtime.Store, tenantID, continuityID, query, forbidden string) {
	t.Helper()
	memories, err := store.SearchActiveMemory(context.Background(), tenantID, continuityID, query, 12)
	if err != nil {
		t.Fatal(err)
	}
	for _, memory := range memories {
		if memory.ID == forbidden {
			t.Fatalf("search %q returned forbidden memory %s: %#v", query, forbidden, memories)
		}
	}
}

type recordingBackend struct {
	records map[string]memorybackend.Record
	archive map[string]memorybackend.Record
}

func newRecordingBackend() *recordingBackend {
	return &recordingBackend{records: map[string]memorybackend.Record{}, archive: map[string]memorybackend.Record{}}
}

func (b *recordingBackend) Name() string                 { return "recording" }
func (b *recordingBackend) Health(context.Context) error { return nil }
func (b *recordingBackend) Put(_ context.Context, record memorybackend.Record) error {
	b.records[record.ID] = record
	return nil
}
func (b *recordingBackend) Search(context.Context, memorybackend.Query) ([]memorybackend.Result, error) {
	return nil, nil
}
func (b *recordingBackend) Update(ctx context.Context, record memorybackend.Record) error {
	return b.Put(ctx, record)
}
func (b *recordingBackend) Delete(_ context.Context, _ memorybackend.Scope, recordID string) error {
	if record, exists := b.records[recordID]; exists {
		b.archive[recordID] = record
	}
	delete(b.records, recordID)
	return nil
}
func (b *recordingBackend) ResetScope(_ context.Context, scope memorybackend.Scope) error {
	for id, record := range b.records {
		if record.Scope == scope {
			delete(b.records, id)
		}
	}
	return nil
}
func (b *recordingBackend) RebuildScope(ctx context.Context, scope memorybackend.Scope, records []memorybackend.Record) error {
	if err := b.ResetScope(ctx, scope); err != nil {
		return err
	}
	for _, record := range records {
		if err := b.Put(ctx, record); err != nil {
			return err
		}
	}
	return nil
}
func (b *recordingBackend) Stats(context.Context) (memorybackend.Stats, error) {
	return memorybackend.Stats{RecordCount: len(b.records)}, nil
}
func (b *recordingBackend) status(recordID string) string { return b.records[recordID].Status }
func (b *recordingBackend) has(recordID string) bool {
	_, exists := b.records[recordID]
	return exists
}
