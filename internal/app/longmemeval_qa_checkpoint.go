package app

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"vermory/internal/benchmark"
	"vermory/internal/provider"
)

const (
	longMemEvalQACheckpointSchema = "longmemeval-qa-checkpoint/v1"
	longMemEvalQAReaderCompleted  = "reader_completed"
	longMemEvalQAReaderFailed     = "reader_failed"
	longMemEvalQAAttemptCompleted = "completed"
	longMemEvalQAAttemptFailed    = "failed"
)

type LongMemEvalQAAttempt struct {
	Number            int                  `json:"number"`
	StartedAt         string               `json:"started_at,omitempty"`
	DurationMillis    int64                `json:"duration_ms"`
	Status            string               `json:"status"`
	Output            string               `json:"output,omitempty"`
	Error             string               `json:"error,omitempty"`
	ProviderModel     string               `json:"provider_model,omitempty"`
	Usage             *provider.TokenUsage `json:"usage,omitempty"`
	RawArtifactURI    string               `json:"raw_artifact_uri,omitempty"`
	RawArtifactSHA256 string               `json:"raw_artifact_sha256,omitempty"`
	RawArtifactBytes  int64                `json:"raw_artifact_bytes,omitempty"`
}

type LongMemEvalQAJudgeState struct {
	Config        benchmark.ExecutionModelConfig `json:"config"`
	PromptSHA256  string                         `json:"prompt_sha256,omitempty"`
	Status        string                         `json:"status"`
	Correct       *bool                          `json:"correct,omitempty"`
	Output        string                         `json:"output,omitempty"`
	ProviderModel string                         `json:"provider_model,omitempty"`
	Attempts      []LongMemEvalQAAttempt         `json:"attempts,omitempty"`
}

type LongMemEvalQACheckpoint struct {
	SchemaVersion           string                         `json:"schema_version"`
	RunID                   string                         `json:"run_id"`
	ImplementationRevision  string                         `json:"implementation_revision"`
	DatasetSHA256           string                         `json:"dataset_sha256"`
	RecordSetSHA256         string                         `json:"record_set_sha256"`
	RetrievalSHA256         string                         `json:"retrieval_sha256"`
	RetrievalRunID          string                         `json:"retrieval_run_id"`
	RetrievalRevision       string                         `json:"retrieval_revision"`
	K                       int                            `json:"k"`
	Reader                  benchmark.ExecutionModelConfig `json:"reader"`
	PromptSHA256            string                         `json:"prompt_sha256"`
	ContextSHA256           string                         `json:"context_sha256"`
	RecordID                string                         `json:"record_id"`
	QuestionType            string                         `json:"question_type"`
	Abstention              bool                           `json:"abstention"`
	Condition               string                         `json:"condition"`
	RetrievalClassification string                         `json:"retrieval_classification"`
	RankedOccurrenceKeys    []string                       `json:"ranked_occurrence_keys"`
	RankedSessionIDs        []string                       `json:"ranked_session_ids"`
	ReaderStatus            string                         `json:"reader_status"`
	Response                string                         `json:"response,omitempty"`
	ProviderModel           string                         `json:"provider_model,omitempty"`
	Attempts                []LongMemEvalQAAttempt         `json:"attempts"`
	Score                   *benchmark.DeterministicScore  `json:"score,omitempty"`
	Judge                   *LongMemEvalQAJudgeState       `json:"judge,omitempty"`
}

type longMemEvalQACheckpointContract struct {
	RunID                  string
	ImplementationRevision string
	DatasetSHA256          string
	RecordSetSHA256        string
	RetrievalSHA256        string
	RetrievalRunID         string
	RetrievalRevision      string
	K                      int
	Reader                 benchmark.ExecutionModelConfig
}

type longMemEvalQACheckpointWriter func(file *os.File, data []byte) error

func longMemEvalQACheckpointPath(root, runID, recordID, condition string) (string, error) {
	for value, label := range map[string]string{
		runID:     "run ID",
		recordID:  "record ID",
		condition: "condition",
	} {
		if err := validateLongMemEvalRetrievalSegment(value, label); err != nil {
			return "", err
		}
	}
	if condition != longMemEvalQAPlainCondition && condition != longMemEvalQAVermoryCondition && condition != longMemEvalQAVectorCondition {
		return "", fmt.Errorf("invalid LongMemEval QA condition %q", condition)
	}
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	return filepath.Join(absoluteRoot, "benchmarks", runID, "checkpoints", recordID, condition+".json"), nil
}

func loadLongMemEvalQACheckpoint(path string) (LongMemEvalQACheckpoint, error) {
	file, err := os.Open(path)
	if err != nil {
		return LongMemEvalQACheckpoint{}, err
	}
	defer file.Close()
	decoder := json.NewDecoder(bufio.NewReader(file))
	decoder.DisallowUnknownFields()
	var checkpoint LongMemEvalQACheckpoint
	if err := decoder.Decode(&checkpoint); err != nil {
		return LongMemEvalQACheckpoint{}, fmt.Errorf("decode LongMemEval QA checkpoint: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return LongMemEvalQACheckpoint{}, fmt.Errorf("decode LongMemEval QA checkpoint: trailing JSON")
		}
		return LongMemEvalQACheckpoint{}, fmt.Errorf("decode LongMemEval QA checkpoint: %w", err)
	}
	return checkpoint, nil
}

