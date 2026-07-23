package retrievalablation

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func Execute(ctx context.Context, options Options, outputDir string) (Report, ArtifactPaths, bool, error) {
	if err := validateRunOptions(options); err != nil {
		return Report{}, ArtifactPaths{}, false, err
	}
	if strings.TrimSpace(outputDir) == "" {
		return Report{}, ArtifactPaths{}, false, fmt.Errorf("output directory is required")
	}
	corpus, err := LoadCorpus(options.CorpusPath)
	if err != nil {
		return Report{}, ArtifactPaths{}, false, err
	}
	repositoryRoot, err := findRepositoryRoot(options.CorpusPath)
	if err != nil {
		return Report{}, ArtifactPaths{}, false, err
	}
	if err := ValidateCorpus(repositoryRoot, corpus); err != nil {
		return Report{}, ArtifactPaths{}, false, err
	}
	corpusHash, err := CorpusSHA256(corpus)
	if err != nil {
		return Report{}, ArtifactPaths{}, false, err
	}
	identity := Report{
		RunID:                  options.RunID,
		CorpusSHA256:           corpusHash,
		ImplementationRevision: options.ImplementationRevision,
		EngineVersion:          "rrf-v1",
		Embedding: EmbeddingProfile{
			BaseURL: options.EmbeddingBaseURL, Model: options.EmbeddingModel, Dimensions: options.EmbeddingDimensions,
		},
	}
	identity.RequestFingerprint = ReportRequestFingerprint(identity)
	if existing, paths, found, err := loadReportReplay(outputDir, identity); err != nil {
		return Report{}, ArtifactPaths{}, false, err
	} else if found {
		return existing, paths, true, nil
	}
	report, err := Run(ctx, options)
	if err != nil {
		return Report{}, ArtifactPaths{}, false, err
	}
	paths, replayed, err := WriteReport(outputDir, report)
	if err != nil {
		return Report{}, ArtifactPaths{}, false, err
	}
	return report, paths, replayed, nil
}

func ReportRequestFingerprint(report Report) string {
	identity := struct {
		RunID                  string           `json:"run_id"`
		CorpusSHA256           string           `json:"corpus_sha256"`
		ImplementationRevision string           `json:"implementation_revision"`
		EngineVersion          string           `json:"engine_version"`
		Embedding              EmbeddingProfile `json:"embedding"`
	}{
		RunID:                  strings.TrimSpace(report.RunID),
		CorpusSHA256:           strings.TrimSpace(report.CorpusSHA256),
		ImplementationRevision: strings.TrimSpace(report.ImplementationRevision),
		EngineVersion:          strings.TrimSpace(report.EngineVersion),
		Embedding:              report.Embedding,
	}
	payload, _ := json.Marshal(identity)
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:])
}

func NormalizeReport(report Report) Report {
	report.Conditions = append([]ConditionReport(nil), report.Conditions...)
	conditionOrder := map[string]int{ConditionLexical: 0, ConditionVector: 1, ConditionHybrid: 2}
	sort.Slice(report.Conditions, func(i, j int) bool {
		left, leftKnown := conditionOrder[report.Conditions[i].Name]
		right, rightKnown := conditionOrder[report.Conditions[j].Name]
		if leftKnown && rightKnown && left != right {
			return left < right
		}
		if leftKnown != rightKnown {
			return leftKnown
		}
		return report.Conditions[i].Name < report.Conditions[j].Name
	})
	for index := range report.Conditions {
		condition := &report.Conditions[index]
		condition.Queries = append([]QueryReport(nil), condition.Queries...)
		sort.Slice(condition.Queries, func(i, j int) bool { return condition.Queries[i].QueryID < condition.Queries[j].QueryID })
		for queryIndex := range condition.Queries {
			condition.Queries[queryIndex].Cohorts = sortedStrings(condition.Queries[queryIndex].Cohorts)
		}
	}
	report.Failures = append([]RunFailure(nil), report.Failures...)
	sort.Slice(report.Failures, func(i, j int) bool {
		if report.Failures[i].Condition != report.Failures[j].Condition {
			return report.Failures[i].Condition < report.Failures[j].Condition
		}
		return report.Failures[i].QueryID < report.Failures[j].QueryID
	})
	report.NonClaims = sortedStrings(report.NonClaims)
	return report
}

func WriteReport(outputDir string, report Report) (ArtifactPaths, bool, error) {
	outputDir = strings.TrimSpace(outputDir)
	if outputDir == "" {
		return ArtifactPaths{}, false, fmt.Errorf("output directory is required")
	}
	if report.RunID == "" || report.CorpusSHA256 == "" || report.ImplementationRevision == "" || report.EngineVersion == "" {
		return ArtifactPaths{}, false, fmt.Errorf("report identity is incomplete")
	}
	expectedFingerprint := ReportRequestFingerprint(report)
	if report.RequestFingerprint == "" {
		report.RequestFingerprint = expectedFingerprint
	}
	if report.RequestFingerprint != expectedFingerprint {
		return ArtifactPaths{}, false, fmt.Errorf("report request fingerprint does not match its identity")
	}
	absoluteDir, err := filepath.Abs(outputDir)
	if err != nil {
		return ArtifactPaths{}, false, fmt.Errorf("resolve report output directory: %w", err)
	}
	if err := os.MkdirAll(absoluteDir, 0o755); err != nil {
		return ArtifactPaths{}, false, fmt.Errorf("create report output directory: %w", err)
	}
	paths := ArtifactPaths{
		JSON:     filepath.Join(absoluteDir, "report.json"),
		Markdown: filepath.Join(absoluteDir, "report.md"),
	}
	if _, existingPaths, found, err := loadReportReplay(absoluteDir, report); err != nil {
		return ArtifactPaths{}, false, err
	} else if found {
		return existingPaths, true, nil
	}

	report = NormalizeReport(report)
	jsonPayload, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return ArtifactPaths{}, false, fmt.Errorf("encode retrieval report: %w", err)
	}
	jsonPayload = append(jsonPayload, '\n')
	markdown := renderReportMarkdown(report)
	if err := atomicWriteFile(paths.JSON, jsonPayload); err != nil {
		return ArtifactPaths{}, false, err
	}
	if err := atomicWriteFile(paths.Markdown, []byte(markdown)); err != nil {
		return ArtifactPaths{}, false, err
	}
	return paths, false, nil
}

