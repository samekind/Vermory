package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"vermory/internal/benchmark"
	"vermory/internal/provider"
)

func TestAggregateLongMemEvalQACoversTerminalStatesPairsAndAttributionDeterministically(t *testing.T) {
	fixture := writeLongMemEvalQAReaderFixture(t, 6)
	checkpoints := longMemEvalQAAggregateFixture(*fixture.execution.Judge)
	summary := benchmark.LongMemEvalSummary{RecordCount: 6, ScoredRecordCount: 5, AbstentionRecordCount: 1, SessionCount: 72, TurnCount: 144, RecordSetSHA256: fixture.execution.RecordSetSHA256}

	report, err := aggregateLongMemEvalQA(fixture.options(nil), summary, checkpoints)
	if err != nil {
		t.Fatal(err)
	}
	plain := report.Conditions[longMemEvalQAPlainCondition]
	vermory := report.Conditions[longMemEvalQAVermoryCondition]
	if plain.Total != 6 || plain.Completed != 5 || plain.ReaderFailed != 1 || plain.Judged != 4 || plain.JudgeFailed != 1 || plain.JudgeInvalid != 0 || plain.JudgeCorrect != 2 {
		t.Fatalf("unexpected plain aggregate: %#v", plain)
	}
	if vermory.Total != 6 || vermory.Completed != 6 || vermory.ReaderFailed != 0 || vermory.Judged != 5 || vermory.JudgeFailed != 0 || vermory.JudgeInvalid != 1 || vermory.JudgeCorrect != 2 {
		t.Fatalf("unexpected Vermory aggregate: %#v", vermory)
	}
	if plain.OverallJudgeAccuracy != 0.3333 || vermory.OverallJudgeAccuracy != 0.3333 {
		t.Fatalf("provider failures were hidden from overall accuracy: plain=%v vermory=%v", plain.OverallJudgeAccuracy, vermory.OverallJudgeAccuracy)
	}
	if plain.JudgedAccuracy != 0.5 || vermory.JudgedAccuracy != 0.4 {
		t.Fatalf("unexpected judged-only accuracy: plain=%v vermory=%v", plain.JudgedAccuracy, vermory.JudgedAccuracy)
	}
	if report.Paired.Eligible != 4 || report.Paired.BothCorrect != 1 || report.Paired.PlainOnlyCorrect != 1 || report.Paired.VermoryOnlyCorrect != 1 || report.Paired.NeitherCorrect != 1 || report.Paired.Incomplete != 2 {
		t.Fatalf("unexpected paired table: %#v", report.Paired)
	}
	if len(report.Failures) != 8 {
		t.Fatalf("failure ledger has %d entries, want 8: %#v", len(report.Failures), report.Failures)
	}
	wantPrimary := map[string]int{
		"retrieval_no_evidence":         1,
		"retrieval_partial_evidence":    1,
		"reader_or_aggregation_failure": 2,
		"reader_runtime_failure":        1,
		"judge_failure":                 2,
		"abstention_failure":            1,
	}
	gotPrimary := make(map[string]int)
	lifecycle := 0
	for _, failure := range report.Failures {
		gotPrimary[failure.Primary]++
		if failure.SourceLifecycleCandidate {
			lifecycle++
		}
	}
	if lifecycle != 2 {
		t.Fatalf("source lifecycle candidates=%d want 2", lifecycle)
	}
	for primary, want := range wantPrimary {
		if gotPrimary[primary] != want {
			t.Fatalf("failure category %s=%d want %d", primary, gotPrimary[primary], want)
		}
	}
	if report.RetrievalClasses[longMemEvalQAPlainCondition]["no_evidence_retrieved"].Total == 0 || report.RetrievalClasses[longMemEvalQAVermoryCondition]["partial_evidence_retrieved"].Total == 0 {
		t.Fatalf("retrieval class groups are incomplete: %#v", report.RetrievalClasses)
	}
	if plain.Usage.AvailableAttempts == 0 || plain.ReaderLatency.P95Millis == 0 || report.JudgeDisagreements == 0 {
		t.Fatalf("usage, latency, or deterministic disagreement was not aggregated: %#v", report)
	}

	first, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	for left, right := 0, len(checkpoints)-1; left < right; left, right = left+1, right-1 {
		checkpoints[left], checkpoints[right] = checkpoints[right], checkpoints[left]
	}
	repeated, err := aggregateLongMemEvalQA(fixture.options(nil), summary, checkpoints)
	if err != nil {
		t.Fatal(err)
	}
	second, err := json.Marshal(repeated)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Fatal("aggregate report bytes changed after shuffled checkpoint load order")
	}
}

