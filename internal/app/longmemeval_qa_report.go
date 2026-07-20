package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"vermory/internal/artifact"
	"vermory/internal/benchmark"
)

type LongMemEvalQAUsageAggregate struct {
	AvailableAttempts int `json:"available_attempts"`
	MissingAttempts   int `json:"missing_attempts"`
	InputTokens       int `json:"input_tokens"`
	CachedInputTokens int `json:"cached_input_tokens"`
	OutputTokens      int `json:"output_tokens"`
	ReasoningTokens   int `json:"reasoning_tokens"`
	TotalTokens       int `json:"total_tokens"`
}

type LongMemEvalQALatencyAggregate struct {
	Count       int   `json:"count"`
	TotalMillis int64 `json:"total_ms"`
	P50Millis   int64 `json:"p50_ms"`
	P95Millis   int64 `json:"p95_ms"`
	P99Millis   int64 `json:"p99_ms"`
	MaxMillis   int64 `json:"max_ms"`
}

type LongMemEvalQAConditionAggregate struct {
	Total                     int                           `json:"total"`
	Completed                 int                           `json:"completed"`
	ReaderFailed              int                           `json:"reader_failed"`
	Judged                    int                           `json:"judged"`
	JudgeFailed               int                           `json:"judge_failed"`
	JudgeInvalid              int                           `json:"judge_invalid"`
	JudgeNotRun               int                           `json:"judge_not_run"`
	JudgeCorrect              int                           `json:"judge_correct"`
	ExactMatches              int                           `json:"exact_matches"`
	MeanTokenF1               float64                       `json:"mean_token_f1"`
	MeanAnswerTokenRecall     float64                       `json:"mean_answer_token_recall"`
	OverallJudgeAccuracy      float64                       `json:"overall_judge_accuracy"`
	JudgedAccuracy            float64                       `json:"judged_accuracy"`
	TaskAveragedJudgeAccuracy float64                       `json:"task_averaged_judge_accuracy"`
	AbstentionExpected        int                           `json:"abstention_expected"`
	AbstentionCorrect         int                           `json:"abstention_correct"`
	AbstentionAccuracy        float64                       `json:"abstention_accuracy"`
	ReaderLatency             LongMemEvalQALatencyAggregate `json:"reader_latency"`
	JudgeLatency              LongMemEvalQALatencyAggregate `json:"judge_latency"`
	Usage                     LongMemEvalQAUsageAggregate   `json:"usage"`
	JudgeUsage                LongMemEvalQAUsageAggregate   `json:"judge_usage"`
}

type LongMemEvalQARetrievalClassAggregate struct {
	Total    int     `json:"total"`
	Judged   int     `json:"judged"`
	Correct  int     `json:"correct"`
	Accuracy float64 `json:"accuracy"`
}

type LongMemEvalQAPairedAggregate struct {
	FirstCondition     string `json:"first_condition"`
	SecondCondition    string `json:"second_condition"`
	Eligible           int    `json:"eligible"`
	BothCorrect        int    `json:"both_correct"`
	FirstOnlyCorrect   int    `json:"first_only_correct"`
	SecondOnlyCorrect  int    `json:"second_only_correct"`
	PlainOnlyCorrect   int    `json:"plain_only_correct,omitempty"`
	VermoryOnlyCorrect int    `json:"vermory_only_correct,omitempty"`
	NeitherCorrect     int    `json:"neither_correct"`
	Incomplete         int    `json:"incomplete"`
}

type LongMemEvalQAFailure struct {
	RecordID                 string `json:"record_id"`
	QuestionType             string `json:"question_type"`
	Condition                string `json:"condition"`
	Primary                  string `json:"primary"`
	ReaderStatus             string `json:"reader_status"`
	JudgeStatus              string `json:"judge_status"`
	RetrievalClassification  string `json:"retrieval_classification"`
	Error                    string `json:"error,omitempty"`
	ResponseExcerpt          string `json:"response_excerpt,omitempty"`
	SourceLifecycleCandidate bool   `json:"source_lifecycle_candidate"`
}

