package benchmark

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
)

type LongMemEvalSummary struct {
	RecordCount           int    `json:"record_count"`
	ScoredRecordCount     int    `json:"scored_record_count"`
	AbstentionRecordCount int    `json:"abstention_record_count"`
	SessionCount          int    `json:"session_count"`
	TurnCount             int    `json:"turn_count"`
	RecordSetSHA256       string `json:"record_set_sha256"`
}

func ScanLongMemEval(path string, visit func(LongMemEvalRecord) error) (LongMemEvalSummary, error) {
	file, err := os.Open(path)
	if err != nil {
		return LongMemEvalSummary{}, err
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	opening, err := decoder.Token()
	if err != nil {
		return LongMemEvalSummary{}, fmt.Errorf("decode LongMemEval opening token: %w", err)
	}
	delimiter, ok := opening.(json.Delim)
	if !ok || delimiter != '[' {
		return LongMemEvalSummary{}, fmt.Errorf("LongMemEval dataset must be a JSON array")
	}

	var summary LongMemEvalSummary
	seen := make(map[string]struct{})
	recordIDs := make([]string, 0, 500)
	for decoder.More() {
		var record LongMemEvalRecord
		if err := decoder.Decode(&record); err != nil {
			return LongMemEvalSummary{}, fmt.Errorf("decode LongMemEval record %d: %w", summary.RecordCount, err)
		}
		if err := record.validate(); err != nil {
			return LongMemEvalSummary{}, fmt.Errorf("LongMemEval record %d: %w", summary.RecordCount, err)
		}
		if _, exists := seen[record.QuestionID]; exists {
			return LongMemEvalSummary{}, fmt.Errorf("LongMemEval dataset contains duplicate question_id %q", record.QuestionID)
		}
		seen[record.QuestionID] = struct{}{}
		recordIDs = append(recordIDs, record.QuestionID)
		summary.RecordCount++
		if strings.HasSuffix(record.QuestionID, "_abs") {
			summary.AbstentionRecordCount++
		} else {
			summary.ScoredRecordCount++
		}
		summary.SessionCount += len(record.HaystackSessions)
		for _, session := range record.HaystackSessions {
			summary.TurnCount += len(session)
		}
		if visit != nil {
			if err := visit(record); err != nil {
				return LongMemEvalSummary{}, fmt.Errorf("visit LongMemEval record %q: %w", record.QuestionID, err)
			}
		}
	}

	closing, err := decoder.Token()
	if err != nil {
		return LongMemEvalSummary{}, fmt.Errorf("decode LongMemEval closing token: %w", err)
	}
	delimiter, ok = closing.(json.Delim)
	if !ok || delimiter != ']' {
		return LongMemEvalSummary{}, fmt.Errorf("LongMemEval dataset has an invalid closing token")
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return LongMemEvalSummary{}, fmt.Errorf("LongMemEval dataset contains trailing JSON")
		}
		return LongMemEvalSummary{}, fmt.Errorf("LongMemEval dataset contains trailing JSON: %w", err)
	}
	if summary.RecordCount == 0 {
		return LongMemEvalSummary{}, fmt.Errorf("LongMemEval dataset contains no records")
	}
	summary.RecordSetSHA256 = longMemEvalRecordSetSHA256(recordIDs)
	return summary, nil
}

func longMemEvalRecordSetSHA256(recordIDs []string) string {
	ids := append([]string(nil), recordIDs...)
	sort.Strings(ids)
	canonical := strings.Join(ids, "\n") + "\n"
	digest := sha256.Sum256([]byte(canonical))
	return hex.EncodeToString(digest[:])
}
