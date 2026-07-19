package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"vermory/internal/artifact"
	"vermory/internal/benchmark"
	vermoryruntime "vermory/internal/runtime"
)

const (
	longMemEvalRetrievalBaseline = "plain_token_overlap"
	longMemEvalRetrievalVermory  = "vermory_lexical"
	longMemEvalRetrievalVector   = "vermory_vector"
	longMemEvalRetrievalChannel  = "benchmark_longmemeval_retrieval"
	longMemEvalRetrievalLimit    = 12
)

type LongMemEvalRetrievalOptions struct {
	QualificationPath         string
	ExecutionPath             string
	SourceDatasetPath         string
	DatabaseURL               string
	ArtifactRoot              string
	RunID                     string
	ImplementationRevision    string
	Resume                    bool
	VectorProfilePath         string
	EmbeddingAPIKey           string
	VectorEmbedder            vermoryruntime.Embedder
	ProjectionRecoverySleeper func(context.Context, time.Duration) error
}

type LongMemEvalRetrievalReport struct {
	RunID                  string                                              `json:"run_id"`
	Benchmark              string                                              `json:"benchmark"`
	EvaluationTarget       benchmark.EvaluationTarget                          `json:"evaluation_target"`
	ExecutionScope         benchmark.ExecutionScope                            `json:"execution_scope"`
	ClaimScope             benchmark.ClaimScope                                `json:"claim_scope"`
	DatasetSHA256          string                                              `json:"dataset_sha256"`
	RecordSetSHA256        string                                              `json:"record_set_sha256"`
	ImplementationRevision string                                              `json:"implementation_revision"`
	SourceSummary          benchmark.LongMemEvalSummary                        `json:"source_summary"`
	RecordCount            int                                                 `json:"record_count"`
	ScoredRecordCount      int                                                 `json:"scored_record_count"`
	ImportedMemoryCount    int                                                 `json:"imported_memory_count"`
	Conditions             []string                                            `json:"conditions"`
	Results                []LongMemEvalRetrievalRecordResult                  `json:"results"`
	Aggregates             map[string]LongMemEvalRetrievalAggregate            `json:"aggregates"`
	QuestionTypeAggregates map[string]map[string]LongMemEvalRetrievalAggregate `json:"question_type_aggregates"`
	Failures               []LongMemEvalRetrievalFailure                       `json:"failures"`
	Vector                 *LongMemEvalVectorEvidence                          `json:"vector,omitempty"`
	Artifacts              map[string]string                                   `json:"artifacts"`
	NonClaims              []string                                            `json:"non_claims"`
}

type LongMemEvalRetrievalRecordResult struct {
	SchemaVersion          string                                `json:"schema_version"`
	RunID                  string                                `json:"run_id"`
	ImplementationRevision string                                `json:"implementation_revision"`
	DatasetSHA256          string                                `json:"dataset_sha256"`
	RecordSetSHA256        string                                `json:"record_set_sha256"`
	RecordID               string                                `json:"record_id"`
	QuestionType           string                                `json:"question_type"`
	Abstention             bool                                  `json:"abstention"`
	Status                 string                                `json:"status"`
	ContinuityID           string                                `json:"continuity_id,omitempty"`
	ImportedMemoryCount    int                                   `json:"imported_memory_count"`
	Conditions             []LongMemEvalRetrievalConditionResult `json:"conditions,omitempty"`
	Error                  string                                `json:"error,omitempty"`
}

type LongMemEvalRetrievalConditionResult struct {
	Condition            string                            `json:"condition"`
	Status               string                            `json:"status"`
	Classification       string                            `json:"classification"`
	RankedOccurrenceKeys []string                          `json:"ranked_occurrence_keys"`
	RankedSessionIDs     []string                          `json:"ranked_session_ids"`
	MetricAt5            *benchmark.SessionRetrievalMetric `json:"metric_at_5,omitempty"`
	MetricAt10           *benchmark.SessionRetrievalMetric `json:"metric_at_10,omitempty"`
	MetricAt12           *benchmark.SessionRetrievalMetric `json:"metric_at_12,omitempty"`
	LatencyMilliseconds  int64                             `json:"latency_ms"`
	EffectiveMode        vermoryruntime.RetrievalMode      `json:"effective_mode,omitempty"`
	Degraded             bool                              `json:"degraded,omitempty"`
	FailureCode          string                            `json:"failure_code,omitempty"`
	AuditID              string                            `json:"audit_id,omitempty"`
	Error                string                            `json:"error,omitempty"`
}

type LongMemEvalRetrievalAggregate struct {
	Count int                          `json:"count"`
	At5   benchmark.RetrievalAggregate `json:"at_5"`
	At10  benchmark.RetrievalAggregate `json:"at_10"`
	At12  benchmark.RetrievalAggregate `json:"at_12"`
}

type LongMemEvalRetrievalFailure struct {
	RecordID     string `json:"record_id"`
	QuestionType string `json:"question_type"`
	Condition    string `json:"condition,omitempty"`
	Error        string `json:"error"`
}

type LongMemEvalVectorEvidence struct {
	ProfileID                      string                          `json:"profile_id"`
	ProfileSHA256                  string                          `json:"profile_sha256"`
	Provider                       string                          `json:"provider"`
	RetrievalProfile               string                          `json:"retrieval_profile"`
	Model                          string                          `json:"model"`
	Dimensions                     int                             `json:"dimensions"`
	ProjectionClass                string                          `json:"projection_class"`
	WorkerBatchSize                int                             `json:"worker_batch_size"`
	EmbeddingBatchSize             int                             `json:"embedding_batch_size"`
	HTTPTimeoutSeconds             int                             `json:"http_timeout_seconds"`
	MaxAttempts                    int                             `json:"max_attempts"`
	RetryDelayMilliseconds         int                             `json:"retry_delay_milliseconds"`
	RetryBackoff                   string                          `json:"retry_backoff"`
	ProjectionMaxRecoveries        int                             `json:"projection_max_recoveries"`
	ProjectionRecoveryDelaySeconds int                             `json:"projection_recovery_delay_seconds"`
	RecoveredProjectionFailures    int                             `json:"recovered_projection_failures"`
	UnrecoveredProjectionFailures  int                             `json:"unrecovered_projection_failures"`
	ProjectionRecoverySleeps       int                             `json:"projection_recovery_sleeps"`
	ProjectionDurationMilliseconds int64                           `json:"projection_duration_ms"`
	Projection                     vermoryruntime.ProjectionStatus `json:"projection"`
	Embedding                      longMemEvalEmbeddingStats       `json:"embedding"`
	EffectiveVectorQueries         int                             `json:"effective_vector_queries"`
	DegradedVectorQueries          int                             `json:"degraded_vector_queries"`
	FailureCodeBreakdown           map[string]int                  `json:"failure_code_breakdown"`
	HardGatesPass                  bool                            `json:"hard_gates_pass"`
}

