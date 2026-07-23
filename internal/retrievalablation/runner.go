package retrievalablation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"vermory/internal/memorybackend"
	"vermory/internal/runtime"
)

type Options struct {
	RunID                  string
	Corpus                 Corpus
	DatabaseURL            string
	CorpusPath             string
	RepositoryRoot         string
	EmbeddingBaseURL       string
	EmbeddingAPIKey        string
	EmbeddingModel         string
	EmbeddingDimensions    int
	ImplementationRevision string
	SchemaVersion          int64
}

func Run(ctx context.Context, options Options) (Report, error) {
	if err := validateRunOptions(options); err != nil {
		return Report{}, err
	}
	corpus, err := LoadCorpus(options.CorpusPath)
	if err != nil {
		return Report{}, err
	}
	repositoryRoot, err := findRepositoryRoot(options.CorpusPath)
	if err != nil {
		return Report{}, err
	}
	if err := ValidateCorpus(repositoryRoot, corpus); err != nil {
		return Report{}, err
	}
	store, err := runtime.OpenStore(ctx, options.DatabaseURL)
	if err != nil {
		return Report{}, err
	}
	defer store.Close()
	if err := store.Migrate(ctx); err != nil {
		return Report{}, err
	}
	schemaVersion, err := store.SchemaVersion(ctx)
	if err != nil {
		return Report{}, err
	}
	backend, cleanup, err := memorybackend.OpenBackend(ctx, memorybackend.OpenConfig{
		Name: "native", DatabaseURL: options.DatabaseURL,
		EmbeddingBaseURL: options.EmbeddingBaseURL, EmbeddingAPIKey: options.EmbeddingAPIKey,
		EmbeddingModel: options.EmbeddingModel, Dimensions: options.EmbeddingDimensions,
		HTTPClient: &http.Client{Timeout: 2 * time.Minute},
	})
	if err != nil {
		return Report{}, err
	}
	defer cleanup()
	options.Corpus = corpus
	options.RepositoryRoot = repositoryRoot
	options.SchemaVersion = schemaVersion
	return RunWithDependencies(ctx, options, store, backend)
}

func validateRunOptions(options Options) error {
	if strings.TrimSpace(options.DatabaseURL) == "" {
		return fmt.Errorf("database URL is required")
	}
	if strings.TrimSpace(options.CorpusPath) == "" {
		return fmt.Errorf("corpus path is required")
	}
	if strings.TrimSpace(options.RunID) == "" {
		return fmt.Errorf("run_id is required")
	}
	if strings.TrimSpace(options.EmbeddingBaseURL) == "" {
		return fmt.Errorf("embedding base URL is required")
	}
	parsedBaseURL, err := url.Parse(options.EmbeddingBaseURL)
	if err != nil || (parsedBaseURL.Scheme != "http" && parsedBaseURL.Scheme != "https") || parsedBaseURL.Host == "" {
		return fmt.Errorf("embedding base URL must be an absolute HTTP(S) URL")
	}
	if parsedBaseURL.User != nil || parsedBaseURL.RawQuery != "" || parsedBaseURL.Fragment != "" {
		return fmt.Errorf("embedding base URL must not include credentials, query, or fragment")
	}
	if strings.TrimSpace(options.EmbeddingAPIKey) == "" {
		return fmt.Errorf("embedding API key is required")
	}
	if strings.TrimSpace(options.EmbeddingModel) == "" {
		return fmt.Errorf("embedding model is required")
	}
	if options.EmbeddingDimensions <= 0 || options.EmbeddingDimensions > 4096 {
		return fmt.Errorf("embedding dimensions must be between 1 and 4096")
	}
	if strings.TrimSpace(options.ImplementationRevision) == "" {
		return fmt.Errorf("implementation revision is required")
	}
	return nil
}

