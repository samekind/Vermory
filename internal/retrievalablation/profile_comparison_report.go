package retrievalablation

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"vermory/internal/memorybackend"
	"vermory/internal/runtime"
)

type ProfileComparisonRunOptions struct {
	DatabaseURL            string
	CorpusPath             string
	RunID                  string
	EmbeddingAPIKey        string
	ImplementationRevision string
}

func ExecuteProfileComparison(
	ctx context.Context,
	options ProfileComparisonRunOptions,
	outputDir string,
) (ProfileComparison, ArtifactPaths, bool, error) {
	if err := validateProfileComparisonRunOptions(options); err != nil {
		return ProfileComparison{}, ArtifactPaths{}, false, err
	}
	if strings.TrimSpace(outputDir) == "" {
		return ProfileComparison{}, ArtifactPaths{}, false, fmt.Errorf("output directory is required")
	}
	corpus, err := LoadCorpus(options.CorpusPath)
	if err != nil {
		return ProfileComparison{}, ArtifactPaths{}, false, err
	}
	repositoryRoot, err := findRepositoryRoot(options.CorpusPath)
	if err != nil {
		return ProfileComparison{}, ArtifactPaths{}, false, err
	}
	if err := ValidateCorpus(repositoryRoot, corpus); err != nil {
		return ProfileComparison{}, ArtifactPaths{}, false, err
	}
	corpusHash, err := CorpusSHA256(corpus)
	if err != nil {
		return ProfileComparison{}, ArtifactPaths{}, false, err
	}
	identity := profileComparisonIdentity(options.RunID, corpusHash, options.ImplementationRevision)
	if existing, paths, found, err := loadProfileComparisonReplay(outputDir, identity); err != nil {
		return ProfileComparison{}, ArtifactPaths{}, false, err
	} else if found {
		return existing, paths, true, nil
	}

	report, err := RunProfileComparison(ctx, options, corpus)
	if err != nil {
		return ProfileComparison{}, ArtifactPaths{}, false, err
	}
	paths, replayed, err := WriteProfileComparison(outputDir, report)
	if err != nil {
		return ProfileComparison{}, ArtifactPaths{}, false, err
	}
	return report, paths, replayed, nil
}

func RunProfileComparison(
	ctx context.Context,
	options ProfileComparisonRunOptions,
	corpus Corpus,
) (ProfileComparison, error) {
	if err := validateProfileComparisonRunOptions(options); err != nil {
		return ProfileComparison{}, err
	}
	store, err := runtime.OpenStore(ctx, options.DatabaseURL)
	if err != nil {
		return ProfileComparison{}, err
	}
	defer store.Close()
	if err := store.Migrate(ctx); err != nil {
		return ProfileComparison{}, err
	}
	schemaVersion, err := store.SchemaVersion(ctx)
	if err != nil {
		return ProfileComparison{}, err
	}
	embedders := make(map[string]runtime.Embedder, 2)
	for _, profileID := range []string{runtime.ProductionRetrievalProfileID, runtime.MigrationRetrievalProfileID} {
		spec, _ := runtime.SupportedRetrievalProfile(profileID)
		embedder, err := memorybackend.NewOpenAIEmbedder(
			spec.BaseURL,
			options.EmbeddingAPIKey,
			spec.Model,
			spec.Dimensions,
			&http.Client{Timeout: 2 * time.Minute},
		)
		if err != nil {
			return ProfileComparison{}, fmt.Errorf("configure profile %q: %w", profileID, err)
		}
		embedders[profileID] = embedder
	}
	return RunProfileComparisonWithDependencies(ctx, ProfileComparisonOptions{
		RunID: options.RunID, Corpus: corpus,
		ImplementationRevision: options.ImplementationRevision,
		SchemaVersion:          schemaVersion,
	}, store, embedders)
}

func validateProfileComparisonRunOptions(options ProfileComparisonRunOptions) error {
	if strings.TrimSpace(options.DatabaseURL) == "" {
		return fmt.Errorf("database URL is required")
	}
	if strings.TrimSpace(options.CorpusPath) == "" {
		return fmt.Errorf("corpus path is required")
	}
	if strings.TrimSpace(options.RunID) == "" {
		return fmt.Errorf("run_id is required")
	}
	if strings.TrimSpace(options.EmbeddingAPIKey) == "" {
		return fmt.Errorf("embedding API key is required")
	}
	if strings.TrimSpace(options.ImplementationRevision) == "" {
		return fmt.Errorf("implementation revision is required")
	}
	return nil
}