type longMemEvalSessionOccurrence struct {
	Key     string
	RawID   string
	Session benchmark.LongMemEvalSession
}

func RunLongMemEvalRetrieval(ctx context.Context, opts LongMemEvalRetrievalOptions) (LongMemEvalRetrievalReport, error) {
	if strings.TrimSpace(opts.DatabaseURL) == "" {
		return LongMemEvalRetrievalReport{}, errors.New("benchmark-longmemeval-retrieval requires database-url")
	}
	if strings.TrimSpace(opts.SourceDatasetPath) == "" {
		return LongMemEvalRetrievalReport{}, errors.New("benchmark-longmemeval-retrieval requires source-dataset-path")
	}
	if strings.TrimSpace(opts.ExecutionPath) == "" {
		return LongMemEvalRetrievalReport{}, errors.New("benchmark-longmemeval-retrieval requires execution-path")
	}
	if strings.TrimSpace(opts.ArtifactRoot) == "" {
		opts.ArtifactRoot = "./artifacts"
	}

	root, err := projectRoot()
	if err != nil {
		return LongMemEvalRetrievalReport{}, err
	}
	executionPath := resolveBenchmarkPath(root, opts.ExecutionPath)
	execution, err := benchmark.LoadExecution(executionPath)
	if err != nil {
		return LongMemEvalRetrievalReport{}, err
	}
	qualificationPath := strings.TrimSpace(opts.QualificationPath)
	if qualificationPath == "" {
		qualificationPath = execution.QualificationPath
	}
	qualification, err := benchmark.LoadQualification(resolveBenchmarkPath(root, qualificationPath))
	if err != nil {
		return LongMemEvalRetrievalReport{}, err
	}
	if err := benchmark.ValidateExecution(qualification, execution); err != nil {
		return LongMemEvalRetrievalReport{}, err
	}
	if execution.EvaluationTarget != benchmark.EvaluationTargetRetrieval || execution.ExecutionScope != benchmark.ExecutionScopeFull {
		return LongMemEvalRetrievalReport{}, fmt.Errorf("LongMemEval retrieval runner requires a full retrieval execution")
	}
	vectorEnabled := slices.Equal(execution.Conditions, longMemEvalVectorConditions())
	if !vectorEnabled && !slices.Equal(execution.Conditions, longMemEvalLexicalConditions()) {
		return LongMemEvalRetrievalReport{}, fmt.Errorf("unsupported LongMemEval retrieval conditions %v", execution.Conditions)
	}
	conditions := append([]string(nil), execution.Conditions...)
	var vectorProfile LongMemEvalVectorProfile
	var vectorProfileSHA string
	if vectorEnabled {
		if strings.TrimSpace(opts.VectorProfilePath) == "" {
			return LongMemEvalRetrievalReport{}, fmt.Errorf("LongMemEval vector execution requires --vector-profile")
		}
		vectorProfile, vectorProfileSHA, err = loadLongMemEvalVectorProfile(resolveBenchmarkPath(root, opts.VectorProfilePath))
		if err != nil {
			return LongMemEvalRetrievalReport{}, err
		}
		if !slices.Equal(vectorProfile.Conditions, execution.Conditions) {
			return LongMemEvalRetrievalReport{}, fmt.Errorf("LongMemEval vector profile conditions do not match execution")
		}
	} else if strings.TrimSpace(opts.VectorProfilePath) != "" || opts.VectorEmbedder != nil {
		return LongMemEvalRetrievalReport{}, fmt.Errorf("legacy LongMemEval lexical execution cannot configure a vector profile")
	}

	sourcePath := resolveBenchmarkPath(root, opts.SourceDatasetPath)
	if err := benchmark.VerifyFileSHA256(sourcePath, qualification.Dataset.SHA256); err != nil {
		return LongMemEvalRetrievalReport{}, fmt.Errorf("verify LongMemEval-S source: %w", err)
	}
	info, err := os.Stat(sourcePath)
	if err != nil {
		return LongMemEvalRetrievalReport{}, err
	}
	if info.Size() != qualification.Dataset.SizeBytes {
		return LongMemEvalRetrievalReport{}, fmt.Errorf("LongMemEval-S source size is %d, want %d", info.Size(), qualification.Dataset.SizeBytes)
	}
	summary, err := benchmark.ScanLongMemEval(sourcePath, nil)
	if err != nil {
		return LongMemEvalRetrievalReport{}, err
	}
	if err := validateLongMemEvalRetrievalSummary(qualification, execution, summary); err != nil {
		return LongMemEvalRetrievalReport{}, err
	}

	runID := chooseRunID(opts.RunID, "longmemeval-s-full-retrieval")
	if err := validateLongMemEvalRetrievalSegment(runID, "run ID"); err != nil {
		return LongMemEvalRetrievalReport{}, err
	}
	implementationRevision := strings.TrimSpace(opts.ImplementationRevision)
	if implementationRevision == "" {
		implementationRevision = buildVCSRevision()
	}
	tenantID := "benchmark:" + runID
	store, err := vermoryruntime.OpenStore(ctx, opts.DatabaseURL)
	if err != nil {
		return LongMemEvalRetrievalReport{}, err
	}
	defer store.Close()
	if err := store.Migrate(ctx); err != nil {
		return LongMemEvalRetrievalReport{}, err
	}
	retriever, err := vermoryruntime.NewRetrievalCoordinator(store, nil, vermoryruntime.RetrievalProfile{})
	if err != nil {
		return LongMemEvalRetrievalReport{}, err
	}
	artifactStore := artifact.NewLocalStore(opts.ArtifactRoot)
	prefix := filepath.ToSlash(filepath.Join("benchmarks", runID))
	var vectorRetriever *vermoryruntime.RetrievalCoordinator
	var vectorMeter *longMemEvalRetryingEmbedder
	var vectorEvidence *LongMemEvalVectorEvidence
	if vectorEnabled {
		vectorMeter, err = configureLongMemEvalVectorEmbedder(opts, vectorProfile)
		if err != nil {
			return LongMemEvalRetrievalReport{}, err
		}
		vectorRetriever, err = vermoryruntime.NewRetrievalCoordinator(store, vectorMeter, vectorProfile.runtimeProfile())
		if err != nil {
			return LongMemEvalRetrievalReport{}, err
		}
		if _, err := benchmark.ScanLongMemEval(sourcePath, func(record benchmark.LongMemEvalRecord) error {
			_, err := importLongMemEvalRetrievalRecord(ctx, store, tenantID, runID, execution, record)
			return err
		}); err != nil {
			return LongMemEvalRetrievalReport{}, fmt.Errorf("import LongMemEval vector authority: %w", err)
		}
		worker, err := vermoryruntime.NewProjectionWorker(store, vectorMeter, vermoryruntime.ProjectionWorkerOptions{
			TenantID:           tenantID,
			Profile:            vectorProfile.runtimeProfile(),
			BatchSize:          vectorProfile.WorkerBatchSize,
			EmbeddingBatchSize: vectorProfile.EmbeddingBatchSize,
		})
		if err != nil {
			return LongMemEvalRetrievalReport{}, err
		}
		projectionStarted := time.Now()
		recoverySleeper := opts.ProjectionRecoverySleeper
		if recoverySleeper == nil {
			recoverySleeper = sleepLongMemEvalEmbeddingRetry
		}
		recoveriesUsed := 0
		pendingProjectionFailures := 0
		recoveredProjectionFailures := 0
		projectionRecoverySleeps := 0
		for {
			result, runErr := worker.RunOnce(ctx)
			if runErr != nil {
				if result.FailureCode == "embedding_unavailable" && vectorProfile.ProjectionMaxRecoveries > 0 {
					if recoveriesUsed >= vectorProfile.ProjectionMaxRecoveries {
						return LongMemEvalRetrievalReport{}, fmt.Errorf(
							"project LongMemEval vector authority: projection recovery budget exhausted after %d recoveries: %w",
							recoveriesUsed, runErr,
						)
					}
					recoveriesUsed++
					pendingProjectionFailures++
					projectionRecoverySleeps++
					if sleepErr := recoverySleeper(ctx, time.Duration(vectorProfile.ProjectionRecoveryDelaySeconds)*time.Second); sleepErr != nil {
						return LongMemEvalRetrievalReport{}, fmt.Errorf("project LongMemEval vector authority recovery cooldown: %w", sleepErr)
					}
					continue
				}
				return LongMemEvalRetrievalReport{}, fmt.Errorf("project LongMemEval vector authority: %w", runErr)
			}
			if pendingProjectionFailures > 0 {
				recoveredProjectionFailures += pendingProjectionFailures
				pendingProjectionFailures = 0
			}
			if result.AlreadyRunning {
				return LongMemEvalRetrievalReport{}, fmt.Errorf("LongMemEval vector projection worker is already running")
			}
			if result.Lag == 0 {
				break
			}
		}
		projectionStatus, err := store.RetrievalProjectionStatus(ctx, tenantID, vectorProfile.RetrievalProfile)
		if err != nil {
			return LongMemEvalRetrievalReport{}, err
		}
		vectorEvidence = &LongMemEvalVectorEvidence{
			ProfileID: vectorProfile.ID, ProfileSHA256: vectorProfileSHA,
			Provider: vectorProfile.Provider, RetrievalProfile: vectorProfile.RetrievalProfile,
			Model: vectorProfile.Model, Dimensions: vectorProfile.Dimensions,
			ProjectionClass:                vectorProfile.ProjectionClass,
			WorkerBatchSize:                vectorProfile.WorkerBatchSize,
			EmbeddingBatchSize:             vectorProfile.EmbeddingBatchSize,
			HTTPTimeoutSeconds:             vectorProfile.HTTPTimeoutSeconds,
			MaxAttempts:                    vectorProfile.MaxAttempts,
			RetryDelayMilliseconds:         vectorProfile.RetryDelayMilliseconds,
			RetryBackoff:                   vectorProfile.RetryBackoff,
			ProjectionMaxRecoveries:        vectorProfile.ProjectionMaxRecoveries,
			ProjectionRecoveryDelaySeconds: vectorProfile.ProjectionRecoveryDelaySeconds,
			RecoveredProjectionFailures:    recoveredProjectionFailures,
			UnrecoveredProjectionFailures:  pendingProjectionFailures,
			ProjectionRecoverySleeps:       projectionRecoverySleeps,
			ProjectionDurationMilliseconds: time.Since(projectionStarted).Milliseconds(),
			Projection:                     projectionStatus, FailureCodeBreakdown: make(map[string]int),
		}
		if projectionStatus.Status != "idle" || projectionStatus.Lag != 0 || projectionStatus.VectorCount != int64(summary.SessionCount) {
			return LongMemEvalRetrievalReport{}, fmt.Errorf("LongMemEval vector projection is not current: status=%s lag=%d vectors=%d want=%d", projectionStatus.Status, projectionStatus.Lag, projectionStatus.VectorCount, summary.SessionCount)
		}
	}
	results := make([]LongMemEvalRetrievalRecordResult, 0, summary.RecordCount)

	_, err = benchmark.ScanLongMemEval(sourcePath, func(record benchmark.LongMemEvalRecord) error {
		checkpointPath, err := longMemEvalRetrievalCheckpointPath(opts.ArtifactRoot, runID, record.QuestionID)
		if err != nil {
			return err
		}
		if _, statErr := os.Stat(checkpointPath); statErr == nil {
			if !opts.Resume {
				return fmt.Errorf("checkpoint already exists for record %q; use --resume", record.QuestionID)
			}
			checkpoint, err := loadLongMemEvalRetrievalCheckpoint(checkpointPath)
			if err != nil {
				return err
			}
			if err := validateLongMemEvalRetrievalCheckpoint(checkpoint, runID, implementationRevision, execution, record.QuestionID, conditions); err != nil {
				return err
			}
			results = append(results, checkpoint)
			return nil
		} else if !errors.Is(statErr, os.ErrNotExist) {
			return statErr
		}

		checkpoint, runErr := runLongMemEvalRetrievalRecord(ctx, store, retriever, vectorRetriever, tenantID, runID, implementationRevision, execution, record)
		if runErr != nil {
			checkpoint.Status = "runtime_failure"
			checkpoint.Error = runErr.Error()
		}
		key := filepath.ToSlash(filepath.Join(prefix, "checkpoints", record.QuestionID+".json"))
		if _, err := putJSONArtifact(ctx, artifactStore, key, checkpoint); err != nil {
			return err
		}
		results = append(results, checkpoint)
		return nil
	})
	if err != nil {
		return LongMemEvalRetrievalReport{}, err
	}

	report := finalizeLongMemEvalRetrievalReport(runID, implementationRevision, qualification, execution, summary, results)
	if vectorEvidence != nil {
		vectorEvidence.Embedding = vectorMeter.stats()
		for _, result := range report.Results {
			for _, condition := range result.Conditions {
				if condition.Condition != longMemEvalRetrievalVector {
					continue
				}
				if condition.Status == "completed" && condition.EffectiveMode == vermoryruntime.RetrievalVector && !condition.Degraded {
					vectorEvidence.EffectiveVectorQueries++
				}
				if condition.Degraded {
					vectorEvidence.DegradedVectorQueries++
					vectorEvidence.FailureCodeBreakdown[condition.FailureCode]++
				}
			}
		}
		vectorEvidence.HardGatesPass = vectorEvidence.Projection.Status == "idle" &&
			vectorEvidence.Projection.Lag == 0 && vectorEvidence.Projection.VectorCount == int64(summary.SessionCount) &&
			vectorEvidence.EffectiveVectorQueries == summary.RecordCount && vectorEvidence.DegradedVectorQueries == 0 &&
			vectorEvidence.UnrecoveredProjectionFailures == 0 &&
			vectorEvidence.ProjectionRecoverySleeps == vectorEvidence.RecoveredProjectionFailures &&
			vectorEvidence.RecoveredProjectionFailures <= vectorEvidence.ProjectionMaxRecoveries &&
			vectorEvidence.Embedding.TerminalFailures == int64(vectorEvidence.RecoveredProjectionFailures) &&
			vectorEvidence.Embedding.SuccessfulItems == int64(summary.SessionCount+summary.RecordCount)
		report.Vector = vectorEvidence
	}
	if err := writeLongMemEvalRetrievalArtifacts(ctx, artifactStore, opts.ArtifactRoot, prefix, qualification, execution, &report); err != nil {
		return LongMemEvalRetrievalReport{}, err
	}
	if len(report.Failures) != 0 || report.ScoredRecordCount != execution.ExpectedScoredRecordCount || report.Vector != nil && !report.Vector.HardGatesPass {
		return report, fmt.Errorf("LongMemEval retrieval completed with %d runtime failures and %d/%d scored records; report=%s",
			len(report.Failures), report.ScoredRecordCount, execution.ExpectedScoredRecordCount, report.Artifacts["report"])
	}
	return report, nil
}

