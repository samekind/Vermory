package retrievalablation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"vermory/internal/memorybackend"
	"vermory/internal/runtime"
)

type ProfileComparisonOptions struct {
	RunID                  string
	Corpus                 Corpus
	ImplementationRevision string
	SchemaVersion          int64
}

type ProfileComparison struct {
	RunID                  string                     `json:"run_id"`
	RequestFingerprint     string                     `json:"request_fingerprint"`
	CorpusSHA256           string                     `json:"corpus_sha256"`
	ImplementationRevision string                     `json:"implementation_revision"`
	SchemaVersion          int64                      `json:"schema_version"`
	AuthorityFingerprint   string                     `json:"authority_fingerprint"`
	StartedAt              time.Time                  `json:"started_at"`
	Duration               time.Duration              `json:"duration"`
	Profiles               []ProfileComparisonReport  `json:"profiles"`
	Policy                 ProfilePromotionPolicy     `json:"promotion_policy"`
	Decision               ProfilePromotionDecision   `json:"promotion_decision"`
	HardGates              ProfileComparisonHardGates `json:"hard_gates"`
	QualificationStatus    string                     `json:"qualification_status"`
	NonClaims              []string                   `json:"non_claims"`
}

type ProfileComparisonReport struct {
	ProfileID                   string               `json:"profile_id"`
	Model                       string               `json:"model"`
	Dimensions                  int                  `json:"dimensions"`
	LifecycleStatus             string               `json:"lifecycle_status"`
	EmbeddingRequestCount       int64                `json:"embedding_request_count"`
	ProjectionBuildDuration     time.Duration        `json:"projection_build_duration"`
	ProjectionVectorCount       int64                `json:"projection_vector_count"`
	ProjectionLag               int64                `json:"projection_lag"`
	Queries                     []QueryReport        `json:"queries"`
	Metrics                     Aggregate            `json:"metrics"`
	Cohorts                     map[string]Aggregate `json:"cohorts"`
	HardGates                   HardGateReport       `json:"hard_gates"`
	ProjectionRebuildEquivalent bool                 `json:"projection_rebuild_equivalent"`
}

type ProfileComparisonHardGates struct {
	Pass                bool  `json:"pass"`
	ProfileFailureCount int   `json:"profile_failure_count"`
	ForbiddenCount      int   `json:"forbidden_count"`
	IneligibleCount     int   `json:"ineligible_count"`
	DegradedCount       int   `json:"degraded_count"`
	ProjectionLag       int64 `json:"projection_lag"`
}

type ProfilePromotionPolicy struct {
	MaxRecallRegression    float64       `json:"max_recall_regression"`
	MaxMRRRegression       float64       `json:"max_mrr_regression"`
	MaxNDCGRegression      float64       `json:"max_ndcg_regression"`
	MaxP95Ratio            float64       `json:"max_p95_ratio"`
	MaxP95AbsoluteIncrease time.Duration `json:"max_p95_absolute_increase"`
	MinHitAt1Gain          float64       `json:"min_hit_at_1_gain"`
	MinMRRGain             float64       `json:"min_mrr_gain"`
	MinP95ImprovementRatio float64       `json:"min_p95_improvement_ratio"`
}

type ProfilePromotionDecision struct {
	IncumbentProfileID string        `json:"incumbent_profile_id"`
	CandidateProfileID string        `json:"candidate_profile_id"`
	Promote            bool          `json:"promote"`
	Status             string        `json:"status"`
	ReasonCodes        []string      `json:"reason_codes"`
	HitAt1Delta        float64       `json:"hit_at_1_delta"`
	RecallDelta        float64       `json:"recall_delta"`
	MRRDelta           float64       `json:"mrr_delta"`
	NDCGDelta          float64       `json:"ndcg_delta"`
	P95Delta           time.Duration `json:"p95_delta"`
}