type LongMemEvalQAReport struct {
	RunID                  string                                                     `json:"run_id"`
	Benchmark              string                                                     `json:"benchmark"`
	ExecutionScope         benchmark.ExecutionScope                                   `json:"execution_scope"`
	ClaimScope             benchmark.ClaimScope                                       `json:"claim_scope"`
	DatasetSHA256          string                                                     `json:"dataset_sha256"`
	RecordSetSHA256        string                                                     `json:"record_set_sha256"`
	RetrievalSHA256        string                                                     `json:"retrieval_sha256"`
	ImplementationRevision string                                                     `json:"implementation_revision"`
	SourceSummary          benchmark.LongMemEvalSummary                               `json:"source_summary"`
	RecordCount            int                                                        `json:"record_count"`
	TaskCount              int                                                        `json:"task_count"`
	Reader                 benchmark.ExecutionModelConfig                             `json:"reader"`
	Judge                  benchmark.ExecutionModelConfig                             `json:"judge"`
	Conditions             map[string]LongMemEvalQAConditionAggregate                 `json:"conditions"`
	QuestionTypes          map[string]map[string]LongMemEvalQAConditionAggregate      `json:"question_types"`
	RetrievalClasses       map[string]map[string]LongMemEvalQARetrievalClassAggregate `json:"retrieval_classes"`
	Paired                 LongMemEvalQAPairedAggregate                               `json:"paired"`
	JudgeDisagreements     int                                                        `json:"judge_deterministic_disagreements"`
	Failures               []LongMemEvalQAFailure                                     `json:"failures"`
	Artifacts              map[string]string                                          `json:"artifacts,omitempty"`
	NonClaims              []string                                                   `json:"non_claims"`
}

type longMemEvalQAConditionBuilder struct {
	aggregate       LongMemEvalQAConditionAggregate
	tokenF1         float64
	answerRecall    float64
	readerLatencies []int64
	judgeLatencies  []int64
}

type longMemEvalQAPairState struct {
	first  *bool
	second *bool
}

type longMemEvalQASafeScore struct {
	ExactMatch         bool    `json:"exact_match"`
	TokenF1            float64 `json:"token_f1"`
	AnswerTokenRecall  float64 `json:"answer_token_recall"`
	AbstentionExpected bool    `json:"abstention_expected"`
	AbstentionDetected bool    `json:"abstention_detected"`
}

type longMemEvalQAReaderResult struct {
	RecordID                string                      `json:"record_id"`
	QuestionType            string                      `json:"question_type"`
	Abstention              bool                        `json:"abstention"`
	Condition               string                      `json:"condition"`
	RetrievalClassification string                      `json:"retrieval_classification"`
	Status                  string                      `json:"status"`
	Response                string                      `json:"response,omitempty"`
	ProviderModel           string                      `json:"provider_model,omitempty"`
	AttemptCount            int                         `json:"attempt_count"`
	LatencyMillis           int64                       `json:"latency_ms"`
	Usage                   LongMemEvalQAUsageAggregate `json:"usage"`
	Score                   *longMemEvalQASafeScore     `json:"score,omitempty"`
}

type longMemEvalQAJudgeResult struct {
	RecordID      string                      `json:"record_id"`
	QuestionType  string                      `json:"question_type"`
	Abstention    bool                        `json:"abstention"`
	Condition     string                      `json:"condition"`
	Status        string                      `json:"status"`
	Correct       *bool                       `json:"correct,omitempty"`
	Output        string                      `json:"output,omitempty"`
	ProviderModel string                      `json:"provider_model,omitempty"`
	AttemptCount  int                         `json:"attempt_count"`
	LatencyMillis int64                       `json:"latency_ms"`
	Usage         LongMemEvalQAUsageAggregate `json:"usage"`
}