func longMemEvalLexicalConditions() []string {
	return []string{longMemEvalRetrievalBaseline, longMemEvalRetrievalVermory}
}

func longMemEvalVectorConditions() []string {
	return []string{longMemEvalRetrievalBaseline, longMemEvalRetrievalVermory, longMemEvalRetrievalVector}
}

func validateLongMemEvalRetrievalSummary(qualification benchmark.Qualification, execution benchmark.ExecutionManifest, summary benchmark.LongMemEvalSummary) error {
	if summary.RecordCount != qualification.Dataset.RecordCount {
		return fmt.Errorf("LongMemEval-S source has %d records, want %d", summary.RecordCount, qualification.Dataset.RecordCount)
	}
	if summary.RecordSetSHA256 != execution.RecordSetSHA256 {
		return fmt.Errorf("LongMemEval-S record-set digest is %s, want %s", summary.RecordSetSHA256, execution.RecordSetSHA256)
	}
	if summary.SessionCount != execution.ExpectedSessionCount {
		return fmt.Errorf("LongMemEval-S source has %d sessions, want %d", summary.SessionCount, execution.ExpectedSessionCount)
	}
	if summary.TurnCount != execution.ExpectedTurnCount {
		return fmt.Errorf("LongMemEval-S source has %d turns, want %d", summary.TurnCount, execution.ExpectedTurnCount)
	}
	if summary.ScoredRecordCount != execution.ExpectedScoredRecordCount {
		return fmt.Errorf("LongMemEval-S source has %d scored records, want %d", summary.ScoredRecordCount, execution.ExpectedScoredRecordCount)
	}
	return nil
}

