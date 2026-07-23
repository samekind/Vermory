package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime/debug"
	"sort"
	"strings"
	"unicode/utf8"

	"vermory/internal/artifact"
	"vermory/internal/benchmark"
	"vermory/internal/provider"
	vermoryruntime "vermory/internal/runtime"
)

const (
	longMemEvalNoContext        = "no_context"
	longMemEvalFullHistory      = "full_oracle_history"
	longMemEvalPlainRetrieval   = "plain_lexical_retrieval"
	longMemEvalVermoryPacket    = "vermory_packet"
	longMemEvalSystemPrompt     = "Answer the question only from the supplied conversation memory. If the information is absent or insufficient, say that it cannot be determined. Respond in English with the shortest sufficient answer and reuse exact factual wording or numbers from the source when possible. Do not use tools or external sources."
	longMemEvalArtifactPrefix   = "benchmarks"
	longMemEvalConversationChan = "benchmark_longmemeval"
)

type LongMemEvalOptions struct {
	QualificationPath      string
	ExecutionPath          string
	SourceDatasetPath      string
	DatabaseURL            string
	ArtifactRoot           string
	Provider               string
	BaseURL                string
	APIKeyEnv              string
	Model                  string
	RunID                  string
	ImplementationRevision string
	ProviderOverride       provider.Provider
	ProviderName           string
	ProviderMode           string
}

type LongMemEvalReport struct {
	RunID             string                          `json:"run_id"`
	Benchmark         string                          `json:"benchmark"`
	ExecutionScope    benchmark.ExecutionScope        `json:"execution_scope"`
	ClaimScope        benchmark.ClaimScope            `json:"claim_scope"`
	DatasetSHA256     string                          `json:"dataset_sha256"`
	FixtureSHA256     string                          `json:"fixture_sha256"`
	SourceRecords     int                             `json:"source_records"`
	SelectedRecordIDs []string                        `json:"selected_record_ids"`
	ProviderMode      string                          `json:"provider_mode"`
	ProviderName      string                          `json:"provider_name"`
	Model             string                          `json:"model"`
	Conditions        []string                        `json:"conditions"`
	Results           []LongMemEvalConditionResult    `json:"results"`
	Aggregates        map[string]LongMemEvalAggregate `json:"aggregates"`
	Artifacts         map[string]string               `json:"artifacts"`
	NonClaims         []string                        `json:"non_claims"`
}

type LongMemEvalConditionResult struct {
	RecordID     string                        `json:"record_id"`
	QuestionType string                        `json:"question_type"`
	Condition    string                        `json:"condition"`
	Status       string                        `json:"status"`
	Response     string                        `json:"response,omitempty"`
	Model        string                        `json:"model,omitempty"`
	Error        string                        `json:"error,omitempty"`
	Score        *benchmark.DeterministicScore `json:"score,omitempty"`
	RequestURI   string                        `json:"request_uri"`
	ResponseURI  string                        `json:"response_uri"`
}

type LongMemEvalAggregate struct {
	Total                 int     `json:"total"`
	Completed             int     `json:"completed"`
	Failed                int     `json:"failed"`
	ExactMatches          int     `json:"exact_matches"`
	MeanTokenF1           float64 `json:"mean_token_f1"`
	MeanAnswerTokenRecall float64 `json:"mean_answer_token_recall"`
	AbstentionExpected    int     `json:"abstention_expected"`
	AbstentionDetected    int     `json:"abstention_detected"`
}

type longMemEvalRequestArtifact struct {
	RecordID     string `json:"record_id"`
	QuestionType string `json:"question_type"`
	QuestionDate string `json:"question_date"`
	Condition    string `json:"condition"`
	Question     string `json:"question"`
	Context      string `json:"context,omitempty"`
}

type longMemEvalResponseArtifact struct {
	RecordID    string `json:"record_id"`
	Condition   string `json:"condition"`
	Status      string `json:"status"`
	Model       string `json:"model,omitempty"`
	Output      string `json:"output,omitempty"`
	Error       string `json:"error,omitempty"`
	RawArtifact string `json:"raw_artifact,omitempty"`
}

func LongMemEvalConditions() []string {
	return []string{
		longMemEvalNoContext,
		longMemEvalFullHistory,
		longMemEvalPlainRetrieval,
		longMemEvalVermoryPacket,
	}
}