func FinalizeLongMemEvalQA(opts LongMemEvalQAOptions) (LongMemEvalQAReport, error) {
	opts, contract, retrieval, sourcePath, err := prepareLongMemEvalQAInputs(opts)
	if err != nil {
		return LongMemEvalQAReport{}, err
	}
	sourceSummary, err := benchmark.ScanLongMemEval(sourcePath, nil)
	if err != nil {
		return LongMemEvalQAReport{}, err
	}
	if err := validateLongMemEvalRetrievalSummary(opts.Qualification, opts.Execution, sourceSummary); err != nil {
		return LongMemEvalQAReport{}, err
	}
	if len(retrieval) != opts.Qualification.Dataset.RecordCount {
		return LongMemEvalQAReport{}, fmt.Errorf("retrieval input contains %d records, want %d", len(retrieval), opts.Qualification.Dataset.RecordCount)
	}

	expectedTasks := opts.Qualification.Dataset.RecordCount * len(opts.Execution.Conditions)
	checkpoints := make([]LongMemEvalQACheckpoint, 0, expectedTasks)
	seen := make(map[string]struct{}, len(retrieval))
	_, err = benchmark.ScanLongMemEval(sourcePath, func(record benchmark.LongMemEvalRecord) error {
		result, exists := retrieval[record.QuestionID]
		if !exists {
			return fmt.Errorf("retrieval input is missing record %q", record.QuestionID)
		}
		seen[record.QuestionID] = struct{}{}
		tasks, err := BuildLongMemEvalQATasks(record, result, contract.K)
		if err != nil {
			return err
		}
		judgeRecord := benchmark.LongMemEvalRecord{QuestionID: record.QuestionID, QuestionType: record.QuestionType, Question: record.Question, Answer: record.Answer}
		for _, task := range tasks {
			path, err := longMemEvalQACheckpointPath(opts.ArtifactRoot, contract.RunID, task.RecordID, task.Condition)
			if err != nil {
				return err
			}
			checkpoint, err := loadLongMemEvalQACheckpoint(path)
			if err != nil {
				if errors.Is(err, os.ErrNotExist) {
					return fmt.Errorf("LongMemEval QA checkpoint is missing for record %q condition %q", task.RecordID, task.Condition)
				}
				return err
			}
			if err := validateLongMemEvalQACheckpoint(checkpoint, task, contract); err != nil {
				return err
			}
			if checkpoint.Judge == nil {
				return fmt.Errorf("LongMemEval QA judge state is missing for record %q condition %q", task.RecordID, task.Condition)
			}
			expectedPromptSHA256, err := longMemEvalQAJudgePromptSHA(judgeRecord, checkpoint)
			if err != nil {
				return err
			}
			if err := validateLongMemEvalQAJudgeState(*checkpoint.Judge, checkpoint.ReaderStatus, *opts.Execution.Judge, expectedPromptSHA256); err != nil {
				return err
			}
			checkpoints = append(checkpoints, checkpoint)
		}
		return nil
	})
	if err != nil {
		return LongMemEvalQAReport{}, err
	}
	if len(seen) != len(retrieval) {
		return LongMemEvalQAReport{}, fmt.Errorf("retrieval input contains records outside the qualified source")
	}
	if len(checkpoints) != expectedTasks {
		return LongMemEvalQAReport{}, fmt.Errorf("LongMemEval QA has %d checkpoints, want %d", len(checkpoints), expectedTasks)
	}
	report, err := aggregateLongMemEvalQA(opts, sourceSummary, checkpoints)
	if err != nil {
		return LongMemEvalQAReport{}, err
	}
	report.ImplementationRevision = contract.ImplementationRevision
	if err := writeLongMemEvalQAArtifacts(context.Background(), opts, checkpoints, &report); err != nil {
		return LongMemEvalQAReport{}, err
	}
	return report, nil
}

