package app

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"vermory/internal/benchmark"
	vermoryruntime "vermory/internal/runtime"
)

func LoadLongMemEvalQARetrieval(path string, execution benchmark.ExecutionManifest) (map[string]LongMemEvalRetrievalRecordResult, error) {
	if execution.RetrievalInput == nil {
		return nil, fmt.Errorf("LongMemEval QA retrieval_input is required")
	}
	input := *execution.RetrievalInput
	if err := benchmark.VerifyFileSHA256(path, input.SHA256); err != nil {
		return nil, fmt.Errorf("verify LongMemEval retrieval SHA-256: %w", err)
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	results := make(map[string]LongMemEvalRetrievalRecordResult, 500)
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64<<10), 2<<20)
	line := 0
	for scanner.Scan() {
		line++
		if strings.TrimSpace(scanner.Text()) == "" {
			return nil, fmt.Errorf("LongMemEval retrieval line %d is empty", line)
		}
		var result LongMemEvalRetrievalRecordResult
		decoder := json.NewDecoder(strings.NewReader(scanner.Text()))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&result); err != nil {
			return nil, fmt.Errorf("decode LongMemEval retrieval line %d: %w", line, err)
		}
		if err := validateLongMemEvalQARetrievalResult(result, execution); err != nil {
			return nil, fmt.Errorf("LongMemEval retrieval line %d: %w", line, err)
		}
		if _, exists := results[result.RecordID]; exists {
			return nil, fmt.Errorf("LongMemEval retrieval contains duplicate record_id %q", result.RecordID)
		}
		results[result.RecordID] = result
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan LongMemEval retrieval results: %w", err)
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("LongMemEval retrieval results contain no records")
	}
	return results, nil
}

func validateLongMemEvalQARetrievalResult(result LongMemEvalRetrievalRecordResult, execution benchmark.ExecutionManifest) error {
	input := execution.RetrievalInput
	if result.SchemaVersion != "longmemeval-retrieval-checkpoint/v1" {
		return fmt.Errorf("schema_version is %q", result.SchemaVersion)
	}
	if result.RunID != input.RunID {
		return fmt.Errorf("run_id is %q, want %q", result.RunID, input.RunID)
	}
	if result.ImplementationRevision != input.ImplementationRevision {
		return fmt.Errorf("implementation_revision is %q, want %q", result.ImplementationRevision, input.ImplementationRevision)
	}
	if result.DatasetSHA256 != execution.DatasetSHA256 {
		return fmt.Errorf("dataset_sha256 is %q, want %q", result.DatasetSHA256, execution.DatasetSHA256)
	}
	if result.RecordSetSHA256 != execution.RecordSetSHA256 {
		return fmt.Errorf("record_set_sha256 is %q, want %q", result.RecordSetSHA256, execution.RecordSetSHA256)
	}
	if strings.TrimSpace(result.RecordID) == "" || strings.TrimSpace(result.QuestionType) == "" {
		return fmt.Errorf("record_id and question_type are required")
	}
	if result.Status != "completed" {
		return fmt.Errorf("record %s status is %q", result.RecordID, result.Status)
	}
	expectedConditions, err := longMemEvalQARetrievalShape(execution.Conditions)
	if err != nil {
		return err
	}
	if len(result.Conditions) != len(expectedConditions) {
		return fmt.Errorf("record %s conditions must be %v", result.RecordID, expectedConditions)
	}
	for index, condition := range result.Conditions {
		if condition.Condition != expectedConditions[index] {
			return fmt.Errorf("record %s condition %d is %s, want %s", result.RecordID, index, condition.Condition, expectedConditions[index])
		}
		if condition.Status != "completed" {
			return fmt.Errorf("record %s condition %s status is %q", result.RecordID, condition.Condition, condition.Status)
		}
		if len(condition.RankedOccurrenceKeys) != len(condition.RankedSessionIDs) {
			return fmt.Errorf("record %s condition %s ranking lengths differ", result.RecordID, condition.Condition)
		}
		if len(condition.RankedOccurrenceKeys) == 0 {
			return fmt.Errorf("record %s condition %s has no rankings", result.RecordID, condition.Condition)
		}
		if !result.Abstention && condition.MetricAt10 == nil {
			return fmt.Errorf("record %s condition %s metric_at_10 is required", result.RecordID, condition.Condition)
		}
		if condition.Condition == longMemEvalRetrievalVector && (condition.EffectiveMode != vermoryruntime.RetrievalVector || condition.Degraded || condition.FailureCode != "") {
			return fmt.Errorf("record %s vector condition is degraded or not effective vector", result.RecordID)
		}
	}
	return nil
}