func RunWithDependencies(
	ctx context.Context,
	options Options,
	store *runtime.Store,
	backend memorybackend.Backend,
) (Report, error) {
	started := time.Now().UTC()
	if strings.TrimSpace(options.RunID) == "" {
		return Report{}, fmt.Errorf("run_id is required")
	}
	if len(options.Corpus.Queries) == 0 {
		return Report{}, fmt.Errorf("retrieval corpus requires queries")
	}
	corpusHash, err := CorpusSHA256(options.Corpus)
	if err != nil {
		return Report{}, err
	}
	seeded, err := SeedCorpus(ctx, store, backend, options.Corpus, options.RunID)
	if err != nil {
		return Report{}, err
	}
	conditions, failures, err := executeConditions(ctx, store, backend, options.Corpus, seeded)
	if err != nil {
		return Report{}, err
	}
	rebuildEquivalent, err := rebuildAndCompare(ctx, store, backend, options.Corpus, seeded, conditions)
	if err != nil {
		return Report{}, err
	}
	authorityFingerprint, err := authorityFingerprint(ctx, store, seeded)
	if err != nil {
		return Report{}, err
	}
	implementationRevision := strings.TrimSpace(options.ImplementationRevision)
	if implementationRevision == "" {
		implementationRevision = "unknown"
	}
	backendStats, err := backend.Stats(ctx)
	if err != nil {
		return Report{}, fmt.Errorf("read vector backend stats: %w", err)
	}
	report := Report{
		RunID:                  options.RunID,
		CorpusSHA256:           corpusHash,
		ImplementationRevision: implementationRevision,
		EngineVersion:          "rrf-v1",
		SchemaVersion:          options.SchemaVersion,
		AuthorityFingerprint:   authorityFingerprint,
		Embedding: EmbeddingProfile{
			BaseURL:    strings.TrimSpace(options.EmbeddingBaseURL),
			Model:      strings.TrimSpace(options.EmbeddingModel),
			Dimensions: options.EmbeddingDimensions,
		},
		EmbeddingRequestCount:       embeddingRequestCount(backendStats),
		StartedAt:                   started,
		Conditions:                  conditions,
		ProjectionRebuildEquivalent: rebuildEquivalent,
		Failures:                    failures,
	}
	for _, condition := range conditions {
		report.HardGates.ForbiddenCount += condition.Metrics.ForbiddenCount
		report.HardGates.IneligibleCount += condition.Metrics.IneligibleCount
	}
	// Forbidden results are a constitutional failure just like ineligible
	// rows. Keep the aggregate counters for diagnosis, but never qualify a
	// run that delivered a declared forbidden memory.
	report.HardGates.Pass = report.HardGates.ForbiddenCount == 0 &&
		report.HardGates.IneligibleCount == 0 && rebuildEquivalent
	if report.HardGates.Pass {
		report.QualificationStatus = "measured"
	} else {
		report.QualificationStatus = "hard_gate_failed"
	}
	report.NonClaims = []string{
		"not a sealed result",
		"not a production default switch",
		"not a source-authority ranking result",
		"not a scale qualification",
	}
	report.Duration = time.Since(started)
	report.RequestFingerprint = ReportRequestFingerprint(report)
	return report, nil
}