func runLongMemEvalRetrievalRecord(
	ctx context.Context,
	store *vermoryruntime.Store,
	retriever *vermoryruntime.RetrievalCoordinator,
	vectorRetriever *vermoryruntime.RetrievalCoordinator,
	tenantID, runID, implementationRevision string,
	execution benchmark.ExecutionManifest,
	record benchmark.LongMemEvalRecord,
) (checkpoint LongMemEvalRetrievalRecordResult, err error) {
	checkpoint = LongMemEvalRetrievalRecordResult{
		SchemaVersion:          "longmemeval-retrieval-checkpoint/v1",
		RunID:                  runID,
		ImplementationRevision: implementationRevision,
		DatasetSHA256:          execution.DatasetSHA256,
		RecordSetSHA256:        execution.RecordSetSHA256,
		RecordID:               record.QuestionID,
		QuestionType:           record.QuestionType,
		Abstention:             strings.HasSuffix(record.QuestionID, "_abs"),
		Status:                 "completed",
	}
	imported, err := importLongMemEvalRetrievalRecord(ctx, store, tenantID, runID, execution, record)
	if err != nil {
		return checkpoint, err
	}
	checkpoint.ContinuityID = imported.ContinuityID
	checkpoint.ImportedMemoryCount = len(imported.Occurrences)

	baselineStarted := time.Now()
	baselineSessions := benchmark.RetrieveSessions(record, longMemEvalRetrievalLimit)
	baselineKeys := make([]string, 0, len(baselineSessions))
	baselineIDs := make([]string, 0, len(baselineSessions))
	for _, session := range baselineSessions {
		baselineKeys = append(baselineKeys, longMemEvalRetrievalOccurrenceKey(session.Position, session.ID))
		baselineIDs = append(baselineIDs, session.ID)
	}
	baseline, err := buildLongMemEvalRetrievalCondition(record, longMemEvalRetrievalBaseline, baselineKeys, baselineIDs, time.Since(baselineStarted))
	if err != nil {
		return checkpoint, err
	}

	vermoryStarted := time.Now()
	retrieved, err := retriever.Retrieve(ctx, vermoryruntime.RetrievalRequest{
		TenantID:      tenantID,
		ContinuityIDs: []string{imported.ContinuityID},
		Query:         record.Question,
		Limit:         longMemEvalRetrievalLimit,
		Mode:          vermoryruntime.RetrievalLexical,
	})
	if err != nil {
		return checkpoint, err
	}
	vermoryKeys, vermoryIDs, err := mapLongMemEvalRetrievedMemories(retrieved.Memories, imported)
	if err != nil {
		return checkpoint, err
	}
	vermoryResult, err := buildLongMemEvalRetrievalCondition(record, longMemEvalRetrievalVermory, vermoryKeys, vermoryIDs, time.Since(vermoryStarted))
	if err != nil {
		return checkpoint, err
	}
	vermoryResult.EffectiveMode = vermoryruntime.RetrievalLexical
	checkpoint.Conditions = []LongMemEvalRetrievalConditionResult{baseline, vermoryResult}

	if vectorRetriever != nil {
		vectorStarted := time.Now()
		vector, err := vectorRetriever.Retrieve(ctx, vermoryruntime.RetrievalRequest{
			OperationID:   fmt.Sprintf("%s:%s:vector", runID, record.QuestionID),
			TenantID:      tenantID,
			ContinuityIDs: []string{imported.ContinuityID},
			Query:         record.Question,
			Limit:         longMemEvalRetrievalLimit,
			Mode:          vermoryruntime.RetrievalVector,
		})
		if err != nil {
			return checkpoint, err
		}
		vectorKeys, vectorIDs, err := mapLongMemEvalRetrievedMemories(vector.Memories, imported)
		if err != nil {
			return checkpoint, err
		}
		vectorResult, err := buildLongMemEvalRetrievalCondition(record, longMemEvalRetrievalVector, vectorKeys, vectorIDs, time.Since(vectorStarted))
		if err != nil {
			return checkpoint, err
		}
		vectorResult.EffectiveMode = vector.Effective
		vectorResult.Degraded = vector.Degraded
		vectorResult.FailureCode = vector.FailureCode
		vectorResult.AuditID = vector.AuditID
		if vector.Degraded || vector.Effective != vermoryruntime.RetrievalVector {
			vectorResult.Status = "degraded"
		}
		checkpoint.Conditions = append(checkpoint.Conditions, vectorResult)
	}
	return checkpoint, nil
}