func aggregateLongMemEvalQA(opts LongMemEvalQAOptions, sourceSummary benchmark.LongMemEvalSummary, checkpoints []LongMemEvalQACheckpoint) (LongMemEvalQAReport, error) {
	if opts.Execution.Reader == nil || opts.Execution.Judge == nil || opts.Execution.RetrievalInput == nil {
		return LongMemEvalQAReport{}, fmt.Errorf("LongMemEval QA execution configs are incomplete")
	}
	if len(opts.Execution.Conditions) != 2 {
		return LongMemEvalQAReport{}, fmt.Errorf("LongMemEval QA requires exactly two conditions")
	}
	expectedTasks := sourceSummary.RecordCount * len(opts.Execution.Conditions)
	if len(checkpoints) != expectedTasks {
		return LongMemEvalQAReport{}, fmt.Errorf("LongMemEval QA has %d checkpoints, want %d", len(checkpoints), expectedTasks)
	}
	sorted := append([]LongMemEvalQACheckpoint(nil), checkpoints...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].RecordID == sorted[j].RecordID {
			return sorted[i].Condition < sorted[j].Condition
		}
		return sorted[i].RecordID < sorted[j].RecordID
	})
	conditionBuilders := make(map[string]*longMemEvalQAConditionBuilder, len(opts.Execution.Conditions))
	typeBuilders := make(map[string]map[string]*longMemEvalQAConditionBuilder)
	retrievalClasses := make(map[string]map[string]LongMemEvalQARetrievalClassAggregate, len(opts.Execution.Conditions))
	for _, condition := range opts.Execution.Conditions {
		conditionBuilders[condition] = &longMemEvalQAConditionBuilder{}
		retrievalClasses[condition] = make(map[string]LongMemEvalQARetrievalClassAggregate)
	}
	pairs := make(map[string]*longMemEvalQAPairState, sourceSummary.RecordCount)
	seen := make(map[string]struct{}, len(sorted))
	failures := make([]LongMemEvalQAFailure, 0)
	disagreements := 0
	for _, checkpoint := range sorted {
		identity := checkpoint.RecordID + "\x00" + checkpoint.Condition
		if _, exists := seen[identity]; exists {
			return LongMemEvalQAReport{}, fmt.Errorf("duplicate LongMemEval QA checkpoint %s/%s", checkpoint.RecordID, checkpoint.Condition)
		}
		seen[identity] = struct{}{}
		builder, exists := conditionBuilders[checkpoint.Condition]
		if !exists {
			return LongMemEvalQAReport{}, fmt.Errorf("unsupported LongMemEval QA condition %q", checkpoint.Condition)
		}
		if typeBuilders[checkpoint.QuestionType] == nil {
			typeBuilders[checkpoint.QuestionType] = make(map[string]*longMemEvalQAConditionBuilder, len(opts.Execution.Conditions))
			for _, condition := range opts.Execution.Conditions {
				typeBuilders[checkpoint.QuestionType][condition] = &longMemEvalQAConditionBuilder{}
			}
		}
		accumulateLongMemEvalQACondition(builder, checkpoint)
		accumulateLongMemEvalQACondition(typeBuilders[checkpoint.QuestionType][checkpoint.Condition], checkpoint)
		classAggregate := retrievalClasses[checkpoint.Condition][checkpoint.RetrievalClassification]
		classAggregate.Total++
		if checkpoint.Judge != nil && checkpoint.Judge.Status == longMemEvalQAJudgeCompleted {
			classAggregate.Judged++
			if *checkpoint.Judge.Correct {
				classAggregate.Correct++
			}
			if checkpoint.Score != nil && checkpoint.Score.ExactMatch != *checkpoint.Judge.Correct {
				disagreements++
			}
		}
		retrievalClasses[checkpoint.Condition][checkpoint.RetrievalClassification] = classAggregate

		pair := pairs[checkpoint.RecordID]
		if pair == nil {
			pair = &longMemEvalQAPairState{}
			pairs[checkpoint.RecordID] = pair
		}
		if checkpoint.Judge != nil && checkpoint.Judge.Status == longMemEvalQAJudgeCompleted {
			correct := *checkpoint.Judge.Correct
			if checkpoint.Condition == opts.Execution.Conditions[0] {
				pair.first = &correct
			} else if checkpoint.Condition == opts.Execution.Conditions[1] {
				pair.second = &correct
			}
		}
		if failure, exists := longMemEvalQAFailureForCheckpoint(checkpoint); exists {
			failures = append(failures, failure)
		}
	}
	if len(pairs) != sourceSummary.RecordCount {
		return LongMemEvalQAReport{}, fmt.Errorf("LongMemEval QA checkpoints represent %d records, want %d", len(pairs), sourceSummary.RecordCount)
	}

	conditions := make(map[string]LongMemEvalQAConditionAggregate, len(conditionBuilders))
	questionTypes := make(map[string]map[string]LongMemEvalQAConditionAggregate, len(typeBuilders))
	for questionType, builders := range typeBuilders {
		questionTypes[questionType] = make(map[string]LongMemEvalQAConditionAggregate, len(builders))
		for condition, builder := range builders {
			questionTypes[questionType][condition] = finalizeLongMemEvalQACondition(builder)
		}
	}
	for condition, builder := range conditionBuilders {
		aggregate := finalizeLongMemEvalQACondition(builder)
		var taskAccuracy float64
		var taskCount int
		for _, byCondition := range questionTypes {
			value := byCondition[condition]
			if value.Total != 0 {
				taskAccuracy += float64(value.JudgeCorrect) / float64(value.Total)
				taskCount++
			}
		}
		if taskCount != 0 {
			aggregate.TaskAveragedJudgeAccuracy = roundBenchmarkMetric(taskAccuracy / float64(taskCount))
		}
		conditions[condition] = aggregate
	}
	for condition, classes := range retrievalClasses {
		for classification, aggregate := range classes {
			if aggregate.Total != 0 {
				aggregate.Accuracy = roundBenchmarkMetric(float64(aggregate.Correct) / float64(aggregate.Total))
			}
			classes[classification] = aggregate
		}
		retrievalClasses[condition] = classes
	}
	paired := LongMemEvalQAPairedAggregate{
		FirstCondition:  opts.Execution.Conditions[0],
		SecondCondition: opts.Execution.Conditions[1],
	}
	for _, pair := range pairs {
		if pair.first == nil || pair.second == nil {
			paired.Incomplete++
			continue
		}
		paired.Eligible++
		switch {
		case *pair.first && *pair.second:
			paired.BothCorrect++
		case *pair.first:
			paired.FirstOnlyCorrect++
		case *pair.second:
			paired.SecondOnlyCorrect++
		default:
			paired.NeitherCorrect++
		}
	}
	if paired.FirstCondition == longMemEvalQAPlainCondition && paired.SecondCondition == longMemEvalQAVermoryCondition {
		paired.PlainOnlyCorrect = paired.FirstOnlyCorrect
		paired.VermoryOnlyCorrect = paired.SecondOnlyCorrect
	}
	sort.Slice(failures, func(i, j int) bool {
		if failures[i].RecordID == failures[j].RecordID {
			return failures[i].Condition < failures[j].Condition
		}
		return failures[i].RecordID < failures[j].RecordID
	})
	return LongMemEvalQAReport{
		RunID:              opts.RunID,
		Benchmark:          opts.Execution.Benchmark,
		ExecutionScope:     opts.Execution.ExecutionScope,
		ClaimScope:         opts.Execution.ClaimScope,
		DatasetSHA256:      opts.Execution.DatasetSHA256,
		RecordSetSHA256:    opts.Execution.RecordSetSHA256,
		RetrievalSHA256:    opts.Execution.RetrievalInput.SHA256,
		SourceSummary:      sourceSummary,
		RecordCount:        sourceSummary.RecordCount,
		TaskCount:          len(sorted),
		Reader:             *opts.Execution.Reader,
		Judge:              *opts.Execution.Judge,
		Conditions:         conditions,
		QuestionTypes:      questionTypes,
		RetrievalClasses:   retrievalClasses,
		Paired:             paired,
		JudgeDisagreements: disagreements,
		Failures:           failures,
		NonClaims:          append([]string(nil), opts.Execution.NonClaims...),
	}, nil
}