func RunLongMemEvalSample(ctx context.Context, opts LongMemEvalOptions) (LongMemEvalReport, error) {
	if strings.TrimSpace(opts.DatabaseURL) == "" {
		return LongMemEvalReport{}, errors.New("benchmark-longmemeval requires database-url")
	}
	if strings.TrimSpace(opts.SourceDatasetPath) == "" {
		return LongMemEvalReport{}, errors.New("benchmark-longmemeval requires source-dataset-path")
	}
	if strings.TrimSpace(opts.ExecutionPath) == "" {
		return LongMemEvalReport{}, errors.New("benchmark-longmemeval requires execution-path")
	}
	if strings.TrimSpace(opts.ArtifactRoot) == "" {
		opts.ArtifactRoot = "./artifacts"
	}

	root, err := projectRoot()
	if err != nil {
		return LongMemEvalReport{}, err
	}
	executionPath := resolveBenchmarkPath(root, opts.ExecutionPath)
	execution, err := benchmark.LoadExecution(executionPath)
	if err != nil {
		return LongMemEvalReport{}, err
	}
	qualificationPath := strings.TrimSpace(opts.QualificationPath)
	if qualificationPath == "" {
		qualificationPath = execution.QualificationPath
	}
	qualificationPath = resolveBenchmarkPath(root, qualificationPath)
	qualification, err := benchmark.LoadQualification(qualificationPath)
	if err != nil {
		return LongMemEvalReport{}, err
	}
	if err := benchmark.ValidateExecution(qualification, execution); err != nil {
		return LongMemEvalReport{}, err
	}

	sourcePath := resolveBenchmarkPath(root, opts.SourceDatasetPath)
	if err := benchmark.VerifyFileSHA256(sourcePath, qualification.Dataset.SHA256); err != nil {
		return LongMemEvalReport{}, fmt.Errorf("verify official source dataset: %w", err)
	}
	if info, err := os.Stat(sourcePath); err != nil {
		return LongMemEvalReport{}, err
	} else if info.Size() != qualification.Dataset.SizeBytes {
		return LongMemEvalReport{}, fmt.Errorf("official source dataset size is %d, want %d", info.Size(), qualification.Dataset.SizeBytes)
	}
	sourceRecords, err := benchmark.LoadLongMemEval(sourcePath)
	if err != nil {
		return LongMemEvalReport{}, err
	}
	if len(sourceRecords) != qualification.Dataset.RecordCount {
		return LongMemEvalReport{}, fmt.Errorf("official source dataset has %d records, want %d", len(sourceRecords), qualification.Dataset.RecordCount)
	}

	fixturePath := resolveBenchmarkPath(root, execution.FixturePath)
	if err := benchmark.VerifyFileSHA256(fixturePath, execution.FixtureSHA256); err != nil {
		return LongMemEvalReport{}, fmt.Errorf("verify frozen fixture: %w", err)
	}
	fixtureRecords, err := benchmark.LoadLongMemEval(fixturePath)
	if err != nil {
		return LongMemEvalReport{}, err
	}
	selectedSource, err := benchmark.SelectRecords(sourceRecords, execution.SelectedRecordIDs)
	if err != nil {
		return LongMemEvalReport{}, err
	}
	selectedFixture, err := benchmark.SelectRecords(fixtureRecords, execution.SelectedRecordIDs)
	if err != nil {
		return LongMemEvalReport{}, err
	}
	if !reflect.DeepEqual(selectedSource, selectedFixture) {
		return LongMemEvalReport{}, errors.New("frozen fixture records do not match the qualified source dataset")
	}

	runID := strings.TrimSpace(opts.RunID)
	if runID == "" {
		runID = strings.TrimSpace(execution.RunID)
	}
	if runID == "" {
		runID = chooseRunID("", "longmemeval-original-sample")
	}
	llm, providerMode, providerName, model, err := longMemEvalProvider(opts)
	if err != nil {
		return LongMemEvalReport{}, err
	}

	store, err := vermoryruntime.OpenStore(ctx, opts.DatabaseURL)
	if err != nil {
		return LongMemEvalReport{}, err
	}
	defer store.Close()
	if err := store.Migrate(ctx); err != nil {
		return LongMemEvalReport{}, err
	}

	artifactStore := artifact.NewLocalStore(opts.ArtifactRoot)
	report := LongMemEvalReport{
		RunID:             runID,
		Benchmark:         execution.Benchmark,
		ExecutionScope:    execution.ExecutionScope,
		ClaimScope:        execution.ClaimScope,
		DatasetSHA256:     qualification.Dataset.SHA256,
		FixtureSHA256:     execution.FixtureSHA256,
		SourceRecords:     len(sourceRecords),
		SelectedRecordIDs: append([]string(nil), execution.SelectedRecordIDs...),
		ProviderMode:      providerMode,
		ProviderName:      providerName,
		Model:             model,
		Conditions:        LongMemEvalConditions(),
		Aggregates:        make(map[string]LongMemEvalAggregate),
		Artifacts:         make(map[string]string),
		NonClaims:         append([]string(nil), execution.NonClaims...),
	}
	tenantID := "benchmark:" + runID
	conversation := vermoryruntime.NewConversationService(store, tenantID, llm, model, vermoryruntime.ConversationServiceConfig{MemoryLimit: 10, RecentLimit: 1})

	for _, record := range selectedFixture {
		for _, condition := range report.Conditions {
			result, err := runLongMemEvalCondition(ctx, artifactStore, store, conversation, qualification, runID, tenantID, record, condition, llm, model)
			if err != nil {
				return LongMemEvalReport{}, err
			}
			report.Results = append(report.Results, result)
		}
	}
	report.Aggregates = aggregateLongMemEval(report.Results)

	artifactPrefix := filepath.ToSlash(filepath.Join(longMemEvalArtifactPrefix, runID))
	sourceURI, err := putJSONArtifact(ctx, artifactStore, filepath.ToSlash(filepath.Join(artifactPrefix, "source.json")), map[string]any{
		"qualification":       qualification,
		"execution_scope":     execution.ExecutionScope,
		"claim_scope":         execution.ClaimScope,
		"sampling_rule":       execution.SamplingRule,
		"selected_record_ids": execution.SelectedRecordIDs,
		"source_dataset_path": qualification.Dataset.Path,
		"source_records":      len(sourceRecords),
		"fixture_sha256":      execution.FixtureSHA256,
	})
	if err != nil {
		return LongMemEvalReport{}, err
	}
	report.Artifacts["source"] = sourceURI
	scoresURI, err := putJSONArtifact(ctx, artifactStore, filepath.ToSlash(filepath.Join(artifactPrefix, "scores.json")), map[string]any{
		"run_id":      runID,
		"claim_scope": execution.ClaimScope,
		"aggregates":  report.Aggregates,
		"results":     report.Results,
	})
	if err != nil {
		return LongMemEvalReport{}, err
	}
	report.Artifacts["scores"] = scoresURI
	reportURI, err := putTextArtifact(ctx, artifactStore, filepath.ToSlash(filepath.Join(artifactPrefix, "report.md")), markdownLongMemEvalReport(report))
	if err != nil {
		return LongMemEvalReport{}, err
	}
	report.Artifacts["report"] = reportURI

	finalExecution := execution
	finalExecution.RunID = runID
	finalExecution.ImplementationRev = strings.TrimSpace(opts.ImplementationRevision)
	if finalExecution.ImplementationRev == "" {
		finalExecution.ImplementationRev = buildVCSRevision()
	}
	finalExecution.Conditions = LongMemEvalConditions()
	finalExecution.Artifacts = copyStringMap(report.Artifacts)
	manifestURI, err := localArtifactURI(opts.ArtifactRoot, filepath.ToSlash(filepath.Join(artifactPrefix, "execution-manifest.json")))
	if err != nil {
		return LongMemEvalReport{}, err
	}
	finalExecution.Artifacts["execution_manifest"] = manifestURI
	if err := benchmark.ValidateExecution(qualification, finalExecution); err != nil {
		return LongMemEvalReport{}, err
	}
	manifestURI, err = putJSONArtifact(ctx, artifactStore, filepath.ToSlash(filepath.Join(artifactPrefix, "execution-manifest.json")), finalExecution)
	if err != nil {
		return LongMemEvalReport{}, err
	}
	report.Artifacts["execution_manifest"] = manifestURI

	if _, err := putJSONArtifact(ctx, artifactStore, filepath.ToSlash(filepath.Join(artifactPrefix, "report.json")), report); err != nil {
		return LongMemEvalReport{}, err
	}
	return report, nil
}

