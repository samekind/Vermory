package retrievalablation

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestLoadCorpusRejectsUnknownFieldsAndTrailingJSON(t *testing.T) {
	tests := []struct {
		name    string
		payload string
	}{
		{
			name: "unknown field",
			payload: `{
  "version":"1",
  "name":"test",
  "scopes":[],
  "records":[],
  "queries":[],
  "extra":true
}`,
		},
		{
			name:    "trailing json",
			payload: `{"version":"1","name":"test","scopes":[],"records":[],"queries":[]} {}`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := writeCorpusTestFile(t, test.payload)
			if _, err := LoadCorpus(path); err == nil {
				t.Fatalf("LoadCorpus accepted %s", test.name)
			}
		})
	}
}

func TestValidateCorpusAcceptsCanonicalMinimum(t *testing.T) {
	root := t.TempDir()
	caseDir := filepath.Join(root, "casebook", "cases", "101-example")
	if err := os.MkdirAll(caseDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(caseDir, "source.md"), []byte("fixture"), 0o644); err != nil {
		t.Fatal(err)
	}
	corpus := Corpus{
		Version: "1",
		Name:    "minimum",
		Scopes: []Scope{
			{ID: "workspace-a", TenantID: "tenant-a", Line: "workspace", Anchor: "/fixtures/workspace-a"},
			{ID: "workspace-b", TenantID: "tenant-a", Line: "workspace", Anchor: "/fixtures/workspace-b"},
		},
		Records: []Record{
			{ID: "current", ScopeID: "workspace-a", Content: "Use pnpm exec release:verify --mode locked.", Lifecycle: "active", ProvenanceCase: "101-example"},
			{ID: "other", ScopeID: "workspace-b", Content: "Use npm run release:verify -- --legacy.", Lifecycle: "active", ProvenanceCase: "101-example"},
		},
		Queries: []Query{
			{ID: "release", ScopeID: "workspace-a", Text: "locked release command", Limit: 2, RelevantRecordIDs: []string{"current"}, ForbiddenRecordIDs: []string{"other"}, Cohorts: []string{"technical_command"}},
		},
	}
	if err := ValidateCorpus(root, corpus); err != nil {
		t.Fatalf("ValidateCorpus rejected valid corpus: %v", err)
	}
	first, err := CorpusSHA256(corpus)
	if err != nil {
		t.Fatal(err)
	}
	corpus.Scopes[0], corpus.Scopes[1] = corpus.Scopes[1], corpus.Scopes[0]
	corpus.Records[0], corpus.Records[1] = corpus.Records[1], corpus.Records[0]
	second, err := CorpusSHA256(corpus)
	if err != nil {
		t.Fatal(err)
	}
	if first != second || len(first) != 64 {
		t.Fatalf("canonical hash mismatch: first=%q second=%q", first, second)
	}
}

func TestValidateCorpusRejectsInvalidReferencesAndLifecycles(t *testing.T) {
	root := t.TempDir()
	caseDir := filepath.Join(root, "casebook", "cases", "101-example")
	if err := os.MkdirAll(caseDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(caseDir, "source.md"), []byte("fixture"), 0o644); err != nil {
		t.Fatal(err)
	}
	base := Corpus{
		Version: "1",
		Name:    "invalid",
		Scopes: []Scope{
			{ID: "workspace-a", TenantID: "tenant-a", Line: "workspace", Anchor: "/fixtures/workspace-a"},
		},
		Records: []Record{
			{ID: "current", ScopeID: "workspace-a", Content: "Current fact.", Lifecycle: "active", ProvenanceCase: "101-example"},
			{ID: "old", ScopeID: "workspace-a", Content: "Old fact.", Lifecycle: "superseded", ReplacementID: "current", ProvenanceCase: "101-example"},
		},
		Queries: []Query{
			{ID: "query", ScopeID: "workspace-a", Text: "current", Limit: 2, RelevantRecordIDs: []string{"current"}, ForbiddenRecordIDs: []string{"old"}, Cohorts: []string{"semantic_paraphrase"}},
		},
	}
	tests := []struct {
		name   string
		mutate func(*Corpus)
		want   string
	}{
		{name: "missing provenance", mutate: func(c *Corpus) { c.Records[0].ProvenanceCase = "missing-case" }, want: "provenance"},
		{name: "duplicate scope", mutate: func(c *Corpus) { c.Scopes = append(c.Scopes, c.Scopes[0]) }, want: "duplicate scope"},
		{name: "unknown scope", mutate: func(c *Corpus) { c.Records[0].ScopeID = "unknown" }, want: "unknown scope"},
		{name: "unsupported line", mutate: func(c *Corpus) { c.Scopes[0].Line = "global_defaults" }, want: "line"},
		{name: "unsupported lifecycle", mutate: func(c *Corpus) { c.Records[0].Lifecycle = "archived" }, want: "lifecycle"},
		{name: "invalid limit", mutate: func(c *Corpus) { c.Queries[0].Limit = 13 }, want: "limit"},
		{name: "overlapping expected ids", mutate: func(c *Corpus) { c.Queries[0].ForbiddenRecordIDs = []string{"current"} }, want: "both relevant and forbidden"},
		{name: "non-active relevant", mutate: func(c *Corpus) { c.Queries[0].RelevantRecordIDs = []string{"old"} }, want: "must be active"},
		{name: "unknown forbidden", mutate: func(c *Corpus) { c.Queries[0].ForbiddenRecordIDs = []string{"missing"} }, want: "unknown forbidden"},
		{
			name: "same-scope active distractor marked forbidden",
			mutate: func(c *Corpus) {
				c.Records = append(c.Records, Record{
					ID: "distractor", ScopeID: "workspace-a", Content: "Unrelated active fact.",
					Lifecycle: "active", ProvenanceCase: "101-example",
				})
				c.Queries[0].ForbiddenRecordIDs = []string{"old", "distractor"}
			},
			want: "same-scope active distractor",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			corpus := cloneCorpusForTest(base)
			test.mutate(&corpus)
			err := ValidateCorpus(root, corpus)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("ValidateCorpus error=%v, want substring %q", err, test.want)
			}
		})
	}
}

