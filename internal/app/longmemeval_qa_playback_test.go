package app

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"vermory/internal/benchmark"
)

func TestLoadLongMemEvalQARetrievalAcceptsFrozenRankings(t *testing.T) {
	recordA := longMemEvalQAPlaybackRecord("record-a")
	recordB := longMemEvalQAPlaybackRecord("record-b")
	resultA := longMemEvalQAPlaybackResult(recordA)
	resultB := longMemEvalQAPlaybackResult(recordB)
	path, execution := writeLongMemEvalQARetrieval(t, []LongMemEvalRetrievalRecordResult{resultA, resultB})

	loaded, err := LoadLongMemEvalQARetrieval(path, execution)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 2 {
		t.Fatalf("loaded %d retrieval records, want 2", len(loaded))
	}
	for _, expected := range []LongMemEvalRetrievalRecordResult{resultA, resultB} {
		if !reflect.DeepEqual(loaded[expected.RecordID], expected) {
			t.Fatalf("loaded retrieval result changed:\nwant=%#v\ngot=%#v", expected, loaded[expected.RecordID])
		}
	}
}

func TestLoadLongMemEvalQARetrievalAcceptsRepeatedDistractorRawIDsAtDifferentPositions(t *testing.T) {
	record := longMemEvalQAPlaybackRecord("record-repeat")
	record.HaystackSessionIDs[10] = "duplicate-distractor"
	record.HaystackSessionIDs[11] = "duplicate-distractor"
	result := longMemEvalQAPlaybackResult(record)
	path, execution := writeLongMemEvalQARetrieval(t, []LongMemEvalRetrievalRecordResult{result})

	if _, err := LoadLongMemEvalQARetrieval(path, execution); err != nil {
		t.Fatal(err)
	}
	if _, err := BuildLongMemEvalQATasks(record, result, 10); err != nil {
		t.Fatal(err)
	}
}

func TestLoadLongMemEvalQARetrievalRejectsIdentityMismatch(t *testing.T) {
	record := longMemEvalQAPlaybackRecord("record-mismatch")
	base := longMemEvalQAPlaybackResult(record)
	tests := map[string]func(*LongMemEvalRetrievalRecordResult){
		"run_id": func(result *LongMemEvalRetrievalRecordResult) { result.RunID = "other-run" },
		"implementation_revision": func(result *LongMemEvalRetrievalRecordResult) {
			result.ImplementationRevision = strings.Repeat("9", 40)
		},
		"dataset_sha256":    func(result *LongMemEvalRetrievalRecordResult) { result.DatasetSHA256 = strings.Repeat("8", 64) },
		"record_set_sha256": func(result *LongMemEvalRetrievalRecordResult) { result.RecordSetSHA256 = strings.Repeat("7", 64) },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			result := base
			mutate(&result)
			path, execution := writeLongMemEvalQARetrieval(t, []LongMemEvalRetrievalRecordResult{result})
			_, err := LoadLongMemEvalQARetrieval(path, execution)
			if err == nil || !strings.Contains(err.Error(), name) {
				t.Fatalf("expected %s rejection, got %v", name, err)
			}
		})
	}
}

func TestLoadLongMemEvalQARetrievalRejectsDuplicateRecord(t *testing.T) {
	record := longMemEvalQAPlaybackRecord("record-duplicate")
	result := longMemEvalQAPlaybackResult(record)
	path, execution := writeLongMemEvalQARetrieval(t, []LongMemEvalRetrievalRecordResult{result, result})
	_, err := LoadLongMemEvalQARetrieval(path, execution)
	if err == nil || !strings.Contains(err.Error(), "duplicate record_id") {
		t.Fatalf("expected duplicate rejection, got %v", err)
	}
}