type importedLongMemEvalRetrievalRecord struct {
	ContinuityID string
	Occurrences  []longMemEvalSessionOccurrence
	ByMemoryID   map[string]longMemEvalSessionOccurrence
	Active       map[string]struct{}
}

func importLongMemEvalRetrievalRecord(
	ctx context.Context,
	store *vermoryruntime.Store,
	tenantID, runID string,
	execution benchmark.ExecutionManifest,
	record benchmark.LongMemEvalRecord,
) (importedLongMemEvalRetrievalRecord, error) {
	anchor := vermoryruntime.ConversationAnchor{Channel: longMemEvalRetrievalChannel, ThreadID: runID + ":" + record.QuestionID}
	resolution, err := store.ResolveOrCreateConversation(ctx, tenantID, anchor)
	if err != nil {
		return importedLongMemEvalRetrievalRecord{}, err
	}
	occurrences := longMemEvalRetrievalOccurrences(record)
	byMemoryID := make(map[string]longMemEvalSessionOccurrence, len(occurrences))
	for _, occurrence := range occurrences {
		receipt, err := store.CommitGovernedObservation(ctx, tenantID, resolution.ContinuityID, vermoryruntime.CommitObservationRequest{
			OperationID: fmt.Sprintf("%s:%s:source:%06d:%s", runID, record.QuestionID, occurrence.Session.Position, occurrence.RawID),
			Kind:        vermoryruntime.ObservationKindSourceUpdate,
			Content:     occurrence.Session.SemanticText(),
			SourceRef:   fmt.Sprintf("longmemeval-s:%s:%s:%06d:%s", execution.DatasetSHA256, record.QuestionID, occurrence.Session.Position, occurrence.RawID),
		})
		if err != nil {
			return importedLongMemEvalRetrievalRecord{}, err
		}
		if receipt.Memory.Status != "active" {
			return importedLongMemEvalRetrievalRecord{}, fmt.Errorf("session occurrence %s was not activated", occurrence.Key)
		}
		byMemoryID[receipt.Memory.MemoryID] = occurrence
	}

	authority, err := store.ListGovernedMemories(ctx, tenantID, resolution.ContinuityID)
	if err != nil {
		return importedLongMemEvalRetrievalRecord{}, err
	}
	if len(authority) != len(occurrences) {
		return importedLongMemEvalRetrievalRecord{}, fmt.Errorf("continuity %s has %d governed memories, want %d", resolution.ContinuityID, len(authority), len(occurrences))
	}
	active := make(map[string]struct{}, len(authority))
	for _, memory := range authority {
		if memory.LifecycleStatus != "active" {
			return importedLongMemEvalRetrievalRecord{}, fmt.Errorf("continuity %s contains non-active memory %s", resolution.ContinuityID, memory.ID)
		}
		active[memory.ID] = struct{}{}
	}
	for memoryID := range byMemoryID {
		if _, exists := active[memoryID]; !exists {
			return importedLongMemEvalRetrievalRecord{}, fmt.Errorf("imported memory %s is missing from active authority", memoryID)
		}
	}
	return importedLongMemEvalRetrievalRecord{
		ContinuityID: resolution.ContinuityID,
		Occurrences:  occurrences,
		ByMemoryID:   byMemoryID,
		Active:       active,
	}, nil
}