func DefaultProfilePromotionPolicy() ProfilePromotionPolicy {
	return ProfilePromotionPolicy{
		MaxRecallRegression:    0,
		MaxMRRRegression:       0.02,
		MaxNDCGRegression:      0.02,
		MaxP95Ratio:            1.25,
		MaxP95AbsoluteIncrease: 75 * time.Millisecond,
		MinHitAt1Gain:          0.02,
		MinMRRGain:             0.02,
		MinP95ImprovementRatio: 0.15,
	}
}

func EvaluateProfilePromotion(
	policy ProfilePromotionPolicy,
	incumbent ProfileComparisonReport,
	candidate ProfileComparisonReport,
) ProfilePromotionDecision {
	decision := ProfilePromotionDecision{
		IncumbentProfileID: incumbent.ProfileID,
		CandidateProfileID: candidate.ProfileID,
		Status:             "keep_candidate",
		HitAt1Delta:        candidate.Metrics.HitAt1 - incumbent.Metrics.HitAt1,
		RecallDelta:        candidate.Metrics.RecallAtK - incumbent.Metrics.RecallAtK,
		MRRDelta:           candidate.Metrics.MRR - incumbent.Metrics.MRR,
		NDCGDelta:          candidate.Metrics.NDCGAtK - incumbent.Metrics.NDCGAtK,
		P95Delta:           candidate.Metrics.SearchP95 - incumbent.Metrics.SearchP95,
	}
	if !candidate.HardGates.Pass || !candidate.ProjectionRebuildEquivalent {
		decision.ReasonCodes = append(decision.ReasonCodes, "hard_gate_failed")
	}
	if incumbent.Metrics.QueryCount == 0 || candidate.Metrics.QueryCount != incumbent.Metrics.QueryCount {
		decision.ReasonCodes = append(decision.ReasonCodes, "query_coverage_mismatch")
	}
	if decision.RecallDelta < -policy.MaxRecallRegression ||
		decision.MRRDelta < -policy.MaxMRRRegression ||
		decision.NDCGDelta < -policy.MaxNDCGRegression {
		decision.ReasonCodes = append(decision.ReasonCodes, "quality_regression")
	}
	if profileLatencyRegressed(policy, incumbent.Metrics.SearchP95, candidate.Metrics.SearchP95) {
		decision.ReasonCodes = append(decision.ReasonCodes, "latency_regression")
	}
	if len(decision.ReasonCodes) > 0 {
		return decision
	}
	qualityBenefit := decision.HitAt1Delta >= policy.MinHitAt1Gain || decision.MRRDelta >= policy.MinMRRGain
	latencyBenefit := incumbent.Metrics.SearchP95 > 0 &&
		float64(incumbent.Metrics.SearchP95-candidate.Metrics.SearchP95)/float64(incumbent.Metrics.SearchP95) >= policy.MinP95ImprovementRatio
	if !qualityBenefit && !latencyBenefit {
		decision.ReasonCodes = []string{"no_clear_benefit"}
		return decision
	}
	decision.Promote = true
	decision.Status = "promote"
	decision.ReasonCodes = []string{"promotion_thresholds_met"}
	return decision
}

func profileLatencyRegressed(policy ProfilePromotionPolicy, incumbent, candidate time.Duration) bool {
	if incumbent <= 0 {
		return candidate > 0
	}
	ratioLimit := time.Duration(float64(incumbent) * policy.MaxP95Ratio)
	absoluteLimit := incumbent + policy.MaxP95AbsoluteIncrease
	limit := ratioLimit
	if absoluteLimit > limit {
		limit = absoluteLimit
	}
	return candidate > limit
}