func longMemEvalQARetrievalShape(qaConditions []string) ([]string, error) {
	switch strings.Join(qaConditions, "\x00") {
	case longMemEvalQAPlainCondition + "\x00" + longMemEvalQAVermoryCondition:
		return []string{longMemEvalRetrievalBaseline, longMemEvalRetrievalVermory}, nil
	case longMemEvalQAVermoryCondition + "\x00" + longMemEvalQAVectorCondition:
		return []string{longMemEvalRetrievalBaseline, longMemEvalRetrievalVermory, longMemEvalRetrievalVector}, nil
	default:
		return nil, fmt.Errorf("unsupported LongMemEval QA conditions %v", qaConditions)
	}
}

func BuildLongMemEvalQATasks(record benchmark.LongMemEvalRecord, retrieval LongMemEvalRetrievalRecordResult, k int) ([]LongMemEvalQATask, error) {
	if k != 10 {
		return nil, fmt.Errorf("LongMemEval QA playback must use K=10")
	}
	if record.QuestionID != retrieval.RecordID {
		return nil, fmt.Errorf("source record_id %q does not match retrieval %q", record.QuestionID, retrieval.RecordID)
	}
	if record.QuestionType != retrieval.QuestionType {
		return nil, fmt.Errorf("source question_type %q does not match retrieval %q", record.QuestionType, retrieval.QuestionType)
	}
	var selected []LongMemEvalRetrievalConditionResult
	orderSeed := ""
	switch {
	case len(retrieval.Conditions) == 2 && retrieval.Conditions[0].Condition == longMemEvalRetrievalBaseline && retrieval.Conditions[1].Condition == longMemEvalRetrievalVermory:
		selected = retrieval.Conditions
		orderSeed = "longmemeval-w15-v1:"
	case len(retrieval.Conditions) == 3 && retrieval.Conditions[0].Condition == longMemEvalRetrievalBaseline && retrieval.Conditions[1].Condition == longMemEvalRetrievalVermory && retrieval.Conditions[2].Condition == longMemEvalRetrievalVector:
		selected = retrieval.Conditions[1:]
		orderSeed = "longmemeval-w29-v1:"
	default:
		return nil, fmt.Errorf("retrieval record %s conditions are incomplete", retrieval.RecordID)
	}

	tasks := make([]LongMemEvalQATask, 0, 2)
	for _, condition := range selected {
		task, err := buildLongMemEvalQATask(record, retrieval, condition, k)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}
	orderDigest := sha256.Sum256([]byte(orderSeed + record.QuestionID))
	if orderDigest[0]&1 == 1 {
		tasks[0], tasks[1] = tasks[1], tasks[0]
	}
	return tasks, nil
}