func TestAggregateLongMemEvalQALabelsLexicalVectorPairWithoutLegacyMisnaming(t *testing.T) {
	fixture := writeLongMemEvalQAReaderFixture(t, 6)
	opts := fixture.options(nil)
	opts.Execution.Conditions = []string{longMemEvalQAVermoryCondition, longMemEvalQAVectorCondition}
	checkpoints := longMemEvalQAAggregateFixture(*fixture.execution.Judge)
	for index := range checkpoints {
		if checkpoints[index].Condition == longMemEvalQAPlainCondition {
			checkpoints[index].Condition = longMemEvalQAVermoryCondition
		} else {
			checkpoints[index].Condition = longMemEvalQAVectorCondition
		}
	}
	summary := benchmark.LongMemEvalSummary{RecordCount: 6, ScoredRecordCount: 5, AbstentionRecordCount: 1, SessionCount: 72, TurnCount: 144, RecordSetSHA256: fixture.execution.RecordSetSHA256}

	report, err := aggregateLongMemEvalQA(opts, summary, checkpoints)
	if err != nil {
		t.Fatal(err)
	}
	if report.Paired.FirstCondition != longMemEvalQAVermoryCondition || report.Paired.SecondCondition != longMemEvalQAVectorCondition {
		t.Fatalf("unexpected pair labels: %#v", report.Paired)
	}
	if report.Paired.FirstOnlyCorrect != 1 || report.Paired.SecondOnlyCorrect != 1 || report.Paired.PlainOnlyCorrect != 0 || report.Paired.VermoryOnlyCorrect != 0 {
		t.Fatalf("generic or legacy pair counts are wrong: %#v", report.Paired)
	}
	markdown := markdownLongMemEvalQAReport(report)
	if !strings.Contains(markdown, "`vermory_lexical_k10` only") || !strings.Contains(markdown, "`vermory_vector_k10` only") || strings.Contains(markdown, "plain only") {
		t.Fatalf("vector pair markdown is mislabeled: %s", markdown)
	}
}