func RunProfileComparisonWithDependencies(
	ctx context.Context,
	options ProfileComparisonOptions,
	store *runtime.Store,
	embedders map[string]runtime.Embedder,
) (ProfileComparison, error) {
	started := time.Now().UTC()
	if store == nil {
		return ProfileComparison{}, fmt.Errorf("runtime store is required")
	}
	if strings.TrimSpace(options.RunID) == "" {
		return ProfileComparison{}, fmt.Errorf("run_id is required")
	}
	if strings.TrimSpace(options.ImplementationRevision) == "" {
		return ProfileComparison{}, fmt.Errorf("implementation revision is required")
	}
	if len(options.Corpus.Queries) == 0 {
		return ProfileComparison{}, fmt.Errorf("retrieval corpus requires queries")
	}
	corpusHash, err := CorpusSHA256(options.Corpus)
	if err != nil {
		return ProfileComparison{}, err
	}
	seeded, err := SeedCorpus(ctx, store, authoritySeedBackend{}, options.Corpus, options.RunID)
	if err != nil {
		return ProfileComparison{}, err
	}
	authorityHash, err := authorityFingerprint(ctx, store, seeded)
	if err != nil {
		return ProfileComparison{}, err
	}

	profileIDs := []string{runtime.ProductionRetrievalProfileID, runtime.MigrationRetrievalProfileID}
	report := ProfileComparison{
		RunID: options.RunID, CorpusSHA256: corpusHash,
		ImplementationRevision: options.ImplementationRevision,
		SchemaVersion:          options.SchemaVersion, AuthorityFingerprint: authorityHash,
		StartedAt: started, Policy: DefaultProfilePromotionPolicy(),
		NonClaims: []string{
			"not a model ranking",
			"not a production default switch without the recorded decision",
			"not a sealed result",
			"not a scale qualification",
		},
	}
	for _, profileID := range profileIDs {
		spec, _ := runtime.SupportedRetrievalProfile(profileID)
		embedder := embedders[profileID]
		if embedder == nil {
			return ProfileComparison{}, fmt.Errorf("embedder for profile %q is required", profileID)
		}
		counted := &profileCountingEmbedder{Embedder: embedder}
		profile, err := runOneProfileComparison(ctx, options.RunID, store, options.Corpus, seeded, spec, counted)
		if err != nil {
			return ProfileComparison{}, err
		}
		report.Profiles = append(report.Profiles, profile)
	}
	report.Decision = EvaluateProfilePromotion(report.Policy, report.Profiles[0], report.Profiles[1])
	for _, profile := range report.Profiles {
		if !profile.HardGates.Pass {
			report.HardGates.ProfileFailureCount++
		}
		report.HardGates.ForbiddenCount += profile.HardGates.ForbiddenCount
		report.HardGates.IneligibleCount += profile.HardGates.IneligibleCount
		report.HardGates.DegradedCount += profile.HardGates.DegradedCount
		report.HardGates.ProjectionLag += profile.ProjectionLag
	}
	report.HardGates.Pass = report.HardGates.ProfileFailureCount == 0 &&
		report.HardGates.ForbiddenCount == 0 && report.HardGates.IneligibleCount == 0 &&
		report.HardGates.DegradedCount == 0 && report.HardGates.ProjectionLag == 0
	if report.HardGates.Pass {
		report.QualificationStatus = "measured"
	} else {
		report.QualificationStatus = "hard_gate_failed"
	}
	report.Duration = time.Since(started)
	report.RequestFingerprint = profileComparisonFingerprint(report)
	return report, nil
}