func executeConditions(
	ctx context.Context,
	store *runtime.Store,
	backend memorybackend.Backend,
	corpus Corpus,
	seeded SeededCorpus,
) ([]ConditionReport, []RunFailure, error) {
	lexicalReport := ConditionReport{Name: ConditionLexical}
	vectorReport := ConditionReport{Name: ConditionVector}
	hybridReport := ConditionReport{Name: ConditionHybrid}
	failures := make([]RunFailure, 0)
	memoryToRecord := make(map[string]string, len(seeded.Records))
	for recordID, seededRecord := range seeded.Records {
		memoryToRecord[seededRecord.MemoryID] = recordID
	}

	for _, query := range corpus.Queries {
		scope, exists := seeded.Scopes[query.ScopeID]
		if !exists {
			return nil, nil, fmt.Errorf("query %q references unseeded scope %q", query.ID, query.ScopeID)
		}
		active, err := activeMemorySet(ctx, store, scope)
		if err != nil {
			return nil, nil, fmt.Errorf("load active authority for query %q: %w", query.ID, err)
		}

		lexicalStarted := time.Now()
		lexicalMemories, err := store.SearchActiveMemory(ctx, scope.TenantID, scope.ContinuityID, query.Text, 12)
		lexicalDuration := time.Since(lexicalStarted)
		if err != nil {
			return nil, nil, fmt.Errorf("lexical query %q: %w", query.ID, err)
		}
		lexicalResults := make([]RankedResult, 0, len(lexicalMemories))
		for index, memory := range lexicalMemories {
			lexicalResults = append(lexicalResults, RankedResult{
				MemoryID: memory.ID, RecordID: memoryToRecord[memory.ID], Content: memory.Content,
				LexicalRank: index + 1, Eligible: active[memory.ID],
			})
		}
		lexicalDelivered := truncateRanked(lexicalResults, query.Limit)
		lexicalQuery := QueryReport{
			QueryID: query.ID, Cohorts: append([]string(nil), query.Cohorts...), Duration: lexicalDuration,
			Results: lexicalDelivered, Metrics: ScoreQuery(query, lexicalDelivered),
		}
		lexicalReport.Queries = append(lexicalReport.Queries, lexicalQuery)

		vectorStarted := time.Now()
		vectorBackendResults, vectorErr := backend.Search(ctx, memorybackend.Query{
			Scope: scope.BackendScope, Text: query.Text, Limit: vectorCandidateLimit(query.Limit),
		})
		vectorDuration := time.Since(vectorStarted)
		if vectorErr != nil {
			message := vectorErr.Error()
			vectorReport.Queries = append(vectorReport.Queries, QueryReport{
				QueryID: query.ID, Cohorts: append([]string(nil), query.Cohorts...), Duration: vectorDuration, Error: message,
			})
			fallback := FallbackToLexical(lexicalResults, query.Limit)
			hybridReport.Queries = append(hybridReport.Queries, QueryReport{
				QueryID: query.ID, Cohorts: append([]string(nil), query.Cohorts...), Duration: lexicalDuration + vectorDuration,
				Results: fallback, Metrics: ScoreQuery(query, fallback), DegradedToLexical: true,
			})
			failures = append(failures, RunFailure{Condition: ConditionVector, QueryID: query.ID, Error: message})
			continue
		}

		vectorEligible := make([]RankedResult, 0, len(vectorBackendResults))
		vectorRejected := make([]RankedResult, 0)
		for index, backendResult := range vectorBackendResults {
			recordID := backendResult.Record.Metadata["record_id"]
			if recordID == "" {
				recordID = memoryToRecord[backendResult.Record.ID]
			}
			result := RankedResult{
				MemoryID: backendResult.Record.ID, RecordID: recordID, Content: backendResult.Record.Content,
				Score: backendResult.Score, VectorRank: index + 1, Eligible: active[backendResult.Record.ID],
			}
			if result.Eligible {
				vectorEligible = append(vectorEligible, result)
			} else {
				vectorRejected = append(vectorRejected, result)
			}
		}
		vectorDelivered := truncateRanked(vectorEligible, query.Limit)
		vectorMetrics := ScoreQuery(query, vectorDelivered)
		vectorMetrics.IneligibleCount += len(vectorRejected)
		vectorReport.Queries = append(vectorReport.Queries, QueryReport{
			QueryID: query.ID, Cohorts: append([]string(nil), query.Cohorts...), Duration: vectorDuration,
			Results: vectorDelivered, RejectedResults: vectorRejected, Metrics: vectorMetrics,
		})

		fusionStarted := time.Now()
		hybridResults := FuseRRF(query.Text, query.Limit, lexicalResults, vectorEligible)
		hybridDuration := lexicalDuration + vectorDuration + time.Since(fusionStarted)
		hybridReport.Queries = append(hybridReport.Queries, QueryReport{
			QueryID: query.ID, Cohorts: append([]string(nil), query.Cohorts...), Duration: hybridDuration,
			Results: hybridResults, Metrics: ScoreQuery(query, hybridResults),
		})
	}

	for _, report := range []*ConditionReport{&lexicalReport, &vectorReport, &hybridReport} {
		report.Metrics = AggregateMetrics(report.Queries)
		report.Cohorts = AggregateCohorts(report.Queries)
	}
	return []ConditionReport{lexicalReport, vectorReport, hybridReport}, failures, nil
}

func rebuildAndCompare(
	ctx context.Context,
	store *runtime.Store,
	backend memorybackend.Backend,
	corpus Corpus,
	seeded SeededCorpus,
	before []ConditionReport,
) (bool, error) {
	scopeIDs := make([]string, 0, len(seeded.Scopes))
	for scopeID := range seeded.Scopes {
		scopeIDs = append(scopeIDs, scopeID)
	}
	sort.Strings(scopeIDs)
	for _, scopeID := range scopeIDs {
		records := append([]memorybackend.Record(nil), seeded.BackendRecords[scopeID]...)
		sort.Slice(records, func(i, j int) bool { return records[i].ID < records[j].ID })
		if err := backend.RebuildScope(ctx, seeded.Scopes[scopeID].BackendScope, records); err != nil {
			return false, fmt.Errorf("rebuild vector scope %q: %w", scopeID, err)
		}
	}
	after, _, err := executeConditions(ctx, store, backend, corpus, seeded)
	if err != nil {
		return false, err
	}
	return comparableConditionResults(before) == comparableConditionResults(after), nil
}

