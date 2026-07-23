package app

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"vermory/internal/benchmark"
)

func TestLongMemEvalQACheckpointAtomicWritePreservesPreviousBytesOnFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "checkpoints", "record-a", longMemEvalQAPlainCondition+".json")
	checkpoint := longMemEvalQATestCheckpoint()
	if err := writeLongMemEvalQACheckpoint(path, checkpoint); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	checkpoint.Response = "changed response"
	err = writeLongMemEvalQACheckpointWith(path, checkpoint, func(file *os.File, data []byte) error {
		if _, writeErr := file.Write(data[:len(data)/2]); writeErr != nil {
			return writeErr
		}
		return errors.New("injected checkpoint write failure")
	})
	if err == nil || !strings.Contains(err.Error(), "injected checkpoint write failure") {
		t.Fatalf("expected injected write failure, got %v", err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatal("failed atomic update changed the previous checkpoint bytes")
	}
	matches, err := filepath.Glob(filepath.Join(filepath.Dir(path), ".vermory-qa-checkpoint-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("temporary checkpoint files were not removed: %v", matches)
	}
}

func TestLongMemEvalQACheckpointRejectsFrozenIdentityMismatch(t *testing.T) {
	checkpoint := longMemEvalQATestCheckpoint()
	task := longMemEvalQATestTask()
	contract := longMemEvalQATestCheckpointContract()
	if err := validateLongMemEvalQACheckpoint(checkpoint, task, contract); err != nil {
		t.Fatalf("valid checkpoint rejected: %v", err)
	}

	tests := map[string]func(*LongMemEvalQACheckpoint){
		"run_id":                  func(value *LongMemEvalQACheckpoint) { value.RunID = "other-run" },
		"implementation_revision": func(value *LongMemEvalQACheckpoint) { value.ImplementationRevision = strings.Repeat("9", 40) },
		"dataset_sha256":          func(value *LongMemEvalQACheckpoint) { value.DatasetSHA256 = strings.Repeat("8", 64) },
		"record_set_sha256":       func(value *LongMemEvalQACheckpoint) { value.RecordSetSHA256 = strings.Repeat("7", 64) },
		"retrieval_sha256":        func(value *LongMemEvalQACheckpoint) { value.RetrievalSHA256 = strings.Repeat("6", 64) },
		"retrieval_run_id":        func(value *LongMemEvalQACheckpoint) { value.RetrievalRunID = "other-retrieval" },
		"retrieval_revision":      func(value *LongMemEvalQACheckpoint) { value.RetrievalRevision = strings.Repeat("5", 40) },
		"k":                       func(value *LongMemEvalQACheckpoint) { value.K = 9 },
		"reader":                  func(value *LongMemEvalQACheckpoint) { value.Reader.Model = "other-model" },
		"prompt_sha256":           func(value *LongMemEvalQACheckpoint) { value.PromptSHA256 = strings.Repeat("4", 64) },
		"context_sha256":          func(value *LongMemEvalQACheckpoint) { value.ContextSHA256 = strings.Repeat("3", 64) },
		"record_id":               func(value *LongMemEvalQACheckpoint) { value.RecordID = "other-record" },
		"question_type":           func(value *LongMemEvalQACheckpoint) { value.QuestionType = "other-type" },
		"condition":               func(value *LongMemEvalQACheckpoint) { value.Condition = longMemEvalQAVermoryCondition },
		"ranking":                 func(value *LongMemEvalQACheckpoint) { value.RankedOccurrenceKeys[0] = "000001:other" },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			changed := checkpoint
			changed.Reader = checkpoint.Reader
			changed.RankedOccurrenceKeys = append([]string(nil), checkpoint.RankedOccurrenceKeys...)
			changed.RankedSessionIDs = append([]string(nil), checkpoint.RankedSessionIDs...)
			mutate(&changed)
			if err := validateLongMemEvalQACheckpoint(changed, task, contract); err == nil || !strings.Contains(err.Error(), name) {
				t.Fatalf("expected %s rejection, got %v", name, err)
			}
		})
	}
}