func runOneProfileComparison(
	ctx context.Context,
	runID string,
	store *runtime.Store,
	corpus Corpus,
	seeded SeededCorpus,
	spec runtime.RetrievalProfileSpec,
	embedder *profileCountingEmbedder,
) (ProfileComparisonReport, error) {
	profile := runtime.RetrievalProfile{
		ID: spec.ID, BaseURL: spec.BaseURL, Model: spec.Model,
		Dimensions: spec.Dimensions, ProjectionClass: spec.ProjectionClass,
	}
	buildDuration, vectorCount, lag, err := buildProfileProjection(ctx, store, seeded, profile, embedder)
	if err != nil {
		return ProfileComparisonReport{}, fmt.Errorf("build profile %q: %w", spec.ID, err)
	}
	coordinator, err := runtime.NewRetrievalCoordinator(store, embedder, profile)
	if err != nil {
		return ProfileComparisonReport{}, err
	}
	queries, err := executeProfileQueries(ctx, runID+":"+spec.ID+":before", coordinator, store, corpus, seeded)
	if err != nil {
		return ProfileComparisonReport{}, err
	}

	_, rebuiltCount, rebuiltLag, err := buildProfileProjection(ctx, store, seeded, profile, embedder)
	if err != nil {
		return ProfileComparisonReport{}, fmt.Errorf("rebuild profile %q: %w", spec.ID, err)
	}
	rebuiltQueries, err := executeProfileQueries(ctx, runID+":"+spec.ID+":after", coordinator, store, corpus, seeded)
	if err != nil {
		return ProfileComparisonReport{}, err
	}
	rebuildEquivalent := comparableQueryResults(queries) == comparableQueryResults(rebuiltQueries)
	metrics := AggregateMetrics(queries)
	hardGates := HardGateReport{
		ForbiddenCount:  metrics.ForbiddenCount,
		IneligibleCount: metrics.IneligibleCount,
	}
	for _, query := range queries {
		if query.DegradedToLexical {
			hardGates.DegradedCount++
		}
	}
	hardGates.Pass = hardGates.ForbiddenCount == 0 && hardGates.IneligibleCount == 0 &&
		hardGates.DegradedCount == 0 && rebuildEquivalent && lag == 0 && rebuiltLag == 0 && vectorCount == rebuiltCount
	return ProfileComparisonReport{
		ProfileID: spec.ID, Model: spec.Model, Dimensions: spec.Dimensions, LifecycleStatus: spec.Status,
		EmbeddingRequestCount: embedder.Count(), ProjectionBuildDuration: buildDuration,
		ProjectionVectorCount: vectorCount, ProjectionLag: rebuiltLag,
		Queries: queries, Metrics: metrics, Cohorts: AggregateCohorts(queries),
		HardGates: hardGates, ProjectionRebuildEquivalent: rebuildEquivalent,
	}, nil
}

func buildProfileProjection(
	ctx context.Context,
	store *runtime.Store,
	seeded SeededCorpus,
	profile runtime.RetrievalProfile,
	embedder runtime.Embedder,
) (time.Duration, int64, int64, error) {
	tenantSet := make(map[string]struct{})
	for _, scope := range seeded.Scopes {
		tenantSet[scope.TenantID] = struct{}{}
	}
	tenantIDs := make([]string, 0, len(tenantSet))
	for tenantID := range tenantSet {
		tenantIDs = append(tenantIDs, tenantID)
	}
	sort.Strings(tenantIDs)
	started := time.Now()
	for _, tenantID := range tenantIDs {
		if err := store.ResetVectorProjection(ctx, tenantID, profile.ID); err != nil {
			return 0, 0, 0, err
		}
		worker, err := runtime.NewProjectionWorker(store, embedder, runtime.ProjectionWorkerOptions{
			TenantID: tenantID, Profile: profile, BatchSize: 256,
		})
		if err != nil {
			return 0, 0, 0, err
		}
		if _, err := worker.RebuildCurrent(ctx); err != nil {
			return 0, 0, 0, err
		}
	}
	duration := time.Since(started)
	var vectorCount, lag int64
	for _, tenantID := range tenantIDs {
		status, err := store.RetrievalProjectionStatus(ctx, tenantID, profile.ID)
		if err != nil {
			return 0, 0, 0, err
		}
		vectorCount += status.VectorCount
		lag += status.Lag
	}
	return duration, vectorCount, lag, nil
}