func comparableConditionResults(conditions []ConditionReport) string {
	type queryResult struct {
		Condition string
		QueryID   string
		Results   []string
		Rejected  []string
		Degraded  bool
		Error     string
	}
	comparable := make([]queryResult, 0)
	for _, condition := range conditions {
		if condition.Name == ConditionLexical {
			continue
		}
		for _, query := range condition.Queries {
			comparable = append(comparable, queryResult{
				Condition: condition.Name,
				QueryID:   query.QueryID,
				Results:   recordIDs(query.Results),
				Rejected:  recordIDs(query.RejectedResults),
				Degraded:  query.DegradedToLexical,
				Error:     query.Error,
			})
		}
	}
	return fmt.Sprintf("%#v", comparable)
}

func activeMemorySet(ctx context.Context, store *runtime.Store, scope SeededScope) (map[string]bool, error) {
	memories, err := store.ListGovernedMemories(ctx, scope.TenantID, scope.ContinuityID)
	if err != nil {
		return nil, err
	}
	active := make(map[string]bool, len(memories))
	for _, memory := range memories {
		if memory.LifecycleStatus == "active" {
			active[memory.ID] = true
		}
	}
	return active, nil
}

func vectorCandidateLimit(limit int) int {
	candidates := limit * 4
	if candidates < 20 {
		candidates = 20
	}
	if candidates > 100 {
		candidates = 100
	}
	return candidates
}

func truncateRanked(results []RankedResult, limit int) []RankedResult {
	if limit <= 0 || len(results) == 0 {
		return nil
	}
	if limit > len(results) {
		limit = len(results)
	}
	return append([]RankedResult(nil), results[:limit]...)
}

func recordIDs(results []RankedResult) []string {
	ids := make([]string, len(results))
	for index, result := range results {
		ids[index] = result.RecordID
	}
	return ids
}

func findRepositoryRoot(corpusPath string) (string, error) {
	absolutePath, err := filepath.Abs(corpusPath)
	if err != nil {
		return "", fmt.Errorf("resolve corpus path: %w", err)
	}
	current := filepath.Dir(absolutePath)
	for {
		casebook := filepath.Join(current, "casebook", "cases")
		if info, err := os.Stat(casebook); err == nil && info.IsDir() {
			return current, nil
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}
	return "", fmt.Errorf("corpus path is not inside a Vermory repository with casebook/cases")
}

func authorityFingerprint(ctx context.Context, store *runtime.Store, seeded SeededCorpus) (string, error) {
	scopeIDs := make([]string, 0, len(seeded.Scopes))
	for scopeID := range seeded.Scopes {
		scopeIDs = append(scopeIDs, scopeID)
	}
	sort.Strings(scopeIDs)
	lines := make([]string, 0, len(seeded.Records))
	for _, scopeID := range scopeIDs {
		scope := seeded.Scopes[scopeID]
		memories, err := store.ListGovernedMemories(ctx, scope.TenantID, scope.ContinuityID)
		if err != nil {
			return "", fmt.Errorf("fingerprint authority scope %q: %w", scopeID, err)
		}
		for _, memory := range memories {
			lines = append(lines, strings.Join([]string{
				scope.TenantID, scope.ContinuityID, memory.ID, memory.MemoryKey,
				memory.LifecycleStatus, memory.Content, memory.SupersedesMemoryID,
			}, "\x1f"))
		}
	}
	sort.Strings(lines)
	digest := sha256.Sum256([]byte(strings.Join(lines, "\n")))
	return hex.EncodeToString(digest[:]), nil
}

func embeddingRequestCount(stats memorybackend.Stats) int64 {
	value, exists := stats.Extra["embedding_requests"]
	if !exists {
		return 0
	}
	switch count := value.(type) {
	case int64:
		return count
	case int:
		return int64(count)
	case float64:
		return int64(count)
	default:
		return 0
	}
}