func accumulateLongMemEvalQACondition(builder *longMemEvalQAConditionBuilder, checkpoint LongMemEvalQACheckpoint) {
	aggregate := &builder.aggregate
	aggregate.Total++
	readerLatency := longMemEvalQAAttemptsLatency(checkpoint.Attempts)
	builder.readerLatencies = append(builder.readerLatencies, readerLatency)
	addLongMemEvalQAUsage(&aggregate.Usage, checkpoint.Attempts)
	if checkpoint.ReaderStatus == longMemEvalQAReaderCompleted {
		aggregate.Completed++
		if checkpoint.Score != nil {
			if checkpoint.Score.ExactMatch {
				aggregate.ExactMatches++
			}
			builder.tokenF1 += checkpoint.Score.TokenF1
			builder.answerRecall += checkpoint.Score.AnswerTokenRecall
		}
	} else {
		aggregate.ReaderFailed++
	}
	if checkpoint.Abstention {
		aggregate.AbstentionExpected++
	}
	if checkpoint.Judge == nil {
		return
	}
	judgeLatency := longMemEvalQAAttemptsLatency(checkpoint.Judge.Attempts)
	if len(checkpoint.Judge.Attempts) != 0 {
		builder.judgeLatencies = append(builder.judgeLatencies, judgeLatency)
	}
	addLongMemEvalQAUsage(&aggregate.JudgeUsage, checkpoint.Judge.Attempts)
	switch checkpoint.Judge.Status {
	case longMemEvalQAJudgeCompleted:
		aggregate.Judged++
		if *checkpoint.Judge.Correct {
			aggregate.JudgeCorrect++
			if checkpoint.Abstention {
				aggregate.AbstentionCorrect++
			}
		}
	case longMemEvalQAJudgeFailed:
		aggregate.JudgeFailed++
	case longMemEvalQAJudgeInvalid:
		aggregate.JudgeInvalid++
	case longMemEvalQAJudgeNotRunReaderFailed:
		aggregate.JudgeNotRun++
	}
}

func finalizeLongMemEvalQACondition(builder *longMemEvalQAConditionBuilder) LongMemEvalQAConditionAggregate {
	aggregate := builder.aggregate
	if aggregate.Completed != 0 {
		aggregate.MeanTokenF1 = roundBenchmarkMetric(builder.tokenF1 / float64(aggregate.Completed))
		aggregate.MeanAnswerTokenRecall = roundBenchmarkMetric(builder.answerRecall / float64(aggregate.Completed))
	}
	if aggregate.Total != 0 {
		aggregate.OverallJudgeAccuracy = roundBenchmarkMetric(float64(aggregate.JudgeCorrect) / float64(aggregate.Total))
	}
	if aggregate.Judged != 0 {
		aggregate.JudgedAccuracy = roundBenchmarkMetric(float64(aggregate.JudgeCorrect) / float64(aggregate.Judged))
	}
	if aggregate.AbstentionExpected != 0 {
		aggregate.AbstentionAccuracy = roundBenchmarkMetric(float64(aggregate.AbstentionCorrect) / float64(aggregate.AbstentionExpected))
	}
	aggregate.ReaderLatency = aggregateLongMemEvalQALatencies(builder.readerLatencies)
	aggregate.JudgeLatency = aggregateLongMemEvalQALatencies(builder.judgeLatencies)
	return aggregate
}