func executeProfileQueries(
	ctx context.Context,
	operationPrefix string,
	coordinator *runtime.RetrievalCoordinator,
	store *runtime.Store,
	corpus Corpus,
	seeded SeededCorpus,
) ([]QueryReport, error) {
	memoryToRecord := make(map[string]string, len(seeded.Records))
	for recordID, record := range seeded.Records {
		memoryToRecord[record.MemoryID] = recordID
	}
	reports := make([]QueryReport, 0, len(corpus.Queries))
	for _, query := range corpus.Queries {
		scope, exists := seeded.Scopes[query.ScopeID]
		if !exists {
			return nil, fmt.Errorf("query %q references unseeded scope %q", query.ID, query.ScopeID)
		}
		active, err := activeMemorySet(ctx, store, scope)
		if err != nil {
			return nil, err
		}
		started := time.Now()
		result, err := coordinator.Retrieve(ctx, runtime.RetrievalRequest{
			OperationID: operationPrefix + ":" + query.ID,
			TenantID:    scope.TenantID, ContinuityIDs: []string{scope.ContinuityID},
			Query: query.Text, Limit: query.Limit, Mode: runtime.RetrievalVector,
		})
		duration := time.Since(started)
		if err != nil {
			return nil, fmt.Errorf("profile query %q: %w", query.ID, err)
		}
		ranked := make([]RankedResult, 0, len(result.Memories))
		for index, memory := range result.Memories {
			ranked = append(ranked, RankedResult{
				MemoryID: memory.ID, RecordID: memoryToRecord[memory.ID], Content: memory.Content,
				VectorRank: index + 1, Eligible: active[memory.ID],
			})
		}
		reports = append(reports, QueryReport{
			QueryID: query.ID, Cohorts: append([]string(nil), query.Cohorts...), Duration: duration,
			Results: ranked, Metrics: ScoreQuery(query, ranked), DegradedToLexical: result.Degraded,
		})
	}
	return reports, nil
}

func comparableQueryResults(queries []QueryReport) string {
	type result struct {
		QueryID  string
		Records  []string
		Degraded bool
	}
	values := make([]result, 0, len(queries))
	for _, query := range queries {
		values = append(values, result{QueryID: query.QueryID, Records: recordIDs(query.Results), Degraded: query.DegradedToLexical})
	}
	payload, _ := json.Marshal(values)
	return string(payload)
}

func profileComparisonFingerprint(report ProfileComparison) string {
	type profileIdentity struct {
		ProfileID       string `json:"profile_id"`
		Model           string `json:"model"`
		Dimensions      int    `json:"dimensions"`
		LifecycleStatus string `json:"lifecycle_status"`
	}
	identity := struct {
		RunID                  string                 `json:"run_id"`
		CorpusSHA256           string                 `json:"corpus_sha256"`
		ImplementationRevision string                 `json:"implementation_revision"`
		Profiles               []profileIdentity      `json:"profiles"`
		Policy                 ProfilePromotionPolicy `json:"policy"`
	}{
		RunID: report.RunID, CorpusSHA256: report.CorpusSHA256,
		ImplementationRevision: report.ImplementationRevision, Policy: report.Policy,
	}
	for _, profile := range report.Profiles {
		identity.Profiles = append(identity.Profiles, profileIdentity{
			ProfileID: profile.ProfileID, Model: profile.Model, Dimensions: profile.Dimensions,
			LifecycleStatus: profile.LifecycleStatus,
		})
	}
	payload, _ := json.Marshal(identity)
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:])
}

type profileCountingEmbedder struct {
	runtime.Embedder
	requests atomic.Int64
}

func (embedder *profileCountingEmbedder) Embed(ctx context.Context, content string) ([]float32, error) {
	embedder.requests.Add(1)
	return embedder.Embedder.Embed(ctx, content)
}

func (embedder *profileCountingEmbedder) Count() int64 {
	return embedder.requests.Load()
}

type authoritySeedBackend struct{}

func (authoritySeedBackend) Name() string                                    { return "authority-only" }
func (authoritySeedBackend) Health(context.Context) error                    { return nil }
func (authoritySeedBackend) Put(context.Context, memorybackend.Record) error { return nil }
func (authoritySeedBackend) Search(context.Context, memorybackend.Query) ([]memorybackend.Result, error) {
	return nil, nil
}
func (authoritySeedBackend) Update(context.Context, memorybackend.Record) error        { return nil }
func (authoritySeedBackend) Delete(context.Context, memorybackend.Scope, string) error { return nil }
func (authoritySeedBackend) ResetScope(context.Context, memorybackend.Scope) error     { return nil }
func (authoritySeedBackend) RebuildScope(context.Context, memorybackend.Scope, []memorybackend.Record) error {
	return nil
}
func (authoritySeedBackend) Stats(context.Context) (memorybackend.Stats, error) {
	return memorybackend.Stats{}, nil
}