func writeLongMemEvalQACheckpoint(path string, checkpoint LongMemEvalQACheckpoint) error {
	return writeLongMemEvalQACheckpointWith(path, checkpoint, func(file *os.File, data []byte) error {
		_, err := file.Write(data)
		return err
	})
}

func writeLongMemEvalQACheckpointWith(path string, checkpoint LongMemEvalQACheckpoint, writer longMemEvalQACheckpointWriter) error {
	data, err := json.MarshalIndent(checkpoint, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".vermory-qa-checkpoint-*")
	if err != nil {
		return err
	}
	tempPath := file.Name()
	keep := false
	defer func() {
		if !keep {
			_ = os.Remove(tempPath)
		}
	}()
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return err
	}
	if err := writer(file, data); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := os.Rename(tempPath, path); err != nil {
		return err
	}
	keep = true
	return nil
}

func validateLongMemEvalQACheckpoint(checkpoint LongMemEvalQACheckpoint, task LongMemEvalQATask, contract longMemEvalQACheckpointContract) error {
	if checkpoint.SchemaVersion != longMemEvalQACheckpointSchema {
		return fmt.Errorf("checkpoint schema_version is %q", checkpoint.SchemaVersion)
	}
	checks := []struct {
		name string
		got  string
		want string
	}{
		{"run_id", checkpoint.RunID, contract.RunID},
		{"implementation_revision", checkpoint.ImplementationRevision, contract.ImplementationRevision},
		{"dataset_sha256", checkpoint.DatasetSHA256, contract.DatasetSHA256},
		{"record_set_sha256", checkpoint.RecordSetSHA256, contract.RecordSetSHA256},
		{"retrieval_sha256", checkpoint.RetrievalSHA256, contract.RetrievalSHA256},
		{"retrieval_run_id", checkpoint.RetrievalRunID, contract.RetrievalRunID},
		{"retrieval_revision", checkpoint.RetrievalRevision, contract.RetrievalRevision},
		{"prompt_sha256", checkpoint.PromptSHA256, task.PromptSHA256},
		{"context_sha256", checkpoint.ContextSHA256, task.ContextSHA256},
		{"record_id", checkpoint.RecordID, task.RecordID},
		{"question_type", checkpoint.QuestionType, task.QuestionType},
		{"condition", checkpoint.Condition, task.Condition},
		{"retrieval_classification", checkpoint.RetrievalClassification, task.RetrievalClassification},
	}
	for _, check := range checks {
		if check.got != check.want {
			return fmt.Errorf("checkpoint %s is %q, want %q", check.name, check.got, check.want)
		}
	}
	if checkpoint.K != contract.K {
		return fmt.Errorf("checkpoint k is %d, want %d", checkpoint.K, contract.K)
	}
	if checkpoint.Reader != contract.Reader {
		return fmt.Errorf("checkpoint reader differs from frozen reader config")
	}
	if checkpoint.Abstention != task.Abstention {
		return fmt.Errorf("checkpoint abstention is %t, want %t", checkpoint.Abstention, task.Abstention)
	}
	if !slices.Equal(checkpoint.RankedOccurrenceKeys, task.RankedOccurrenceKeys) || !slices.Equal(checkpoint.RankedSessionIDs, task.RankedSessionIDs) {
		return fmt.Errorf("checkpoint ranking differs from frozen retrieval ranking")
	}
	if len(checkpoint.Attempts) == 0 {
		return fmt.Errorf("checkpoint attempts are required")
	}
	for index, attempt := range checkpoint.Attempts {
		if attempt.Number != index+1 {
			return fmt.Errorf("checkpoint attempt %d number is %d", index, attempt.Number)
		}
		if len(attempt.Error) > 1000 {
			return fmt.Errorf("checkpoint attempt %d error exceeds 1000 bytes", attempt.Number)
		}
	}
	switch checkpoint.ReaderStatus {
	case longMemEvalQAReaderCompleted:
		if strings.TrimSpace(checkpoint.Response) == "" || checkpoint.Score == nil {
			return fmt.Errorf("checkpoint completed reader requires response and score")
		}
		if checkpoint.Attempts[len(checkpoint.Attempts)-1].Status != longMemEvalQAAttemptCompleted {
			return fmt.Errorf("checkpoint completed reader final attempt is not completed")
		}
	case longMemEvalQAReaderFailed:
		if checkpoint.Response != "" || checkpoint.Score != nil {
			return fmt.Errorf("checkpoint failed reader cannot contain response or score")
		}
		if len(checkpoint.Attempts) != contract.Reader.MaxAttempts {
			return fmt.Errorf("checkpoint failed reader has %d attempts, want %d", len(checkpoint.Attempts), contract.Reader.MaxAttempts)
		}
		for _, attempt := range checkpoint.Attempts {
			if attempt.Status != longMemEvalQAAttemptFailed {
				return fmt.Errorf("checkpoint failed reader attempt %d has status %q", attempt.Number, attempt.Status)
			}
		}
	default:
		return fmt.Errorf("checkpoint reader_status is %q", checkpoint.ReaderStatus)
	}
	return nil
}