func mapLongMemEvalRetrievedMemories(memories []vermoryruntime.Memory, imported importedLongMemEvalRetrievalRecord) ([]string, []string, error) {
	keys := make([]string, 0, len(memories))
	ids := make([]string, 0, len(memories))
	for _, memory := range memories {
		if _, exists := imported.Active[memory.ID]; !exists {
			return nil, nil, fmt.Errorf("retrieval returned memory %s outside active continuity authority", memory.ID)
		}
		occurrence, exists := imported.ByMemoryID[memory.ID]
		if !exists {
			return nil, nil, fmt.Errorf("retrieval returned unmapped memory %s", memory.ID)
		}
		keys = append(keys, occurrence.Key)
		ids = append(ids, occurrence.RawID)
	}
	return keys, ids, nil
}

func longMemEvalRetrievalOccurrences(record benchmark.LongMemEvalRecord) []longMemEvalSessionOccurrence {
	occurrences := make([]longMemEvalSessionOccurrence, 0, len(record.HaystackSessions))
	for position, turns := range record.HaystackSessions {
		session := benchmark.LongMemEvalSession{
			ID:       record.HaystackSessionIDs[position],
			Date:     record.HaystackDates[position],
			Turns:    append([]benchmark.LongMemEvalTurn(nil), turns...),
			Position: position,
		}
		occurrences = append(occurrences, longMemEvalSessionOccurrence{
			Key:     longMemEvalRetrievalOccurrenceKey(position, session.ID),
			RawID:   session.ID,
			Session: session,
		})
	}
	return occurrences
}

func longMemEvalRetrievalOccurrenceKey(position int, rawID string) string {
	return fmt.Sprintf("%06d:%s", position, rawID)
}

func buildLongMemEvalRetrievalCondition(record benchmark.LongMemEvalRecord, condition string, keys, ids []string, latency time.Duration) (LongMemEvalRetrievalConditionResult, error) {
	result := LongMemEvalRetrievalConditionResult{
		Condition:            condition,
		Status:               "completed",
		RankedOccurrenceKeys: append([]string(nil), keys...),
		RankedSessionIDs:     append([]string(nil), ids...),
		LatencyMilliseconds:  latency.Milliseconds(),
	}
	if strings.HasSuffix(record.QuestionID, "_abs") {
		result.Classification = "abstention_unscored"
		return result, nil
	}
	at5, err := benchmark.EvaluateSessionRetrieval(ids, record.AnswerSessionIDs, 5)
	if err != nil {
		return LongMemEvalRetrievalConditionResult{}, err
	}
	at10, err := benchmark.EvaluateSessionRetrieval(ids, record.AnswerSessionIDs, 10)
	if err != nil {
		return LongMemEvalRetrievalConditionResult{}, err
	}
	at12, err := benchmark.EvaluateSessionRetrieval(ids, record.AnswerSessionIDs, 12)
	if err != nil {
		return LongMemEvalRetrievalConditionResult{}, err
	}
	result.MetricAt5 = &at5
	result.MetricAt10 = &at10
	result.MetricAt12 = &at12
	switch {
	case at12.RecallAll == 1:
		result.Classification = "all_evidence_retrieved"
	case at12.RecallAny == 1:
		result.Classification = "partial_evidence_retrieved"
	default:
		result.Classification = "no_evidence_retrieved"
	}
	return result, nil
}

func longMemEvalRetrievalCheckpointPath(root, runID, recordID string) (string, error) {
	if err := validateLongMemEvalRetrievalSegment(recordID, "record ID"); err != nil {
		return "", err
	}
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	return filepath.Join(absoluteRoot, "benchmarks", runID, "checkpoints", recordID+".json"), nil
}

func validateLongMemEvalRetrievalSegment(value, label string) error {
	value = strings.TrimSpace(value)
	if value == "" || value == "." || value == ".." || filepath.Base(value) != value {
		return fmt.Errorf("invalid LongMemEval retrieval %s %q", label, value)
	}
	return nil
}

func loadLongMemEvalRetrievalCheckpoint(path string) (LongMemEvalRetrievalRecordResult, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return LongMemEvalRetrievalRecordResult{}, err
	}
	var checkpoint LongMemEvalRetrievalRecordResult
	if err := json.Unmarshal(data, &checkpoint); err != nil {
		return LongMemEvalRetrievalRecordResult{}, fmt.Errorf("decode LongMemEval retrieval checkpoint: %w", err)
	}
	return checkpoint, nil
}

func validateLongMemEvalRetrievalCheckpoint(checkpoint LongMemEvalRetrievalRecordResult, runID, implementationRevision string, execution benchmark.ExecutionManifest, recordID string, conditions []string) error {
	if checkpoint.SchemaVersion != "longmemeval-retrieval-checkpoint/v1" {
		return fmt.Errorf("checkpoint schema_version is invalid for record %q", recordID)
	}
	if checkpoint.RunID != runID {
		return fmt.Errorf("checkpoint run_id is %q, want %q", checkpoint.RunID, runID)
	}
	if checkpoint.ImplementationRevision != implementationRevision {
		return fmt.Errorf("checkpoint implementation_revision is %q, want %q", checkpoint.ImplementationRevision, implementationRevision)
	}
	if checkpoint.DatasetSHA256 != execution.DatasetSHA256 {
		return fmt.Errorf("checkpoint dataset_sha256 is %q, want %q", checkpoint.DatasetSHA256, execution.DatasetSHA256)
	}
	if checkpoint.RecordSetSHA256 != execution.RecordSetSHA256 {
		return fmt.Errorf("checkpoint record_set_sha256 is %q, want %q", checkpoint.RecordSetSHA256, execution.RecordSetSHA256)
	}
	if checkpoint.RecordID != recordID {
		return fmt.Errorf("checkpoint record_id is %q, want %q", checkpoint.RecordID, recordID)
	}
	checkpointConditions := make([]string, 0, len(checkpoint.Conditions))
	for _, condition := range checkpoint.Conditions {
		checkpointConditions = append(checkpointConditions, condition.Condition)
	}
	if checkpoint.Status == "completed" && !slices.Equal(checkpointConditions, conditions) {
		return fmt.Errorf("checkpoint conditions are %v, want %v", checkpointConditions, conditions)
	}
	return nil
}