func runLongMemEvalCondition(
	ctx context.Context,
	artifactStore *artifact.LocalStore,
	store *vermoryruntime.Store,
	conversation *vermoryruntime.ConversationService,
	qualification benchmark.Qualification,
	runID, tenantID string,
	record benchmark.LongMemEvalRecord,
	condition string,
	llm provider.Provider,
	model string,
) (LongMemEvalConditionResult, error) {
	contextPacket := ""
	var generated provider.GenerateResponse
	var generateErr error
	result := LongMemEvalConditionResult{
		RecordID:     record.QuestionID,
		QuestionType: record.QuestionType,
		Condition:    condition,
		Status:       "completed",
	}

	switch condition {
	case longMemEvalNoContext:
		generated, generateErr = llm.Generate(ctx, provider.GenerateRequest{Model: model, System: longMemEvalSystemPrompt, Prompt: record.Question})
	case longMemEvalFullHistory:
		contextPacket = longMemEvalFullContext(record)
		generated, generateErr = llm.Generate(ctx, provider.GenerateRequest{Model: model, System: longMemEvalSystemPrompt, Prompt: record.Question, ContextPacket: contextPacket})
	case longMemEvalPlainRetrieval:
		contextPacket = longMemEvalRetrievedContext(benchmark.RetrieveSessions(record, 5))
		generated, generateErr = llm.Generate(ctx, provider.GenerateRequest{Model: model, System: longMemEvalSystemPrompt, Prompt: record.Question, ContextPacket: contextPacket})
	case longMemEvalVermoryPacket:
		anchor := vermoryruntime.ConversationAnchor{Channel: longMemEvalConversationChan, ThreadID: runID + ":" + record.QuestionID}
		resolution, err := store.ResolveOrCreateConversation(ctx, tenantID, anchor)
		if err != nil {
			return LongMemEvalConditionResult{}, err
		}
		for index, session := range longMemEvalSessions(record) {
			receipt, err := store.CommitGovernedObservation(ctx, tenantID, resolution.ContinuityID, vermoryruntime.CommitObservationRequest{
				OperationID: fmt.Sprintf("%s:%s:source:%d", runID, record.QuestionID, index),
				Kind:        vermoryruntime.ObservationKindSourceUpdate,
				Content:     session.SemanticText(),
				SourceRef:   fmt.Sprintf("longmemeval:%s:%s:%s", qualification.Dataset.SHA256, record.QuestionID, session.ID),
			})
			if err != nil {
				return LongMemEvalConditionResult{}, err
			}
			if receipt.Memory.Status != "active" {
				return LongMemEvalConditionResult{}, fmt.Errorf("source session %s was not activated", session.ID)
			}
		}
		operationID := runID + ":" + record.QuestionID + ":answer"
		prepared, err := conversation.PrepareExternalTurn(ctx, vermoryruntime.ExternalConversationTurnRequest{
			OperationID: operationID,
			Anchor:      anchor,
			Message:     record.Question,
		})
		if err != nil {
			return LongMemEvalConditionResult{}, err
		}
		contextPacket = prepared.Context
		generated, generateErr = llm.Generate(ctx, provider.GenerateRequest{
			Model:         model,
			System:        longMemEvalSystemPrompt,
			Prompt:        record.Question,
			ContextPacket: contextPacket,
		})
		if generateErr != nil {
			_, err = conversation.FailExternalTurn(ctx, vermoryruntime.FailExternalConversationTurnRequest{
				OperationID:    operationID,
				Anchor:         anchor,
				FailureCode:    "provider_error",
				FailureMessage: truncateLongMemEvalFailure(generateErr.Error()),
			})
			if err != nil {
				return LongMemEvalConditionResult{}, err
			}
			break
		}
		generatedModel := strings.TrimSpace(generated.Model)
		if generatedModel == "" {
			generatedModel = model
		}
		completed, err := conversation.CompleteExternalTurn(ctx, vermoryruntime.CompleteExternalConversationTurnRequest{
			OperationID: operationID,
			Anchor:      anchor,
			Answer:      generated.Output,
			Model:       generatedModel,
		})
		if err != nil {
			return LongMemEvalConditionResult{}, err
		}
		if completed.Status != vermoryruntime.ChatTurnCompleted {
			return LongMemEvalConditionResult{}, fmt.Errorf("Vermory turn completed with status %q", completed.Status)
		}
	default:
		return LongMemEvalConditionResult{}, fmt.Errorf("unsupported LongMemEval condition %q", condition)
	}

	requestKey := filepath.ToSlash(filepath.Join(longMemEvalArtifactPrefix, runID, "requests", record.QuestionID, condition+".json"))
	requestURI, err := putJSONArtifact(ctx, artifactStore, requestKey, longMemEvalRequestArtifact{
		RecordID:     record.QuestionID,
		QuestionType: record.QuestionType,
		QuestionDate: record.QuestionDate,
		Condition:    condition,
		Question:     record.Question,
		Context:      contextPacket,
	})
	if err != nil {
		return LongMemEvalConditionResult{}, err
	}
	result.RequestURI = requestURI

	responseArtifact := longMemEvalResponseArtifact{RecordID: record.QuestionID, Condition: condition}
	if generateErr != nil {
		result.Status = "failed"
		result.Error = generateErr.Error()
		responseArtifact.Status = "failed"
		responseArtifact.Error = result.Error
	} else {
		result.Response = strings.TrimSpace(generated.Output)
		result.Model = strings.TrimSpace(generated.Model)
		if result.Model == "" {
			result.Model = model
		}
		score := benchmark.ScoreAnswer(record, result.Response)
		result.Score = &score
		responseArtifact.Status = "completed"
		responseArtifact.Model = result.Model
		responseArtifact.Output = result.Response
		responseArtifact.RawArtifact = string(generated.RawArtifact)
	}
	responseKey := filepath.ToSlash(filepath.Join(longMemEvalArtifactPrefix, runID, "responses", record.QuestionID, condition+".json"))
	responseURI, err := putJSONArtifact(ctx, artifactStore, responseKey, responseArtifact)
	if err != nil {
		return LongMemEvalConditionResult{}, err
	}
	result.ResponseURI = responseURI
	return result, nil
}