func TestLoadLongMemEvalQARetrievalRejectsMissingCondition(t *testing.T) {
	record := longMemEvalQAPlaybackRecord("record-condition")
	result := longMemEvalQAPlaybackResult(record)
	result.Conditions = result.Conditions[:1]
	path, execution := writeLongMemEvalQARetrieval(t, []LongMemEvalRetrievalRecordResult{result})
	_, err := LoadLongMemEvalQARetrieval(path, execution)
	if err == nil || !strings.Contains(err.Error(), "conditions") {
		t.Fatalf("expected condition rejection, got %v", err)
	}
}

func TestLoadLongMemEvalQARetrievalRejectsFileDigestMismatch(t *testing.T) {
	record := longMemEvalQAPlaybackRecord("record-digest")
	path, execution := writeLongMemEvalQARetrieval(t, []LongMemEvalRetrievalRecordResult{longMemEvalQAPlaybackResult(record)})
	execution.RetrievalInput.SHA256 = strings.Repeat("f", 64)
	_, err := LoadLongMemEvalQARetrieval(path, execution)
	if err == nil || !strings.Contains(err.Error(), "SHA-256") {
		t.Fatalf("expected digest rejection, got %v", err)
	}
}

func TestBuildLongMemEvalQATasksUsesExactK10PrefixesAndSemanticWrappers(t *testing.T) {
	record := longMemEvalQAPlaybackRecord("record-context")
	result := longMemEvalQAPlaybackResult(record)

	tasks, err := BuildLongMemEvalQATasks(record, result, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 2 {
		t.Fatalf("expected two tasks, got %d", len(tasks))
	}
	byCondition := make(map[string]LongMemEvalQATask, len(tasks))
	for _, task := range tasks {
		byCondition[task.Condition] = task
		if task.Question != record.Question || task.SystemPrompt == "" {
			t.Fatalf("task changed question or omitted system prompt: %#v", task)
		}
		if len(task.RankedOccurrenceKeys) != 10 || len(task.RankedSessionIDs) != 10 {
			t.Fatalf("task did not preserve K10 ranking: %#v", task)
		}
		for _, forbidden := range []string{
			"has_answer", "answer_session_ids", "question_type", "continuity_id",
			"tenant_id", "memory_id", "source_ref", "expected_score", record.Answer,
		} {
			if strings.Contains(task.ContextPacket, forbidden) {
				t.Fatalf("context leaked %q: %s", forbidden, task.ContextPacket)
			}
		}
	}
	plain := byCondition[longMemEvalQAPlainCondition]
	vermory := byCondition[longMemEvalQAVermoryCondition]
	if !strings.HasPrefix(plain.ContextPacket, "Retrieved conversation memory:\n") {
		t.Fatalf("unexpected plain wrapper: %q", plain.ContextPacket)
	}
	if !strings.HasPrefix(vermory.ContextPacket, "Governed memory:\n") {
		t.Fatalf("unexpected Vermory wrapper: %q", vermory.ContextPacket)
	}
	if plain.RetrievalClassification != "all_evidence_retrieved" {
		t.Fatalf("unexpected plain K10 classification: %q", plain.RetrievalClassification)
	}
	if vermory.RetrievalClassification != "partial_evidence_retrieved" {
		t.Fatalf("unexpected Vermory K10 classification: %q", vermory.RetrievalClassification)
	}
	if plain.ContextSHA256 == "" || plain.PromptSHA256 == "" || vermory.ContextSHA256 == "" {
		t.Fatalf("task hashes were not populated: %#v %#v", plain, vermory)
	}

	repeated, err := BuildLongMemEvalQATasks(record, result, 10)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(tasks, repeated) {
		t.Fatalf("task order or bytes changed across repeats:\nfirst=%#v\nsecond=%#v", tasks, repeated)
	}
}

func TestLongMemEvalQAVectorPlaybackSelectsLexicalAndEffectiveVectorRankings(t *testing.T) {
	record := longMemEvalQAPlaybackRecord("record-vector")
	result := longMemEvalQAPlaybackVectorResult(record)
	path, execution := writeLongMemEvalQARetrieval(t, []LongMemEvalRetrievalRecordResult{result})
	execution.Conditions = []string{longMemEvalQAVermoryCondition, longMemEvalQAVectorCondition}

	loaded, err := LoadLongMemEvalQARetrieval(path, execution)
	if err != nil {
		t.Fatal(err)
	}
	tasks, err := BuildLongMemEvalQATasks(record, loaded[record.QuestionID], 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 2 {
		t.Fatalf("expected two selected tasks, got %d", len(tasks))
	}
	byCondition := make(map[string]LongMemEvalQATask, len(tasks))
	for _, task := range tasks {
		byCondition[task.Condition] = task
		if !strings.HasPrefix(task.ContextPacket, "Governed memory:\n") {
			t.Fatalf("condition %s did not use governed context: %q", task.Condition, task.ContextPacket)
		}
	}
	lexical := byCondition[longMemEvalQAVermoryCondition]
	vector := byCondition[longMemEvalQAVectorCondition]
	if lexical.Condition == "" || vector.Condition == "" {
		t.Fatalf("missing lexical/vector tasks: %#v", byCondition)
	}
	if lexical.ContextSHA256 == vector.ContextSHA256 {
		t.Fatal("lexical and vector rankings produced identical frozen context")
	}
	if vector.RetrievalClassification != "all_evidence_retrieved" {
		t.Fatalf("unexpected vector classification: %q", vector.RetrievalClassification)
	}

	degraded := result
	degraded.Conditions = append([]LongMemEvalRetrievalConditionResult(nil), result.Conditions...)
	degraded.Conditions[2].Degraded = true
	path, execution = writeLongMemEvalQARetrieval(t, []LongMemEvalRetrievalRecordResult{degraded})
	execution.Conditions = []string{longMemEvalQAVermoryCondition, longMemEvalQAVectorCondition}
	if _, err := LoadLongMemEvalQARetrieval(path, execution); err == nil || !strings.Contains(err.Error(), "degraded") {
		t.Fatalf("expected degraded vector rejection, got %v", err)
	}
}

func TestBuildLongMemEvalQATasksPreservesShortProductionRanking(t *testing.T) {
	record := longMemEvalQAPlaybackRecord("record-short-ranking")
	result := longMemEvalQAPlaybackResult(record)
	result.Conditions[1].RankedOccurrenceKeys = result.Conditions[1].RankedOccurrenceKeys[:1]
	result.Conditions[1].RankedSessionIDs = result.Conditions[1].RankedSessionIDs[:1]

	path, execution := writeLongMemEvalQARetrieval(t, []LongMemEvalRetrievalRecordResult{result})
	loaded, err := LoadLongMemEvalQARetrieval(path, execution)
	if err != nil {
		t.Fatal(err)
	}
	tasks, err := BuildLongMemEvalQATasks(record, loaded[record.QuestionID], 10)
	if err != nil {
		t.Fatal(err)
	}

	byCondition := make(map[string]LongMemEvalQATask, len(tasks))
	for _, task := range tasks {
		byCondition[task.Condition] = task
	}
	plain := byCondition[longMemEvalQAPlainCondition]
	vermory := byCondition[longMemEvalQAVermoryCondition]
	if len(plain.RankedOccurrenceKeys) != 10 || len(plain.RankedSessionIDs) != 10 {
		t.Fatalf("plain task did not preserve its K10 prefix: %#v", plain)
	}
	if len(vermory.RankedOccurrenceKeys) != 1 || len(vermory.RankedSessionIDs) != 1 {
		t.Fatalf("Vermory task did not preserve the one-session ranking: %#v", vermory)
	}
	selected := record.HaystackSessions[11][0].Content
	if !strings.Contains(vermory.ContextPacket, selected) {
		t.Fatalf("Vermory context omitted its selected session: %q", vermory.ContextPacket)
	}
	if strings.Contains(vermory.ContextPacket, record.HaystackSessions[10][0].Content) {
		t.Fatalf("Vermory context manufactured an unranked filler session: %q", vermory.ContextPacket)
	}
}

func TestBuildLongMemEvalQATasksRejectsOccurrenceMismatchAndInvalidK(t *testing.T) {
	record := longMemEvalQAPlaybackRecord("record-invalid")
	result := longMemEvalQAPlaybackResult(record)
	result.Conditions[0].RankedOccurrenceKeys[0] = "000000:wrong-id"
	if _, err := BuildLongMemEvalQATasks(record, result, 10); err == nil || !strings.Contains(err.Error(), "occurrence") {
		t.Fatalf("expected occurrence rejection, got %v", err)
	}

	result = longMemEvalQAPlaybackResult(record)
	result.Conditions[0].RankedOccurrenceKeys[0] = "999999:session-00"
	if _, err := BuildLongMemEvalQATasks(record, result, 10); err == nil || !strings.Contains(err.Error(), "out of range") {
		t.Fatalf("expected unknown occurrence rejection, got %v", err)
	}

	result = longMemEvalQAPlaybackResult(record)
	result.Conditions[0].RankedSessionIDs[0] = "wrong-id"
	if _, err := BuildLongMemEvalQATasks(record, result, 10); err == nil || !strings.Contains(err.Error(), "raw ID") {
		t.Fatalf("expected ranked ID rejection, got %v", err)
	}

	result = longMemEvalQAPlaybackResult(record)
	if _, err := BuildLongMemEvalQATasks(record, result, 9); err == nil || !strings.Contains(err.Error(), "K=10") {
		t.Fatalf("expected K rejection, got %v", err)
	}
}

func TestLongMemEvalQARealPlaybackMetadata(t *testing.T) {
	sourcePath := os.Getenv("VERMORY_LONGMEMEVAL_S_DATASET")
	retrievalPath := os.Getenv("VERMORY_LONGMEMEVAL_W14_RESULTS")
	if sourcePath == "" || retrievalPath == "" {
		t.Skip("real LongMemEval-S source and W14 retrieval results are not configured")
	}
	execution, err := benchmark.LoadExecution("../../casebook/benchmarks/executions/longmemeval-s-full-reader-qa.json")
	if err != nil {
		t.Fatal(err)
	}
	retrieval, err := LoadLongMemEvalQARetrieval(retrievalPath, execution)
	if err != nil {
		t.Fatal(err)
	}
	if len(retrieval) != 500 {
		t.Fatalf("loaded %d W14 records, want 500", len(retrieval))
	}
	tasks := 0
	summary, err := benchmark.ScanLongMemEval(sourcePath, func(record benchmark.LongMemEvalRecord) error {
		result, exists := retrieval[record.QuestionID]
		if !exists {
			return os.ErrNotExist
		}
		built, err := BuildLongMemEvalQATasks(record, result, execution.RetrievalInput.K)
		if err != nil {
			return err
		}
		tasks += len(built)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if summary.RecordCount != 500 || summary.SessionCount != 23867 || summary.TurnCount != 246750 || summary.ScoredRecordCount != 470 || summary.AbstentionRecordCount != 30 {
		t.Fatalf("unexpected real source summary: %#v", summary)
	}
	if summary.RecordSetSHA256 != execution.RecordSetSHA256 {
		t.Fatalf("record-set digest=%s want %s", summary.RecordSetSHA256, execution.RecordSetSHA256)
	}
	if tasks != 1000 {
		t.Fatalf("built %d tasks, want 1000", tasks)
	}
}

func TestLongMemEvalQAW28RealPlaybackMetadata(t *testing.T) {
	sourcePath := os.Getenv("VERMORY_LONGMEMEVAL_S_DATASET")
	retrievalPath := os.Getenv("VERMORY_LONGMEMEVAL_W28_RESULTS")
	if sourcePath == "" || retrievalPath == "" {
		t.Skip("real LongMemEval-S source and W28 retrieval results are not configured")
	}
	execution, err := benchmark.LoadExecution("../../casebook/benchmarks/executions/longmemeval-s-full-vector-reader-qa.json")
	if err != nil {
		t.Fatal(err)
	}
	retrieval, err := LoadLongMemEvalQARetrieval(retrievalPath, execution)
	if err != nil {
		t.Fatal(err)
	}
	if len(retrieval) != 500 {
		t.Fatalf("loaded %d W28 records, want 500", len(retrieval))
	}
	tasks := 0
	summary, err := benchmark.ScanLongMemEval(sourcePath, func(record benchmark.LongMemEvalRecord) error {
		result, exists := retrieval[record.QuestionID]
		if !exists {
			return os.ErrNotExist
		}
		built, err := BuildLongMemEvalQATasks(record, result, execution.RetrievalInput.K)
		if err != nil {
			return err
		}
		for _, task := range built {
			if task.Condition != longMemEvalQAVermoryCondition && task.Condition != longMemEvalQAVectorCondition {
				return os.ErrInvalid
			}
		}
		tasks += len(built)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if summary.RecordCount != 500 || summary.SessionCount != 23867 || summary.TurnCount != 246750 || summary.ScoredRecordCount != 470 || summary.AbstentionRecordCount != 30 {
		t.Fatalf("unexpected real source summary: %#v", summary)
	}
	if summary.RecordSetSHA256 != execution.RecordSetSHA256 {
		t.Fatalf("record-set digest=%s want %s", summary.RecordSetSHA256, execution.RecordSetSHA256)
	}
	if tasks != 1000 {
		t.Fatalf("built %d tasks, want 1000", tasks)
	}
}

func longMemEvalQAPlaybackRecord(id string) benchmark.LongMemEvalRecord {
	record := benchmark.LongMemEvalRecord{
		QuestionID:       id,
		QuestionType:     "multi-session",
		Question:         "What was the final deployment decision?",
		Answer:           "REFERENCE-ANSWER-MUST-NOT-BE-INJECTED",
		QuestionDate:     "2026-07-15",
		AnswerSessionIDs: []string{"session-00", "session-01"},
	}
	for index := 0; index < 12; index++ {
		record.HaystackDates = append(record.HaystackDates, "2026-07-"+twoDigits(index+1))
		record.HaystackSessionIDs = append(record.HaystackSessionIDs, "session-"+twoDigits(index))
		record.HaystackSessions = append(record.HaystackSessions, []benchmark.LongMemEvalTurn{
			{Role: "user", Content: "deployment memory " + twoDigits(index)},
			{Role: "assistant", Content: "recorded decision " + twoDigits(index)},
		})
	}
	return record
}

func longMemEvalQAPlaybackResult(record benchmark.LongMemEvalRecord) LongMemEvalRetrievalRecordResult {
	plainKeys := make([]string, 0, 12)
	plainIDs := make([]string, 0, 12)
	vermoryKeys := make([]string, 0, 12)
	vermoryIDs := make([]string, 0, 12)
	for index := 0; index < 12; index++ {
		plainKeys = append(plainKeys, longMemEvalRetrievalOccurrenceKey(index, record.HaystackSessionIDs[index]))
		plainIDs = append(plainIDs, record.HaystackSessionIDs[index])
		reversed := 11 - index
		vermoryKeys = append(vermoryKeys, longMemEvalRetrievalOccurrenceKey(reversed, record.HaystackSessionIDs[reversed]))
		vermoryIDs = append(vermoryIDs, record.HaystackSessionIDs[reversed])
	}
	return LongMemEvalRetrievalRecordResult{
		SchemaVersion:          "longmemeval-retrieval-checkpoint/v1",
		RunID:                  "longmemeval-s-full-retrieval-test",
		ImplementationRevision: strings.Repeat("e", 40),
		DatasetSHA256:          strings.Repeat("a", 64),
		RecordSetSHA256:        strings.Repeat("c", 64),
		RecordID:               record.QuestionID,
		QuestionType:           record.QuestionType,
		Status:                 "completed",
		Conditions: []LongMemEvalRetrievalConditionResult{
			{
				Condition:            longMemEvalRetrievalBaseline,
				Status:               "completed",
				RankedOccurrenceKeys: plainKeys,
				RankedSessionIDs:     plainIDs,
				MetricAt10:           &benchmark.SessionRetrievalMetric{K: 10, RecallAny: 1, RecallAll: 1},
			},
			{
				Condition:            longMemEvalRetrievalVermory,
				Status:               "completed",
				RankedOccurrenceKeys: vermoryKeys,
				RankedSessionIDs:     vermoryIDs,
				MetricAt10:           &benchmark.SessionRetrievalMetric{K: 10, RecallAny: 1, RecallAll: 0},
			},
		},
	}
}

func longMemEvalQAPlaybackVectorResult(record benchmark.LongMemEvalRecord) LongMemEvalRetrievalRecordResult {
	result := longMemEvalQAPlaybackResult(record)
	vectorKeys := make([]string, 0, 12)
	vectorIDs := make([]string, 0, 12)
	for index := 0; index < 12; index++ {
		position := (index + 1) % 12
		vectorKeys = append(vectorKeys, longMemEvalRetrievalOccurrenceKey(position, record.HaystackSessionIDs[position]))
		vectorIDs = append(vectorIDs, record.HaystackSessionIDs[position])
	}
	result.Conditions = append(result.Conditions, LongMemEvalRetrievalConditionResult{
		Condition:            longMemEvalRetrievalVector,
		Status:               "completed",
		RankedOccurrenceKeys: vectorKeys,
		RankedSessionIDs:     vectorIDs,
		MetricAt10:           &benchmark.SessionRetrievalMetric{K: 10, RecallAny: 1, RecallAll: 1},
		EffectiveMode:        "vector",
	})
	return result
}

func writeLongMemEvalQARetrieval(t *testing.T, results []LongMemEvalRetrievalRecordResult) (string, benchmark.ExecutionManifest) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "retrieval-results.jsonl")
	var lines strings.Builder
	for _, result := range results {
		data, err := json.Marshal(result)
		if err != nil {
			t.Fatal(err)
		}
		lines.Write(data)
		lines.WriteByte('\n')
	}
	data := []byte(lines.String())
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(data)
	execution := longMemEvalQAPlaybackExecution()
	execution.RetrievalInput.SHA256 = hex.EncodeToString(digest[:])
	return path, execution
}

func longMemEvalQAPlaybackExecution() benchmark.ExecutionManifest {
	return benchmark.ExecutionManifest{
		SchemaVersion:             "benchmark-execution/v1",
		Benchmark:                 "LongMemEval",
		QualificationPath:         "qualification.json",
		DatasetSHA256:             strings.Repeat("a", 64),
		EvaluationTarget:          benchmark.EvaluationTargetQA,
		ExecutionScope:            benchmark.ExecutionScopeFull,
		ClaimScope:                benchmark.ClaimScopeQualifiedDatasetFull,
		SelectionMode:             benchmark.SelectionModeAllRecords,
		RecordSetSHA256:           strings.Repeat("c", 64),
		ExpectedSessionCount:      23867,
		ExpectedTurnCount:         246750,
		ExpectedScoredRecordCount: 470,
		Conditions: []string{
			longMemEvalQAPlainCondition,
			longMemEvalQAVermoryCondition,
		},
		RetrievalInput: &benchmark.RetrievalExecutionInput{
			Path:                   "retrieval-results.jsonl",
			RunID:                  "longmemeval-s-full-retrieval-test",
			ImplementationRevision: strings.Repeat("e", 40),
			K:                      10,
		},
	}
}

func twoDigits(value int) string {
	return string([]byte{'0' + byte(value/10), '0' + byte(value%10)})
}