func aggregateLongMemEvalQALatencies(values []int64) LongMemEvalQALatencyAggregate {
	if len(values) == 0 {
		return LongMemEvalQALatencyAggregate{}
	}
	sorted := append([]int64(nil), values...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	var total int64
	for _, value := range sorted {
		total += value
	}
	return LongMemEvalQALatencyAggregate{
		Count:       len(sorted),
		TotalMillis: total,
		P50Millis:   longMemEvalQAPercentile(sorted, 0.50),
		P95Millis:   longMemEvalQAPercentile(sorted, 0.95),
		P99Millis:   longMemEvalQAPercentile(sorted, 0.99),
		MaxMillis:   sorted[len(sorted)-1],
	}
}

func longMemEvalQAPercentile(sorted []int64, percentile float64) int64 {
	index := int(math.Ceil(percentile*float64(len(sorted)))) - 1
	if index < 0 {
		index = 0
	}
	if index >= len(sorted) {
		index = len(sorted) - 1
	}
	return sorted[index]
}

func addLongMemEvalQAUsage(aggregate *LongMemEvalQAUsageAggregate, attempts []LongMemEvalQAAttempt) {
	for _, attempt := range attempts {
		if attempt.Usage == nil {
			aggregate.MissingAttempts++
			continue
		}
		aggregate.AvailableAttempts++
		aggregate.InputTokens += attempt.Usage.InputTokens
		aggregate.CachedInputTokens += attempt.Usage.CachedInputTokens
		aggregate.OutputTokens += attempt.Usage.OutputTokens
		aggregate.ReasoningTokens += attempt.Usage.ReasoningTokens
		aggregate.TotalTokens += attempt.Usage.TotalTokens
	}
}

func longMemEvalQAAttemptsLatency(attempts []LongMemEvalQAAttempt) int64 {
	var total int64
	for _, attempt := range attempts {
		total += attempt.DurationMillis
	}
	return total
}

func longMemEvalQAFailureForCheckpoint(checkpoint LongMemEvalQACheckpoint) (LongMemEvalQAFailure, bool) {
	failure := LongMemEvalQAFailure{
		RecordID:                checkpoint.RecordID,
		QuestionType:            checkpoint.QuestionType,
		Condition:               checkpoint.Condition,
		ReaderStatus:            checkpoint.ReaderStatus,
		RetrievalClassification: checkpoint.RetrievalClassification,
		ResponseExcerpt:         truncateLongMemEvalQAExcerpt(checkpoint.Response, 500),
	}
	if checkpoint.Judge != nil {
		failure.JudgeStatus = checkpoint.Judge.Status
	}
	if checkpoint.ReaderStatus == longMemEvalQAReaderFailed {
		failure.Primary = "reader_runtime_failure"
		failure.Error = lastLongMemEvalQAAttemptError(checkpoint.Attempts)
		return failure, true
	}
	if checkpoint.Judge == nil {
		return failure, false
	}
	switch checkpoint.Judge.Status {
	case longMemEvalQAJudgeFailed, longMemEvalQAJudgeInvalid:
		failure.Primary = "judge_failure"
		failure.Error = lastLongMemEvalQAAttemptError(checkpoint.Judge.Attempts)
		return failure, true
	case longMemEvalQAJudgeCompleted:
		if *checkpoint.Judge.Correct {
			return LongMemEvalQAFailure{}, false
		}
	default:
		return LongMemEvalQAFailure{}, false
	}
	if checkpoint.Abstention {
		failure.Primary = "abstention_failure"
		return failure, true
	}
	switch checkpoint.RetrievalClassification {
	case "no_evidence_retrieved":
		failure.Primary = "retrieval_no_evidence"
	case "partial_evidence_retrieved":
		failure.Primary = "retrieval_partial_evidence"
	case "all_evidence_retrieved":
		failure.Primary = "reader_or_aggregation_failure"
		failure.SourceLifecycleCandidate = checkpoint.QuestionType == "knowledge-update"
	default:
		failure.Primary = "reader_or_aggregation_failure"
	}
	return failure, true
}

func lastLongMemEvalQAAttemptError(attempts []LongMemEvalQAAttempt) string {
	for index := len(attempts) - 1; index >= 0; index-- {
		if strings.TrimSpace(attempts[index].Error) != "" {
			return attempts[index].Error
		}
	}
	return ""
}

func truncateLongMemEvalQAExcerpt(value string, limit int) string {
	value = strings.TrimSpace(value)
	if len(value) <= limit {
		return value
	}
	return truncateLongMemEvalQAError(value[:limit])
}

func writeLongMemEvalQAArtifacts(ctx context.Context, opts LongMemEvalQAOptions, checkpoints []LongMemEvalQACheckpoint, report *LongMemEvalQAReport) error {
	store := artifact.NewLocalStore(opts.ArtifactRoot)
	finalExecution := opts.Execution
	finalExecution.RunID = report.RunID
	finalExecution.ImplementationRev = report.ImplementationRevision
	prefix := filepath.ToSlash(filepath.Join("benchmarks", report.RunID))
	paths := map[string]string{
		"source":             filepath.ToSlash(filepath.Join(prefix, "source.json")),
		"reader_config":      filepath.ToSlash(filepath.Join(prefix, "reader-config.json")),
		"judge_config":       filepath.ToSlash(filepath.Join(prefix, "judge-config.json")),
		"reader_results":     filepath.ToSlash(filepath.Join(prefix, "reader-results.jsonl")),
		"judge_results":      filepath.ToSlash(filepath.Join(prefix, "judge-results.jsonl")),
		"scores":             filepath.ToSlash(filepath.Join(prefix, "scores.json")),
		"failure_ledger":     filepath.ToSlash(filepath.Join(prefix, "failure-ledger.json")),
		"report":             filepath.ToSlash(filepath.Join(prefix, "report.md")),
		"execution_manifest": filepath.ToSlash(filepath.Join(prefix, "execution-manifest.json")),
		"report_json":        filepath.ToSlash(filepath.Join(prefix, "report.json")),
	}
	report.Artifacts = make(map[string]string, len(paths))
	for name, path := range paths {
		uri, err := localArtifactURI(opts.ArtifactRoot, path)
		if err != nil {
			return err
		}
		report.Artifacts[name] = uri
	}
	systemDigest := sha256.Sum256([]byte(longMemEvalQASystemPrompt))
	if _, err := putJSONArtifact(ctx, store, paths["source"], map[string]any{
		"qualification":   opts.Qualification,
		"execution":       finalExecution,
		"source_summary":  report.SourceSummary,
		"retrieval_input": opts.Execution.RetrievalInput,
	}); err != nil {
		return err
	}
	if _, err := putJSONArtifact(ctx, store, paths["reader_config"], map[string]any{
		"reader":               opts.Execution.Reader,
		"system_prompt_sha256": hex.EncodeToString(systemDigest[:]),
	}); err != nil {
		return err
	}
	if _, err := putJSONArtifact(ctx, store, paths["judge_config"], map[string]any{
		"judge":           opts.Execution.Judge,
		"official_scorer": opts.Qualification.OfficialScorer,
	}); err != nil {
		return err
	}
	readerResults, judgeResults, err := longMemEvalQANormalizedJSONL(checkpoints)
	if err != nil {
		return err
	}
	if _, err := putTextArtifact(ctx, store, paths["reader_results"], readerResults); err != nil {
		return err
	}
	if _, err := putTextArtifact(ctx, store, paths["judge_results"], judgeResults); err != nil {
		return err
	}
	if _, err := putJSONArtifact(ctx, store, paths["scores"], map[string]any{
		"run_id":                            report.RunID,
		"source_summary":                    report.SourceSummary,
		"conditions":                        report.Conditions,
		"question_types":                    report.QuestionTypes,
		"retrieval_classes":                 report.RetrievalClasses,
		"paired":                            report.Paired,
		"judge_deterministic_disagreements": report.JudgeDisagreements,
	}); err != nil {
		return err
	}
	if _, err := putJSONArtifact(ctx, store, paths["failure_ledger"], report.Failures); err != nil {
		return err
	}
	if _, err := putTextArtifact(ctx, store, paths["report"], markdownLongMemEvalQAReport(*report)); err != nil {
		return err
	}
	finalExecution.Artifacts = copyStringMap(report.Artifacts)
	if err := benchmark.ValidateLongMemEvalQAExecution(opts.Qualification, finalExecution); err != nil {
		return err
	}
	if _, err := putJSONArtifact(ctx, store, paths["execution_manifest"], finalExecution); err != nil {
		return err
	}
	_, err = putJSONArtifact(ctx, store, paths["report_json"], report)
	return err
}

func longMemEvalQANormalizedJSONL(checkpoints []LongMemEvalQACheckpoint) (string, string, error) {
	sorted := append([]LongMemEvalQACheckpoint(nil), checkpoints...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].RecordID == sorted[j].RecordID {
			return sorted[i].Condition < sorted[j].Condition
		}
		return sorted[i].RecordID < sorted[j].RecordID
	})
	var readers strings.Builder
	var judges strings.Builder
	for _, checkpoint := range sorted {
		readerUsage := LongMemEvalQAUsageAggregate{}
		addLongMemEvalQAUsage(&readerUsage, checkpoint.Attempts)
		reader := longMemEvalQAReaderResult{
			RecordID:                checkpoint.RecordID,
			QuestionType:            checkpoint.QuestionType,
			Abstention:              checkpoint.Abstention,
			Condition:               checkpoint.Condition,
			RetrievalClassification: checkpoint.RetrievalClassification,
			Status:                  checkpoint.ReaderStatus,
			Response:                checkpoint.Response,
			ProviderModel:           checkpoint.ProviderModel,
			AttemptCount:            len(checkpoint.Attempts),
			LatencyMillis:           longMemEvalQAAttemptsLatency(checkpoint.Attempts),
			Usage:                   readerUsage,
		}
		if checkpoint.Score != nil {
			reader.Score = &longMemEvalQASafeScore{
				ExactMatch:         checkpoint.Score.ExactMatch,
				TokenF1:            checkpoint.Score.TokenF1,
				AnswerTokenRecall:  checkpoint.Score.AnswerTokenRecall,
				AbstentionExpected: checkpoint.Score.AbstentionExpected,
				AbstentionDetected: checkpoint.Score.AbstentionDetected,
			}
		}
		data, err := json.Marshal(reader)
		if err != nil {
			return "", "", err
		}
		readers.Write(data)
		readers.WriteByte('\n')
		judgeUsage := LongMemEvalQAUsageAggregate{}
		addLongMemEvalQAUsage(&judgeUsage, checkpoint.Judge.Attempts)
		judge := longMemEvalQAJudgeResult{
			RecordID:      checkpoint.RecordID,
			QuestionType:  checkpoint.QuestionType,
			Abstention:    checkpoint.Abstention,
			Condition:     checkpoint.Condition,
			Status:        checkpoint.Judge.Status,
			Correct:       checkpoint.Judge.Correct,
			Output:        checkpoint.Judge.Output,
			ProviderModel: checkpoint.Judge.ProviderModel,
			AttemptCount:  len(checkpoint.Judge.Attempts),
			LatencyMillis: longMemEvalQAAttemptsLatency(checkpoint.Judge.Attempts),
			Usage:         judgeUsage,
		}
		data, err = json.Marshal(judge)
		if err != nil {
			return "", "", err
		}
		judges.Write(data)
		judges.WriteByte('\n')
	}
	return readers.String(), judges.String(), nil
}