func longMemEvalFullContext(record benchmark.LongMemEvalRecord) string {
	return longMemEvalRetrievedContext(longMemEvalSessions(record))
}

func longMemEvalRetrievedContext(sessions []benchmark.LongMemEvalSession) string {
	parts := make([]string, 0, len(sessions))
	for _, session := range sessions {
		parts = append(parts, session.SemanticText())
	}
	return strings.Join(parts, "\n\n")
}

func longMemEvalSessions(record benchmark.LongMemEvalRecord) []benchmark.LongMemEvalSession {
	sessions := make([]benchmark.LongMemEvalSession, 0, len(record.HaystackSessions))
	for index, turns := range record.HaystackSessions {
		sessions = append(sessions, benchmark.LongMemEvalSession{
			ID:    record.HaystackSessionIDs[index],
			Date:  record.HaystackDates[index],
			Turns: append([]benchmark.LongMemEvalTurn(nil), turns...),
		})
	}
	return sessions
}

func longMemEvalProvider(opts LongMemEvalOptions) (provider.Provider, string, string, string, error) {
	if opts.ProviderOverride != nil {
		name := strings.TrimSpace(opts.ProviderName)
		if name == "" {
			name = "override"
		}
		mode := strings.TrimSpace(opts.ProviderMode)
		if mode == "" {
			mode = "test"
		}
		model := strings.TrimSpace(opts.Model)
		if model == "" {
			model = "override-model"
		}
		return opts.ProviderOverride, mode, name, model, nil
	}
	return buildProvider(EvalSelfCaseOptions{
		Provider:  opts.Provider,
		BaseURL:   opts.BaseURL,
		APIKeyEnv: opts.APIKeyEnv,
		Model:     opts.Model,
	})
}