func TestFinalizeLongMemEvalQAWritesCompleteArtifactsAndRejectsMissingCheckpoint(t *testing.T) {
	fixture := writeLongMemEvalQAReaderFixture(t, 3)
	readerOpts := fixture.options(&longMemEvalQARecordingProvider{})
	readerOpts.RetrySleeper = func(context.Context, time.Duration) error { return nil }
	if _, err := RunLongMemEvalQAReader(context.Background(), readerOpts); err != nil {
		t.Fatal(err)
	}
	judgeOpts := fixture.options(nil)
	judgeOpts.JudgeProvider = &longMemEvalQAJudgeRecordingProvider{}
	judgeOpts.RetrySleeper = func(context.Context, time.Duration) error { return nil }
	if _, err := RunLongMemEvalQAJudge(context.Background(), judgeOpts); err != nil {
		t.Fatal(err)
	}

	report, err := FinalizeLongMemEvalQA(fixture.options(nil))
	if err != nil {
		t.Fatal(err)
	}
	if report.TaskCount != 6 || len(report.Artifacts) != 10 {
		t.Fatalf("unexpected finalized report: %#v", report)
	}
	for _, name := range []string{"source", "reader_config", "judge_config", "reader_results", "judge_results", "scores", "failure_ledger", "report", "execution_manifest", "report_json"} {
		if report.Artifacts[name] == "" {
			t.Fatalf("missing artifact URI %q", name)
		}
	}
	readerResultsPath := filepath.Join(fixture.artifactRoot, "benchmarks", fixture.execution.RunID, "reader-results.jsonl")
	readerResults, err := os.ReadFile(readerResultsPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"Retrieved conversation memory:", "Governed memory:", "REFERENCE-ANSWER", "normalized_reference", "reference_variant"} {
		if strings.Contains(string(readerResults), forbidden) {
			t.Fatalf("normalized reader results leaked %q", forbidden)
		}
	}
	manifest, err := benchmark.LoadExecution(filepath.Join(fixture.artifactRoot, "benchmarks", fixture.execution.RunID, "execution-manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := benchmark.ValidateLongMemEvalQAExecution(fixture.qualification, manifest); err != nil {
		t.Fatalf("final execution manifest is invalid: %v", err)
	}
	sourceData, err := os.ReadFile(filepath.Join(fixture.artifactRoot, "benchmarks", fixture.execution.RunID, "source.json"))
	if err != nil {
		t.Fatal(err)
	}
	var source struct {
		Execution benchmark.ExecutionManifest `json:"execution"`
	}
	if err := json.Unmarshal(sourceData, &source); err != nil {
		t.Fatal(err)
	}
	if source.Execution.RunID != report.RunID || source.Execution.ImplementationRev != report.ImplementationRevision {
		t.Fatalf("source artifact did not freeze final execution identity: %#v", source.Execution)
	}

	missing, err := longMemEvalQACheckpointPath(fixture.artifactRoot, fixture.execution.RunID, "record-00", longMemEvalQAPlainCondition)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(missing); err != nil {
		t.Fatal(err)
	}
	if _, err := FinalizeLongMemEvalQA(fixture.options(nil)); err == nil || !strings.Contains(err.Error(), "missing") {
		t.Fatalf("expected missing checkpoint rejection, got %v", err)
	}
}

func longMemEvalQAAggregateFixture(judgeConfig benchmark.ExecutionModelConfig) []LongMemEvalQACheckpoint {
	types := []string{"single-session-user", "single-session-assistant", "multi-session", "knowledge-update", "temporal-reasoning", "single-session-preference"}
	checkpoints := make([]LongMemEvalQACheckpoint, 0, 12)
	for index, questionType := range types {
		recordID := "aggregate-" + twoDigits(index)
		if index == 5 {
			recordID += "_abs"
		}
		for _, condition := range []string{longMemEvalQAPlainCondition, longMemEvalQAVermoryCondition} {
			checkpoint := longMemEvalQAAggregateCheckpoint(recordID, questionType, condition, judgeConfig)
			checkpoints = append(checkpoints, checkpoint)
		}
	}
	setJudgeResult(&checkpoints[0], true)
	setJudgeResult(&checkpoints[1], true)
	setJudgeResult(&checkpoints[2], true)
	setJudgeResult(&checkpoints[3], false)
	setJudgeResult(&checkpoints[4], false)
	checkpoints[4].RetrievalClassification = "no_evidence_retrieved"
	setJudgeResult(&checkpoints[5], true)
	setJudgeResult(&checkpoints[6], false)
	setJudgeResult(&checkpoints[7], false)
	checkpoints[6].RetrievalClassification = "all_evidence_retrieved"
	checkpoints[7].RetrievalClassification = "all_evidence_retrieved"
	setReaderFailure(&checkpoints[8], judgeConfig)
	setJudgeInvalid(&checkpoints[9], judgeConfig)
	setJudgeFailed(&checkpoints[10], judgeConfig)
	setJudgeResult(&checkpoints[11], false)
	checkpoints[11].Abstention = true
	checkpoints[11].Score.AbstentionExpected = true
	return checkpoints
}

func longMemEvalQAAggregateCheckpoint(recordID, questionType, condition string, judgeConfig benchmark.ExecutionModelConfig) LongMemEvalQACheckpoint {
	score := benchmark.DeterministicScore{TokenF1: 0.5, AnswerTokenRecall: 0.75, NormalizedResponse: "response"}
	return LongMemEvalQACheckpoint{
		SchemaVersion:           longMemEvalQACheckpointSchema,
		RecordID:                recordID,
		QuestionType:            questionType,
		Condition:               condition,
		RetrievalClassification: "partial_evidence_retrieved",
		ReaderStatus:            longMemEvalQAReaderCompleted,
		Response:                "response",
		ProviderModel:           "reader",
		Attempts: []LongMemEvalQAAttempt{{
			Number: 1, Status: longMemEvalQAAttemptCompleted, DurationMillis: 10,
			Usage: &provider.TokenUsage{InputTokens: 100, CachedInputTokens: 20, OutputTokens: 5, ReasoningTokens: 2, TotalTokens: 105},
		}},
		Score: &score,
		Judge: &LongMemEvalQAJudgeState{Config: judgeConfig},
	}
}

func setJudgeResult(checkpoint *LongMemEvalQACheckpoint, correct bool) {
	checkpoint.Judge.Status = longMemEvalQAJudgeCompleted
	checkpoint.Judge.Correct = &correct
	if correct {
		checkpoint.Judge.Output = "yes"
	} else {
		checkpoint.Judge.Output = "no"
	}
	checkpoint.Judge.ProviderModel = checkpoint.Judge.Config.Model
	checkpoint.Judge.Attempts = []LongMemEvalQAAttempt{{Number: 1, Status: longMemEvalQAAttemptCompleted, DurationMillis: 3, Usage: &provider.TokenUsage{InputTokens: 20, OutputTokens: 1, TotalTokens: 21}}}
	checkpoint.Score.ExactMatch = !correct
}

func setReaderFailure(checkpoint *LongMemEvalQACheckpoint, judgeConfig benchmark.ExecutionModelConfig) {
	checkpoint.ReaderStatus = longMemEvalQAReaderFailed
	checkpoint.Response = ""
	checkpoint.Score = nil
	checkpoint.Attempts = []LongMemEvalQAAttempt{
		{Number: 1, Status: longMemEvalQAAttemptFailed, DurationMillis: 10, Error: "reader failed"},
		{Number: 2, Status: longMemEvalQAAttemptFailed, DurationMillis: 20, Error: "reader failed"},
		{Number: 3, Status: longMemEvalQAAttemptFailed, DurationMillis: 30, Error: "reader failed"},
	}
	checkpoint.Judge = &LongMemEvalQAJudgeState{Config: judgeConfig, Status: longMemEvalQAJudgeNotRunReaderFailed}
}

func setJudgeInvalid(checkpoint *LongMemEvalQACheckpoint, judgeConfig benchmark.ExecutionModelConfig) {
	checkpoint.Judge = &LongMemEvalQAJudgeState{
		Config: judgeConfig, Status: longMemEvalQAJudgeInvalid, Output: "yes because", Attempts: []LongMemEvalQAAttempt{
			{Number: 1, Status: longMemEvalQAAttemptInvalid, DurationMillis: 3, Error: "invalid label", Output: "yes because"},
			{Number: 2, Status: longMemEvalQAAttemptInvalid, DurationMillis: 4, Error: "invalid label", Output: "yes because"},
		},
	}
}

func setJudgeFailed(checkpoint *LongMemEvalQACheckpoint, judgeConfig benchmark.ExecutionModelConfig) {
	checkpoint.Judge = &LongMemEvalQAJudgeState{
		Config: judgeConfig, Status: longMemEvalQAJudgeFailed, Attempts: []LongMemEvalQAAttempt{
			{Number: 1, Status: longMemEvalQAAttemptFailed, DurationMillis: 3, Error: "judge failed"},
			{Number: 2, Status: longMemEvalQAAttemptFailed, DurationMillis: 4, Error: "judge failed"},
		},
	}
}