func TestW08CorpusCoverageAndLifecycleCounts(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "runtime", "cases", "W08-production-retrieval-ablation", "corpus.json")
	corpus, err := LoadCorpus(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateCorpus(root, corpus); err != nil {
		t.Fatal(err)
	}
	if len(corpus.Scopes) < 8 || len(corpus.Queries) != 24 {
		t.Fatalf("unexpected corpus size: scopes=%d queries=%d", len(corpus.Scopes), len(corpus.Queries))
	}
	counts := map[string]int{}
	tenants := map[string]struct{}{}
	cohorts := map[string]struct{}{}
	for _, scope := range corpus.Scopes {
		tenants[scope.TenantID] = struct{}{}
	}
	for _, record := range corpus.Records {
		counts[record.Lifecycle]++
	}
	for _, query := range corpus.Queries {
		for _, cohort := range query.Cohorts {
			cohorts[cohort] = struct{}{}
		}
	}
	wantCounts := map[string]int{"active": 48, "proposed": 4, "superseded": 4, "deleted": 4}
	for lifecycle, want := range wantCounts {
		if counts[lifecycle] != want {
			t.Fatalf("lifecycle %s count=%d, want %d", lifecycle, counts[lifecycle], want)
		}
	}
	if len(tenants) < 4 {
		t.Fatalf("tenant count=%d, want at least 4", len(tenants))
	}
	for _, cohort := range []string{
		"exact_identifier", "path", "feature_flag", "error_code", "model_name",
		"chinese_semantic", "semantic_paraphrase", "mixed_language", "date",
		"duration", "numeric_constraint", "multi_fact", "continuity_isolation",
	} {
		if _, exists := cohorts[cohort]; !exists {
			t.Fatalf("missing required cohort %q; present=%v", cohort, sortedKeys(cohorts))
		}
	}
	digest, err := CorpusSHA256(corpus)
	if err != nil || len(digest) != 64 {
		t.Fatalf("corpus digest=%q error=%v", digest, err)
	}
}

func TestW10IndependentCorpusCoverageAndLifecycleCounts(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "runtime", "cases", "W10-independent-retrieval-batch", "corpus.json")
	corpus, err := LoadCorpus(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateCorpus(root, corpus); err != nil {
		t.Fatal(err)
	}
	if len(corpus.Scopes) != 6 || len(corpus.Records) != 39 || len(corpus.Queries) != 18 {
		t.Fatalf("unexpected corpus size: scopes=%d records=%d queries=%d", len(corpus.Scopes), len(corpus.Records), len(corpus.Queries))
	}
	counts := map[string]int{}
	tenants := map[string]struct{}{}
	cohorts := map[string]struct{}{}
	for _, scope := range corpus.Scopes {
		tenants[scope.TenantID] = struct{}{}
	}
	for _, record := range corpus.Records {
		counts[record.Lifecycle]++
	}
	for _, query := range corpus.Queries {
		for _, cohort := range query.Cohorts {
			cohorts[cohort] = struct{}{}
		}
	}
	wantCounts := map[string]int{"active": 30, "proposed": 3, "superseded": 3, "deleted": 3}
	for lifecycle, want := range wantCounts {
		if counts[lifecycle] != want {
			t.Fatalf("lifecycle %s count=%d, want %d", lifecycle, counts[lifecycle], want)
		}
	}
	if len(tenants) != 4 {
		t.Fatalf("tenant count=%d, want 4", len(tenants))
	}
	for _, cohort := range []string{
		"exact_identifier", "path", "feature_flag", "error_code", "chinese_semantic",
		"semantic_paraphrase", "mixed_language", "date", "duration", "numeric_constraint",
		"multi_fact", "continuity_isolation", "technical_command",
	} {
		if _, exists := cohorts[cohort]; !exists {
			t.Fatalf("missing required cohort %q; present=%v", cohort, sortedKeys(cohorts))
		}
	}
	digest, err := CorpusSHA256(corpus)
	if err != nil || len(digest) != 64 {
		t.Fatalf("corpus digest=%q error=%v", digest, err)
	}
}

func writeCorpusTestFile(t *testing.T, payload string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "corpus.json")
	if err := os.WriteFile(path, []byte(payload), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func cloneCorpusForTest(corpus Corpus) Corpus {
	clone := corpus
	clone.Scopes = append([]Scope(nil), corpus.Scopes...)
	clone.Records = append([]Record(nil), corpus.Records...)
	clone.Queries = append([]Query(nil), corpus.Queries...)
	for index := range clone.Queries {
		clone.Queries[index].RelevantRecordIDs = append([]string(nil), clone.Queries[index].RelevantRecordIDs...)
		clone.Queries[index].ForbiddenRecordIDs = append([]string(nil), clone.Queries[index].ForbiddenRecordIDs...)
		clone.Queries[index].TaskExcludedRecordIDs = append([]string(nil), clone.Queries[index].TaskExcludedRecordIDs...)
		clone.Queries[index].Cohorts = append([]string(nil), clone.Queries[index].Cohorts...)
	}
	return clone
}

func sortedKeys(values map[string]struct{}) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