func buildLongMemEvalQATask(record benchmark.LongMemEvalRecord, retrieval LongMemEvalRetrievalRecordResult, condition LongMemEvalRetrievalConditionResult, k int) (LongMemEvalQATask, error) {
	if len(condition.RankedOccurrenceKeys) == 0 || len(condition.RankedSessionIDs) == 0 {
		return LongMemEvalQATask{}, fmt.Errorf("record %s condition %s contains no rankings", record.QuestionID, condition.Condition)
	}
	limit := min(k, len(condition.RankedOccurrenceKeys))
	sessions := make([]benchmark.LongMemEvalSession, 0, limit)
	keys := append([]string(nil), condition.RankedOccurrenceKeys[:limit]...)
	ids := append([]string(nil), condition.RankedSessionIDs[:limit]...)
	for index := 0; index < limit; index++ {
		position, rawID, err := parseLongMemEvalQARetrievalOccurrence(keys[index])
		if err != nil {
			return LongMemEvalQATask{}, fmt.Errorf("record %s condition %s occurrence %d: %w", record.QuestionID, condition.Condition, index, err)
		}
		if position < 0 || position >= len(record.HaystackSessions) {
			return LongMemEvalQATask{}, fmt.Errorf("record %s condition %s occurrence %q position is out of range", record.QuestionID, condition.Condition, keys[index])
		}
		if record.HaystackSessionIDs[position] != rawID || ids[index] != rawID {
			return LongMemEvalQATask{}, fmt.Errorf("record %s condition %s occurrence %q does not match source raw ID", record.QuestionID, condition.Condition, keys[index])
		}
		sessions = append(sessions, benchmark.LongMemEvalSession{
			ID:       rawID,
			Date:     record.HaystackDates[position],
			Turns:    append([]benchmark.LongMemEvalTurn(nil), record.HaystackSessions[position]...),
			Position: position,
		})
	}

	conditionName := ""
	contextPacket := ""
	switch condition.Condition {
	case longMemEvalRetrievalBaseline:
		conditionName = longMemEvalQAPlainCondition
		contextPacket = "Retrieved conversation memory:\n" + longMemEvalRetrievedContext(sessions)
	case longMemEvalRetrievalVermory, longMemEvalRetrievalVector:
		conditionName = longMemEvalQAVermoryCondition
		if condition.Condition == longMemEvalRetrievalVector {
			conditionName = longMemEvalQAVectorCondition
		}
		memories := make([]vermoryruntime.Memory, 0, len(sessions))
		for _, session := range sessions {
			memories = append(memories, vermoryruntime.Memory{Content: session.SemanticText()})
		}
		contextPacket = vermoryruntime.BuildConversationContext(nil, memories, nil)
	default:
		return LongMemEvalQATask{}, fmt.Errorf("record %s has unsupported retrieval condition %q", record.QuestionID, condition.Condition)
	}
	classification, err := longMemEvalQAK10Classification(retrieval.Abstention, condition.MetricAt10)
	if err != nil {
		return LongMemEvalQATask{}, fmt.Errorf("record %s condition %s: %w", record.QuestionID, condition.Condition, err)
	}
	promptDigest := sha256.Sum256([]byte(longMemEvalQASystemPrompt + "\n\n" + record.Question))
	contextDigest := sha256.Sum256([]byte(contextPacket))
	return LongMemEvalQATask{
		Record:                  record,
		RecordID:                record.QuestionID,
		QuestionType:            record.QuestionType,
		Abstention:              retrieval.Abstention,
		Condition:               conditionName,
		Question:                record.Question,
		SystemPrompt:            longMemEvalQASystemPrompt,
		ContextPacket:           contextPacket,
		PromptSHA256:            hex.EncodeToString(promptDigest[:]),
		ContextSHA256:           hex.EncodeToString(contextDigest[:]),
		RetrievalClassification: classification,
		RankedOccurrenceKeys:    keys,
		RankedSessionIDs:        ids,
	}, nil
}

func parseLongMemEvalQARetrievalOccurrence(key string) (int, string, error) {
	positionText, rawID, ok := strings.Cut(key, ":")
	if !ok || len(positionText) != 6 || strings.TrimSpace(rawID) == "" {
		return 0, "", fmt.Errorf("invalid occurrence key %q", key)
	}
	position, err := strconv.Atoi(positionText)
	if err != nil {
		return 0, "", fmt.Errorf("invalid occurrence key %q", key)
	}
	return position, rawID, nil
}

func longMemEvalQAK10Classification(abstention bool, metric *benchmark.SessionRetrievalMetric) (string, error) {
	if abstention {
		return "abstention_unscored", nil
	}
	if metric == nil || metric.K != 10 {
		return "", fmt.Errorf("metric_at_10 with K=10 is required")
	}
	switch {
	case metric.RecallAll == 1:
		return "all_evidence_retrieved", nil
	case metric.RecallAny == 1:
		return "partial_evidence_retrieved", nil
	default:
		return "no_evidence_retrieved", nil
	}
}