func WriteProfileComparison(outputDir string, report ProfileComparison) (ArtifactPaths, bool, error) {
	outputDir = strings.TrimSpace(outputDir)
	if outputDir == "" {
		return ArtifactPaths{}, false, fmt.Errorf("output directory is required")
	}
	if report.RunID == "" || report.CorpusSHA256 == "" || report.ImplementationRevision == "" || len(report.Profiles) != 2 {
		return ArtifactPaths{}, false, fmt.Errorf("profile comparison identity is incomplete")
	}
	expectedFingerprint := profileComparisonFingerprint(report)
	if report.RequestFingerprint == "" {
		report.RequestFingerprint = expectedFingerprint
	}
	if report.RequestFingerprint != expectedFingerprint {
		return ArtifactPaths{}, false, fmt.Errorf("profile comparison request fingerprint does not match its identity")
	}
	absoluteDir, err := filepath.Abs(outputDir)
	if err != nil {
		return ArtifactPaths{}, false, fmt.Errorf("resolve profile comparison output directory: %w", err)
	}
	if err := os.MkdirAll(absoluteDir, 0o755); err != nil {
		return ArtifactPaths{}, false, fmt.Errorf("create profile comparison output directory: %w", err)
	}
	if existing, paths, found, err := loadProfileComparisonReplay(absoluteDir, report); err != nil {
		return ArtifactPaths{}, false, err
	} else if found {
		_ = existing
		return paths, true, nil
	}

	report = normalizeProfileComparison(report)
	payload, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return ArtifactPaths{}, false, fmt.Errorf("encode profile comparison: %w", err)
	}
	payload = append(payload, '\n')
	paths := ArtifactPaths{
		JSON:     filepath.Join(absoluteDir, "report.json"),
		Markdown: filepath.Join(absoluteDir, "report.md"),
	}
	if err := atomicWriteFile(paths.JSON, payload); err != nil {
		return ArtifactPaths{}, false, err
	}
	if err := atomicWriteFile(paths.Markdown, []byte(renderProfileComparisonMarkdown(report))); err != nil {
		return ArtifactPaths{}, false, err
	}
	return paths, false, nil
}

func loadProfileComparisonReplay(
	outputDir string,
	identity ProfileComparison,
) (ProfileComparison, ArtifactPaths, bool, error) {
	absoluteDir, err := filepath.Abs(strings.TrimSpace(outputDir))
	if err != nil {
		return ProfileComparison{}, ArtifactPaths{}, false, fmt.Errorf("resolve profile comparison replay directory: %w", err)
	}
	paths := ArtifactPaths{JSON: filepath.Join(absoluteDir, "report.json"), Markdown: filepath.Join(absoluteDir, "report.md")}
	payload, err := os.ReadFile(paths.JSON)
	if os.IsNotExist(err) {
		return ProfileComparison{}, paths, false, nil
	}
	if err != nil {
		return ProfileComparison{}, ArtifactPaths{}, false, fmt.Errorf("read profile comparison replay: %w", err)
	}
	var existing ProfileComparison
	if err := json.Unmarshal(payload, &existing); err != nil {
		return ProfileComparison{}, ArtifactPaths{}, false, fmt.Errorf("decode profile comparison replay: %w", err)
	}
	if existing.RunID != identity.RunID || existing.RequestFingerprint != identity.RequestFingerprint {
		return ProfileComparison{}, ArtifactPaths{}, false, fmt.Errorf("conflicting profile comparison replay for run_id %q", identity.RunID)
	}
	if _, err := os.Stat(paths.Markdown); err != nil {
		return ProfileComparison{}, ArtifactPaths{}, false, fmt.Errorf("profile comparison replay is missing report.md: %w", err)
	}
	return existing, paths, true, nil
}

func profileComparisonIdentity(runID, corpusHash, revision string) ProfileComparison {
	report := ProfileComparison{
		RunID: strings.TrimSpace(runID), CorpusSHA256: strings.TrimSpace(corpusHash),
		ImplementationRevision: strings.TrimSpace(revision), Policy: DefaultProfilePromotionPolicy(),
	}
	for _, profileID := range []string{runtime.ProductionRetrievalProfileID, runtime.MigrationRetrievalProfileID} {
		spec, _ := runtime.SupportedRetrievalProfile(profileID)
		report.Profiles = append(report.Profiles, ProfileComparisonReport{
			ProfileID: spec.ID, Model: spec.Model, Dimensions: spec.Dimensions, LifecycleStatus: spec.Status,
		})
	}
	report.RequestFingerprint = profileComparisonFingerprint(report)
	return report
}