func finalizeLongMemEvalRetrievalReport(runID, implementationRevision string, qualification benchmark.Qualification, execution benchmark.ExecutionManifest, summary benchmark.LongMemEvalSummary, results []LongMemEvalRetrievalRecordResult) LongMemEvalRetrievalReport {
	sort.Slice(results, func(i, j int) bool { return results[i].RecordID < results[j].RecordID })
	report := LongMemEvalRetrievalReport{
		RunID:                  runID,
		Benchmark:              execution.Benchmark,
		EvaluationTarget:       execution.EvaluationTarget,
		ExecutionScope:         execution.ExecutionScope,
		ClaimScope:             execution.ClaimScope,
		DatasetSHA256:          execution.DatasetSHA256,
		RecordSetSHA256:        execution.RecordSetSHA256,
		ImplementationRevision: implementationRevision,
		SourceSummary:          summary,
		RecordCount:            len(results),
		Conditions:             append([]string(nil), execution.Conditions...),
		Results:                results,
		Aggregates:             make(map[string]LongMemEvalRetrievalAggregate),
		QuestionTypeAggregates: make(map[string]map[string]LongMemEvalRetrievalAggregate),
		Artifacts:              make(map[string]string),
		NonClaims:              append([]string(nil), execution.NonClaims...),
	}
	allMetrics := make(map[string]map[int][]benchmark.SessionRetrievalMetric)
	byType := make(map[string]map[string]map[int][]benchmark.SessionRetrievalMetric)
	for _, result := range results {
		report.ImportedMemoryCount += result.ImportedMemoryCount
		if result.Status != "completed" {
			report.Failures = append(report.Failures, LongMemEvalRetrievalFailure{RecordID: result.RecordID, QuestionType: result.QuestionType, Error: result.Error})
			continue
		}
		for _, condition := range result.Conditions {
			if condition.Status != "completed" {
				report.Failures = append(report.Failures, LongMemEvalRetrievalFailure{
					RecordID: result.RecordID, QuestionType: result.QuestionType,
					Condition: condition.Condition,
					Error:     fmt.Sprintf("condition status=%s effective=%s degraded=%t failure=%s", condition.Status, condition.EffectiveMode, condition.Degraded, condition.FailureCode),
				})
			}
		}
		if result.Abstention {
			continue
		}
		recordComplete := true
		for _, condition := range result.Conditions {
			if condition.Status != "completed" || condition.MetricAt5 == nil || condition.MetricAt10 == nil || condition.MetricAt12 == nil {
				recordComplete = false
				continue
			}
			if allMetrics[condition.Condition] == nil {
				allMetrics[condition.Condition] = make(map[int][]benchmark.SessionRetrievalMetric)
			}
			allMetrics[condition.Condition][5] = append(allMetrics[condition.Condition][5], *condition.MetricAt5)
			allMetrics[condition.Condition][10] = append(allMetrics[condition.Condition][10], *condition.MetricAt10)
			allMetrics[condition.Condition][12] = append(allMetrics[condition.Condition][12], *condition.MetricAt12)
			if byType[result.QuestionType] == nil {
				byType[result.QuestionType] = make(map[string]map[int][]benchmark.SessionRetrievalMetric)
			}
			if byType[result.QuestionType][condition.Condition] == nil {
				byType[result.QuestionType][condition.Condition] = make(map[int][]benchmark.SessionRetrievalMetric)
			}
			byType[result.QuestionType][condition.Condition][5] = append(byType[result.QuestionType][condition.Condition][5], *condition.MetricAt5)
			byType[result.QuestionType][condition.Condition][10] = append(byType[result.QuestionType][condition.Condition][10], *condition.MetricAt10)
			byType[result.QuestionType][condition.Condition][12] = append(byType[result.QuestionType][condition.Condition][12], *condition.MetricAt12)
		}
		if recordComplete && len(result.Conditions) == len(report.Conditions) {
			report.ScoredRecordCount++
		}
	}
	for _, condition := range report.Conditions {
		report.Aggregates[condition] = aggregateLongMemEvalRetrieval(allMetrics[condition])
	}
	for questionType, conditions := range byType {
		report.QuestionTypeAggregates[questionType] = make(map[string]LongMemEvalRetrievalAggregate)
		for _, condition := range report.Conditions {
			report.QuestionTypeAggregates[questionType][condition] = aggregateLongMemEvalRetrieval(conditions[condition])
		}
	}
	_ = qualification
	return report
}

func aggregateLongMemEvalRetrieval(metrics map[int][]benchmark.SessionRetrievalMetric) LongMemEvalRetrievalAggregate {
	return LongMemEvalRetrievalAggregate{
		Count: len(metrics[10]),
		At5:   benchmark.AggregateSessionRetrieval(metrics[5]),
		At10:  benchmark.AggregateSessionRetrieval(metrics[10]),
		At12:  benchmark.AggregateSessionRetrieval(metrics[12]),
	}
}