func aggregateLongMemEval(results []LongMemEvalConditionResult) map[string]LongMemEvalAggregate {
	aggregates := make(map[string]LongMemEvalAggregate)
	for _, result := range results {
		aggregate := aggregates[result.Condition]
		aggregate.Total++
		if result.Status != "completed" || result.Score == nil {
			aggregate.Failed++
			aggregates[result.Condition] = aggregate
			continue
		}
		aggregate.Completed++
		if result.Score.ExactMatch {
			aggregate.ExactMatches++
		}
		aggregate.MeanTokenF1 += result.Score.TokenF1
		aggregate.MeanAnswerTokenRecall += result.Score.AnswerTokenRecall
		if result.Score.AbstentionExpected {
			aggregate.AbstentionExpected++
			if result.Score.AbstentionDetected {
				aggregate.AbstentionDetected++
			}
		}
		aggregates[result.Condition] = aggregate
	}
	for condition, aggregate := range aggregates {
		if aggregate.Completed > 0 {
			aggregate.MeanTokenF1 = roundBenchmarkMetric(aggregate.MeanTokenF1 / float64(aggregate.Completed))
			aggregate.MeanAnswerTokenRecall = roundBenchmarkMetric(aggregate.MeanAnswerTokenRecall / float64(aggregate.Completed))
		}
		aggregates[condition] = aggregate
	}
	return aggregates
}