func normalizeProfileComparison(report ProfileComparison) ProfileComparison {
	report.Profiles = append([]ProfileComparisonReport(nil), report.Profiles...)
	for index := range report.Profiles {
		report.Profiles[index].Queries = append([]QueryReport(nil), report.Profiles[index].Queries...)
		sort.Slice(report.Profiles[index].Queries, func(i, j int) bool {
			return report.Profiles[index].Queries[i].QueryID < report.Profiles[index].Queries[j].QueryID
		})
	}
	report.NonClaims = sortedStrings(report.NonClaims)
	report.Decision.ReasonCodes = sortedStrings(report.Decision.ReasonCodes)
	return report
}

func renderProfileComparisonMarkdown(report ProfileComparison) string {
	var output bytes.Buffer
	output.WriteString("# Retrieval Profile Migration Comparison\n\n")
	fmt.Fprintf(&output, "- Run: `%s`\n", report.RunID)
	fmt.Fprintf(&output, "- Corpus SHA-256: `%s`\n", report.CorpusSHA256)
	fmt.Fprintf(&output, "- Implementation: `%s`\n", report.ImplementationRevision)
	fmt.Fprintf(&output, "- PostgreSQL schema: `%d`\n", report.SchemaVersion)
	fmt.Fprintf(&output, "- Hard gates: %s\n", passLabel(report.HardGates.Pass))
	fmt.Fprintf(&output, "- Decision: `%s`\n", report.Decision.Status)
	fmt.Fprintf(&output, "- Decision reasons: `%s`\n\n", strings.Join(report.Decision.ReasonCodes, ", "))
	output.WriteString("## Profiles\n\n")
	output.WriteString("| Profile | Lifecycle | Model | Queries | Hit@1 | Recall@K | MRR | nDCG@K | P95 | Requests | Vectors | Rebuild |\n")
	output.WriteString("|---|---|---|---:|---:|---:|---:|---:|---:|---:|---:|---|\n")
	for _, profile := range report.Profiles {
		fmt.Fprintf(&output, "| `%s` | `%s` | `%s` | %d | %.4f | %.4f | %.4f | %.4f | %s | %d | %d | `%t` |\n",
			profile.ProfileID, profile.LifecycleStatus, profile.Model, profile.Metrics.QueryCount,
			profile.Metrics.HitAt1, profile.Metrics.RecallAtK, profile.Metrics.MRR,
			profile.Metrics.NDCGAtK, profile.Metrics.SearchP95, profile.EmbeddingRequestCount,
			profile.ProjectionVectorCount, profile.ProjectionRebuildEquivalent)
	}
	output.WriteString("\n## Promotion Policy\n\n")
	fmt.Fprintf(&output, "- Maximum Recall regression: `%.4f`\n", report.Policy.MaxRecallRegression)
	fmt.Fprintf(&output, "- Maximum MRR regression: `%.4f`\n", report.Policy.MaxMRRRegression)
	fmt.Fprintf(&output, "- Maximum nDCG regression: `%.4f`\n", report.Policy.MaxNDCGRegression)
	fmt.Fprintf(&output, "- Maximum P95 ratio: `%.2f`\n", report.Policy.MaxP95Ratio)
	fmt.Fprintf(&output, "- Maximum P95 absolute increase: `%s`\n", report.Policy.MaxP95AbsoluteIncrease)
	fmt.Fprintf(&output, "- Minimum Hit@1 gain: `%.4f`\n", report.Policy.MinHitAt1Gain)
	fmt.Fprintf(&output, "- Minimum MRR gain: `%.4f`\n", report.Policy.MinMRRGain)
	fmt.Fprintf(&output, "- Minimum P95 improvement ratio: `%.2f`\n", report.Policy.MinP95ImprovementRatio)
	output.WriteString("\n## Non-Claims\n\n")
	for _, nonClaim := range report.NonClaims {
		fmt.Fprintf(&output, "- %s\n", nonClaim)
	}
	return output.String()
}
