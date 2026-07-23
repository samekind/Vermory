package benchmark

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScanLongMemEvalCountsAndCanonicalizesRecordIDs(t *testing.T) {
	records := []LongMemEvalRecord{
		streamTestRecord("record-b", "session-b", 2),
		streamTestRecord("record-a", "session-a", 1),
	}
	forward := writeLongMemEvalStreamFixture(t, records, "")
	reverse := writeLongMemEvalStreamFixture(t, []LongMemEvalRecord{records[1], records[0]}, "")

	var visited []string
	summary, err := ScanLongMemEval(forward, func(record LongMemEvalRecord) error {
		visited = append(visited, record.QuestionID)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	reversed, err := ScanLongMemEval(reverse, nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(visited, ",") != "record-b,record-a" {
		t.Fatalf("visitor order changed: %v", visited)
	}
	if summary.RecordCount != 2 || summary.ScoredRecordCount != 2 || summary.AbstentionRecordCount != 0 || summary.SessionCount != 2 || summary.TurnCount != 3 {
		t.Fatalf("unexpected summary: %#v", summary)
	}
	if summary.RecordSetSHA256 == "" || summary.RecordSetSHA256 != reversed.RecordSetSHA256 {
		t.Fatalf("record-set digest is not canonical: forward=%#v reverse=%#v", summary, reversed)
	}
}

func TestScanLongMemEvalRejectsDuplicateIDs(t *testing.T) {
	record := streamTestRecord("duplicate", "session", 1)
	path := writeLongMemEvalStreamFixture(t, []LongMemEvalRecord{record, record}, "")
	if _, err := ScanLongMemEval(path, nil); err == nil || !strings.Contains(err.Error(), "duplicate question_id") {
		t.Fatalf("expected duplicate rejection, got %v", err)
	}
}

func TestScanLongMemEvalAcceptsRepeatedSessionOccurrences(t *testing.T) {
	record := streamTestRecord("record", "repeated", 1)
	record.HaystackDates = append(record.HaystackDates, "2026/07/15")
	record.HaystackSessionIDs = append(record.HaystackSessionIDs, "repeated")
	record.HaystackSessions = append(record.HaystackSessions, append([]LongMemEvalTurn(nil), record.HaystackSessions[0]...))
	path := writeLongMemEvalStreamFixture(t, []LongMemEvalRecord{record}, "")
	summary, err := ScanLongMemEval(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	if summary.SessionCount != 2 || summary.TurnCount != 2 {
		t.Fatalf("repeated occurrences were not retained: %#v", summary)
	}
}

func TestScanLongMemEvalRetainsUnlabeledEmptyTurns(t *testing.T) {
	record := streamTestRecord("record", "session", 1)
	record.HaystackSessions[0] = []LongMemEvalTurn{
		{Role: "user", Content: ""},
		{Role: "assistant", Content: "semantic content"},
	}
	path := writeLongMemEvalStreamFixture(t, []LongMemEvalRecord{record}, "")
	summary, err := ScanLongMemEval(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	if summary.TurnCount != 2 {
		t.Fatalf("empty source turn was not retained in the count: %#v", summary)
	}
	record.HaystackSessions[0][0].HasAnswer = true
	path = writeLongMemEvalStreamFixture(t, []LongMemEvalRecord{record}, "")
	if _, err := ScanLongMemEval(path, nil); err == nil || !strings.Contains(err.Error(), "empty answer-labeled turn") {
		t.Fatalf("expected empty answer-turn rejection, got %v", err)
	}
	record.HaystackSessions[0] = []LongMemEvalTurn{{Role: "user", Content: ""}}
	record.HaystackSessions[0][0].HasAnswer = false
	path = writeLongMemEvalStreamFixture(t, []LongMemEvalRecord{record}, "")
	if _, err := ScanLongMemEval(path, nil); err == nil || !strings.Contains(err.Error(), "no non-empty turns") {
		t.Fatalf("expected empty-session rejection, got %v", err)
	}
}

func TestScanLongMemEvalRejectsMalformedRecordsAndTrailingJSON(t *testing.T) {
	malformed := streamTestRecord("malformed", "session", 1)
	malformed.HaystackDates = nil
	path := writeLongMemEvalStreamFixture(t, []LongMemEvalRecord{malformed}, "")
	if _, err := ScanLongMemEval(path, nil); err == nil || !strings.Contains(err.Error(), "parallel session arrays") {
		t.Fatalf("expected malformed-record rejection, got %v", err)
	}

	path = writeLongMemEvalStreamFixture(t, []LongMemEvalRecord{streamTestRecord("valid", "session", 1)}, `{}`)
	if _, err := ScanLongMemEval(path, nil); err == nil || !strings.Contains(err.Error(), "trailing JSON") {
		t.Fatalf("expected trailing-data rejection, got %v", err)
	}
}

func TestScanLongMemEvalPropagatesVisitorFailure(t *testing.T) {
	path := writeLongMemEvalStreamFixture(t, []LongMemEvalRecord{streamTestRecord("record", "session", 1)}, "")
	want := errors.New("stop after record")
	if _, err := ScanLongMemEval(path, func(LongMemEvalRecord) error { return want }); !errors.Is(err, want) {
		t.Fatalf("expected visitor error, got %v", err)
	}
}

func TestScanQualifiedLongMemEvalSMetadata(t *testing.T) {
	path := os.Getenv("VERMORY_LONGMEMEVAL_S_DATASET")
	if path == "" {
		t.Skip("VERMORY_LONGMEMEVAL_S_DATASET is not set")
	}
	summary, err := ScanLongMemEval(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	if summary.RecordCount != 500 || summary.ScoredRecordCount != 470 || summary.AbstentionRecordCount != 30 || summary.SessionCount != 23867 || summary.TurnCount != 246750 {
		t.Fatalf("unexpected qualified source summary: %#v", summary)
	}
	if summary.RecordSetSHA256 != "f038965c54b03632f86a59104dd77848b66e3f80c08d5fbabdd3984d16457811" {
		t.Fatalf("unexpected qualified record-set digest: %s", summary.RecordSetSHA256)
	}
}

func streamTestRecord(questionID, sessionID string, turns int) LongMemEvalRecord {
	sessionTurns := make([]LongMemEvalTurn, turns)
	for index := range sessionTurns {
		sessionTurns[index] = LongMemEvalTurn{Role: "user", Content: "content"}
	}
	return LongMemEvalRecord{
		QuestionID:         questionID,
		QuestionType:       "single-session-user",
		Question:           "What happened?",
		Answer:             "content",
		QuestionDate:       "2026/07/15",
		HaystackDates:      []string{"2026/07/14"},
		HaystackSessionIDs: []string{sessionID},
		HaystackSessions:   [][]LongMemEvalTurn{sessionTurns},
		AnswerSessionIDs:   []string{sessionID},
	}
}

func writeLongMemEvalStreamFixture(t *testing.T, records []LongMemEvalRecord, suffix string) string {
	t.Helper()
	data, err := json.Marshal(records)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "longmemeval.json")
	if err := os.WriteFile(path, append(data, suffix...), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