func loadReportReplay(outputDir string, identity Report) (Report, ArtifactPaths, bool, error) {
	absoluteDir, err := filepath.Abs(strings.TrimSpace(outputDir))
	if err != nil {
		return Report{}, ArtifactPaths{}, false, fmt.Errorf("resolve report replay directory: %w", err)
	}
	paths := ArtifactPaths{JSON: filepath.Join(absoluteDir, "report.json"), Markdown: filepath.Join(absoluteDir, "report.md")}
	payload, err := os.ReadFile(paths.JSON)
	if os.IsNotExist(err) {
		return Report{}, paths, false, nil
	}
	if err != nil {
		return Report{}, ArtifactPaths{}, false, fmt.Errorf("read existing report replay: %w", err)
	}
	var existing Report
	if err := json.Unmarshal(payload, &existing); err != nil {
		return Report{}, ArtifactPaths{}, false, fmt.Errorf("decode existing report replay: %w", err)
	}
	if existing.RequestFingerprint != identity.RequestFingerprint || existing.RunID != identity.RunID {
		return Report{}, ArtifactPaths{}, false, fmt.Errorf("conflicting report replay for run_id %q", identity.RunID)
	}
	if _, err := os.Stat(paths.Markdown); err != nil {
		return Report{}, ArtifactPaths{}, false, fmt.Errorf("existing report replay is missing report.md: %w", err)
	}
	return existing, paths, true, nil
}

func renderReportMarkdown(report Report) string {
	var output bytes.Buffer
	output.WriteString("# Production Retrieval Ablation\n\n")
	fmt.Fprintf(&output, "- Run: `%s`\n", report.RunID)
	fmt.Fprintf(&output, "- Corpus SHA-256: `%s`\n", report.CorpusSHA256)
	fmt.Fprintf(&output, "- Implementation: `%s`\n", report.ImplementationRevision)
	fmt.Fprintf(&output, "- Engine: `%s`\n", report.EngineVersion)
	fmt.Fprintf(&output, "- PostgreSQL schema: `%d`\n", report.SchemaVersion)
	fmt.Fprintf(&output, "- Embedding: `%s` / `%d` dimensions\n", report.Embedding.Model, report.Embedding.Dimensions)
	fmt.Fprintf(&output, "- Embedding requests: `%d`\n", report.EmbeddingRequestCount)
	fmt.Fprintf(&output, "- Hard gates: %s\n", passLabel(report.HardGates.Pass))
	fmt.Fprintf(&output, "- Projection rebuild equivalent: `%t`\n", report.ProjectionRebuildEquivalent)
	fmt.Fprintf(&output, "- Qualification: `%s`\n\n", report.QualificationStatus)
	output.WriteString("## Conditions\n\n")
	output.WriteString("| Condition | Queries | Hit@1 | Recall@K | MRR | nDCG@K | Forbidden | Ineligible | P95 |\n")
	output.WriteString("|---|---:|---:|---:|---:|---:|---:|---:|---:|\n")
	for _, condition := range report.Conditions {
		metrics := condition.Metrics
		fmt.Fprintf(&output, "| `%s` | %d | %.4f | %.4f | %.4f | %.4f | %d | %d | %s |\n",
			condition.Name, metrics.QueryCount, metrics.HitAt1, metrics.RecallAtK, metrics.MRR,
			metrics.NDCGAtK, metrics.ForbiddenCount, metrics.IneligibleCount, metrics.SearchP95)
	}
	if len(report.Failures) > 0 {
		output.WriteString("\n## Failures\n\n")
		for _, failure := range report.Failures {
			fmt.Fprintf(&output, "- `%s/%s`: %s\n", failure.Condition, failure.QueryID, failure.Error)
		}
	}
	output.WriteString("\n## Non-Claims\n\n")
	for _, nonClaim := range report.NonClaims {
		fmt.Fprintf(&output, "- %s\n", nonClaim)
	}
	return output.String()
}

func atomicWriteFile(path string, payload []byte) error {
	temporary, err := os.CreateTemp(filepath.Dir(path), ".retrieval-report-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary report artifact: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if _, err := temporary.Write(payload); err != nil {
		temporary.Close()
		return fmt.Errorf("write temporary report artifact: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return fmt.Errorf("sync temporary report artifact: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close temporary report artifact: %w", err)
	}
	if err := os.Chmod(temporaryPath, 0o644); err != nil {
		return fmt.Errorf("set report artifact permissions: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("publish report artifact: %w", err)
	}
	return nil
}

func passLabel(pass bool) string {
	if pass {
		return "PASS"
	}
	return "FAIL"
}