func markdownLongMemEvalReport(report LongMemEvalReport) string {
	var b strings.Builder
	b.WriteString("# LongMemEval Original Dataset Sample\n\n")
	b.WriteString(fmt.Sprintf("- Run ID: `%s`\n", report.RunID))
	b.WriteString(fmt.Sprintf("- Scope: `%s`\n", report.ExecutionScope))
	b.WriteString(fmt.Sprintf("- Claim scope: `%s`\n", report.ClaimScope))
	b.WriteString(fmt.Sprintf("- Provider: `%s` / `%s`\n", report.ProviderName, report.Model))
	b.WriteString(fmt.Sprintf("- Qualified source records: `%d`\n", report.SourceRecords))
	b.WriteString(fmt.Sprintf("- Selected records: `%d`\n\n", len(report.SelectedRecordIDs)))
	b.WriteString("## Condition Results\n\n")
	b.WriteString("| Condition | Completed | Failed | Exact | Mean token F1 | Mean answer recall | Abstention |\n")
	b.WriteString("|---|---:|---:|---:|---:|---:|---:|\n")
	conditions := append([]string(nil), report.Conditions...)
	for _, condition := range conditions {
		a := report.Aggregates[condition]
		b.WriteString(fmt.Sprintf("| `%s` | %d | %d | %d/%d | %.4f | %.4f | %d/%d |\n",
			condition, a.Completed, a.Failed, a.ExactMatches, a.Total, a.MeanTokenF1, a.MeanAnswerTokenRecall, a.AbstentionDetected, a.AbstentionExpected))
	}
	b.WriteString("\n## Non-Claims\n\n")
	for _, nonClaim := range report.NonClaims {
		b.WriteString("- " + nonClaim + "\n")
	}
	return b.String()
}

func putJSONArtifact(ctx context.Context, store *artifact.LocalStore, key string, value any) (string, error) {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return "", err
	}
	result, err := store.Put(ctx, key, data)
	if err != nil {
		return "", err
	}
	return result.URI, nil
}

func putTextArtifact(ctx context.Context, store *artifact.LocalStore, key, value string) (string, error) {
	result, err := store.Put(ctx, key, []byte(value))
	if err != nil {
		return "", err
	}
	return result.URI, nil
}

func resolveBenchmarkPath(root, path string) string {
	path = strings.TrimSpace(path)
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Join(root, filepath.Clean(path))
}

func copyStringMap(source map[string]string) map[string]string {
	copy := make(map[string]string, len(source)+1)
	for key, value := range source {
		copy[key] = value
	}
	return copy
}

func buildVCSRevision() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}
	settings := append([]debug.BuildSetting(nil), info.Settings...)
	sort.Slice(settings, func(i, j int) bool { return settings[i].Key < settings[j].Key })
	for _, setting := range settings {
		if setting.Key == "vcs.revision" && strings.TrimSpace(setting.Value) != "" {
			return setting.Value
		}
	}
	return "unknown"
}

func roundBenchmarkMetric(value float64) float64 {
	return float64(int(value*10000+0.5)) / 10000
}

func truncateLongMemEvalFailure(message string) string {
	message = strings.TrimSpace(message)
	if len(message) <= 512 {
		return message
	}
	limit := 512
	for limit > 0 && !utf8.ValidString(message[:limit]) {
		limit--
	}
	return message[:limit]
}