func TestLongMemEvalQACheckpointAcceptsTerminalReaderFailure(t *testing.T) {
	checkpoint := longMemEvalQATestCheckpoint()
	checkpoint.ReaderStatus = longMemEvalQAReaderFailed
	checkpoint.Response = ""
	checkpoint.ProviderModel = ""
	checkpoint.Score = nil
	checkpoint.Attempts = []LongMemEvalQAAttempt{
		{Number: 1, Status: longMemEvalQAAttemptFailed, Error: "provider unavailable"},
		{Number: 2, Status: longMemEvalQAAttemptFailed, Error: "provider unavailable"},
		{Number: 3, Status: longMemEvalQAAttemptFailed, Error: "provider unavailable"},
	}
	if err := validateLongMemEvalQACheckpoint(checkpoint, longMemEvalQATestTask(), longMemEvalQATestCheckpointContract()); err != nil {
		t.Fatalf("terminal reader failure rejected: %v", err)
	}
}

func longMemEvalQATestTask() LongMemEvalQATask {
	return LongMemEvalQATask{
		Record: benchmark.LongMemEvalRecord{
			QuestionID:   "record-a",
			QuestionType: "multi-session",
			Question:     "What happened?",
			Answer:       "the deployment completed",
		},
		RecordID:                "record-a",
		QuestionType:            "multi-session",
		Condition:               longMemEvalQAPlainCondition,
		Question:                "What happened?",
		SystemPrompt:            longMemEvalQASystemPrompt,
		ContextPacket:           "Retrieved conversation memory:\nDate: 2026-07-15\nuser: deployment completed",
		PromptSHA256:            strings.Repeat("1", 64),
		ContextSHA256:           strings.Repeat("2", 64),
		RetrievalClassification: "all_evidence_retrieved",
		RankedOccurrenceKeys:    []string{"000000:session-a"},
		RankedSessionIDs:        []string{"session-a"},
	}
}

func longMemEvalQATestCheckpointContract() longMemEvalQACheckpointContract {
	return longMemEvalQACheckpointContract{
		RunID:                  "qa-test-run",
		ImplementationRevision: strings.Repeat("a", 40),
		DatasetSHA256:          strings.Repeat("b", 64),
		RecordSetSHA256:        strings.Repeat("c", 64),
		RetrievalSHA256:        strings.Repeat("d", 64),
		RetrievalRunID:         "retrieval-test-run",
		RetrievalRevision:      strings.Repeat("e", 40),
		K:                      10,
		Reader:                 benchmark.ExecutionModelConfig{Provider: "test", Model: "reader", Interface: "test", MaxOutputTokens: 128, TimeoutSeconds: 10, Workers: 4, MaxAttempts: 3},
	}
}

func longMemEvalQATestCheckpoint() LongMemEvalQACheckpoint {
	task := longMemEvalQATestTask()
	contract := longMemEvalQATestCheckpointContract()
	score := benchmark.ScoreAnswer(task.Record, "the deployment completed")
	return LongMemEvalQACheckpoint{
		SchemaVersion:           longMemEvalQACheckpointSchema,
		RunID:                   contract.RunID,
		ImplementationRevision:  contract.ImplementationRevision,
		DatasetSHA256:           contract.DatasetSHA256,
		RecordSetSHA256:         contract.RecordSetSHA256,
		RetrievalSHA256:         contract.RetrievalSHA256,
		RetrievalRunID:          contract.RetrievalRunID,
		RetrievalRevision:       contract.RetrievalRevision,
		K:                       contract.K,
		Reader:                  contract.Reader,
		PromptSHA256:            task.PromptSHA256,
		ContextSHA256:           task.ContextSHA256,
		RecordID:                task.RecordID,
		QuestionType:            task.QuestionType,
		Condition:               task.Condition,
		RetrievalClassification: task.RetrievalClassification,
		RankedOccurrenceKeys:    append([]string(nil), task.RankedOccurrenceKeys...),
		RankedSessionIDs:        append([]string(nil), task.RankedSessionIDs...),
		ReaderStatus:            longMemEvalQAReaderCompleted,
		Response:                "the deployment completed",
		ProviderModel:           "reader",
		Attempts:                []LongMemEvalQAAttempt{{Number: 1, Status: longMemEvalQAAttemptCompleted}},
		Score:                   &score,
	}
}