func markdownLongMemEvalQAReport(report LongMemEvalQAReport) string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "# LongMemEval-S Full Reader QA\n\n")
	fmt.Fprintf(&builder, "- Run ID: `%s`\n", report.RunID)
	fmt.Fprintf(&builder, "- Records / reader tasks: `%d` / `%d`\n", report.RecordCount, report.TaskCount)
	fmt.Fprintf(&builder, "- Reader: `%s` / `%s`\n", report.Reader.Provider, report.Reader.Model)
	fmt.Fprintf(&builder, "- Judge: `%s` / `%s` (`%s`)\n", report.Judge.Provider, report.Judge.Model, report.Judge.ScorerClass)
	fmt.Fprintf(&builder, "- Failure ledger entries: `%d`\n\n", len(report.Failures))
	builder.WriteString("## Condition Results\n\n")
	builder.WriteString("| Condition | Completed | Reader failed | Judged | Judge failed | Judge invalid | Correct / total | Overall accuracy | Judged accuracy |\n")
	builder.WriteString("|---|---:|---:|---:|---:|---:|---:|---:|---:|\n")
	for _, condition := range []string{report.Paired.FirstCondition, report.Paired.SecondCondition} {
		aggregate := report.Conditions[condition]
		fmt.Fprintf(&builder, "| `%s` | %d | %d | %d | %d | %d | %d / %d | %.4f | %.4f |\n",
			condition, aggregate.Completed, aggregate.ReaderFailed, aggregate.Judged, aggregate.JudgeFailed, aggregate.JudgeInvalid,
			aggregate.JudgeCorrect, aggregate.Total, aggregate.OverallJudgeAccuracy, aggregate.JudgedAccuracy)
	}
	builder.WriteString("\n## Paired Results\n\n")
	fmt.Fprintf(&builder, "- Eligible: `%d`; both correct: `%d`; `%s` only: `%d`; `%s` only: `%d`; neither: `%d`; incomplete: `%d`.\n",
		report.Paired.Eligible, report.Paired.BothCorrect, report.Paired.FirstCondition, report.Paired.FirstOnlyCorrect,
		report.Paired.SecondCondition, report.Paired.SecondOnlyCorrect, report.Paired.NeitherCorrect, report.Paired.Incomplete)
	builder.WriteString("\n## Non-Claims\n\n")
	for _, nonClaim := range report.NonClaims {
		builder.WriteString("- " + nonClaim + "\n")
	}
	return builder.String()
}