func writeLongMemEvalRetrievalArtifacts(ctx context.Context, store *artifact.LocalStore, artifactRoot, prefix string, qualification benchmark.Qualification, execution benchmark.ExecutionManifest, report *LongMemEvalRetrievalReport) error {
	sourceURI, err := putJSONArtifact(ctx, store, filepath.ToSlash(filepath.Join(prefix, "source.json")), map[string]any{
		"qualification": qualification,
		"execution":     execution,
		"summary":       report.SourceSummary,
	})
	if err != nil {
		return err
	}
	report.Artifacts["source"] = sourceURI

	var jsonl strings.Builder
	for _, result := range report.Results {
		line, err := json.Marshal(result)
		if err != nil {
			return err
		}
		jsonl.Write(line)
		jsonl.WriteByte('\n')
	}
	resultsURI, err := putTextArtifact(ctx, store, filepath.ToSlash(filepath.Join(prefix, "retrieval-results.jsonl")), jsonl.String())
	if err != nil {
		return err
	}
	report.Artifacts["retrieval_results"] = resultsURI
	scoresURI, err := putJSONArtifact(ctx, store, filepath.ToSlash(filepath.Join(prefix, "scores.json")), map[string]any{
		"run_id":                   report.RunID,
		"evaluation_target":        report.EvaluationTarget,
		"claim_scope":              report.ClaimScope,
		"source_summary":           report.SourceSummary,
		"aggregates":               report.Aggregates,
		"question_type_aggregates": report.QuestionTypeAggregates,
		"vector":                   report.Vector,
		"results":                  report.Results,
	})
	if err != nil {
		return err
	}
	report.Artifacts["scores"] = scoresURI
	failuresURI, err := putJSONArtifact(ctx, store, filepath.ToSlash(filepath.Join(prefix, "failure-ledger.json")), report.Failures)
	if err != nil {
		return err
	}
	report.Artifacts["failure_ledger"] = failuresURI
	reportURI, err := putTextArtifact(ctx, store, filepath.ToSlash(filepath.Join(prefix, "report.md")), markdownLongMemEvalRetrievalReport(*report))
	if err != nil {
		return err
	}
	report.Artifacts["report"] = reportURI

	finalExecution := execution
	finalExecution.RunID = report.RunID
	finalExecution.ImplementationRev = report.ImplementationRevision
	finalExecution.Artifacts = copyStringMap(report.Artifacts)
	manifestURI, err := localArtifactURI(artifactRoot, filepath.ToSlash(filepath.Join(prefix, "execution-manifest.json")))
	if err != nil {
		return err
	}
	finalExecution.Artifacts["execution_manifest"] = manifestURI
	if err := benchmark.ValidateExecution(qualification, finalExecution); err != nil {
		return err
	}
	manifestURI, err = putJSONArtifact(ctx, store, filepath.ToSlash(filepath.Join(prefix, "execution-manifest.json")), finalExecution)
	if err != nil {
		return err
	}
	report.Artifacts["execution_manifest"] = manifestURI
	_, err = putJSONArtifact(ctx, store, filepath.ToSlash(filepath.Join(prefix, "report.json")), report)
	return err
}

func markdownLongMemEvalRetrievalReport(report LongMemEvalRetrievalReport) string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "# LongMemEval-S Full Retrieval Qualification\n\n")
	fmt.Fprintf(&builder, "- Run ID: `%s`\n", report.RunID)
	fmt.Fprintf(&builder, "- Scope: `%s` / `%s`\n", report.ExecutionScope, report.ClaimScope)
	fmt.Fprintf(&builder, "- Records: `%d` total / `%d` scored\n", report.RecordCount, report.ScoredRecordCount)
	fmt.Fprintf(&builder, "- Imported governed memories: `%d`\n", report.ImportedMemoryCount)
	fmt.Fprintf(&builder, "- Runtime failures: `%d`\n\n", len(report.Failures))
	if report.Vector != nil {
		fmt.Fprintf(&builder, "- Vector profile: `%s` (`%s`)\n", report.Vector.RetrievalProfile, report.Vector.ProfileSHA256)
		fmt.Fprintf(&builder, "- Embedding retry contract: timeout=`%ds`, attempts=`%d`, delay=`%dms`, backoff=`%s`\n", report.Vector.HTTPTimeoutSeconds, report.Vector.MaxAttempts, report.Vector.RetryDelayMilliseconds, report.Vector.RetryBackoff)
		fmt.Fprintf(&builder, "- Projection recovery contract: recoveries=`%d`, cooldown=`%ds`\n", report.Vector.ProjectionMaxRecoveries, report.Vector.ProjectionRecoveryDelaySeconds)
		fmt.Fprintf(&builder, "- Projection: status=`%s`, lag=`%d`, vectors=`%d`\n", report.Vector.Projection.Status, report.Vector.Projection.Lag, report.Vector.Projection.VectorCount)
		fmt.Fprintf(&builder, "- Projection failures: recovered=`%d`, unrecovered=`%d`, cooldowns=`%d`\n", report.Vector.RecoveredProjectionFailures, report.Vector.UnrecoveredProjectionFailures, report.Vector.ProjectionRecoverySleeps)
		fmt.Fprintf(&builder, "- Vector queries: effective=`%d`, degraded=`%d`\n", report.Vector.EffectiveVectorQueries, report.Vector.DegradedVectorQueries)
		fmt.Fprintf(&builder, "- Embedding: operations=`%d`, attempts=`%d`, successful items=`%d`, failed attempts=`%d`, terminal failures=`%d`\n", report.Vector.Embedding.LogicalOperations, report.Vector.Embedding.ProviderAttempts, report.Vector.Embedding.SuccessfulItems, report.Vector.Embedding.FailedAttempts, report.Vector.Embedding.TerminalFailures)
		fmt.Fprintf(&builder, "- Vector hard gates: `%t`\n\n", report.Vector.HardGatesPass)
	}
	builder.WriteString("## Aggregate Retrieval\n\n")
	builder.WriteString("| Condition | K | Recall any | Recall all | nDCG | MRR |\n")
	builder.WriteString("|---|---:|---:|---:|---:|---:|\n")
	for _, condition := range report.Conditions {
		aggregate := report.Aggregates[condition]
		for _, row := range []struct {
			k int
			a benchmark.RetrievalAggregate
		}{{5, aggregate.At5}, {10, aggregate.At10}, {12, aggregate.At12}} {
			fmt.Fprintf(&builder, "| `%s` | %d | %.4f | %.4f | %.4f | %.4f |\n", condition, row.k, row.a.MeanRecallAny, row.a.MeanRecallAll, row.a.MeanNDCG, row.a.MeanMRR)
		}
	}
	builder.WriteString("\n## Non-Claims\n\n")
	for _, nonClaim := range report.NonClaims {
		builder.WriteString("- " + nonClaim + "\n")
	}
	return builder.String()
}
