package benchmark

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestLoadLongMemEvalRejectsMisalignedSessionArrays(t *testing.T) {
	path := filepath.Join(t.TempDir(), "invalid.json")
	data := `[{"question_id":"q1","question_type":"single-session-user","question":"Where?","answer":"Paris","question_date":"2024/01/01","haystack_dates":["2023/12/01"],"haystack_session_ids":[],"haystack_sessions":[[{"role":"user","content":"I went to Paris."}]],"answer_session_ids":["s1"]}]`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadLongMemEval(path); err == nil || !strings.Contains(err.Error(), "parallel session arrays") {
		t.Fatalf("expected parallel-array validation error, got %v", err)
	}
}

func TestSelectRecordsPreservesFrozenIDOrder(t *testing.T) {
	records := []LongMemEvalRecord{
		{QuestionID: "a"},
		{QuestionID: "b"},
		{QuestionID: "c"},
	}
	selected, err := SelectRecords(records, []string{"c", "a"})
	if err != nil {
		t.Fatal(err)
	}
	got := []string{selected[0].QuestionID, selected[1].QuestionID}
	if !reflect.DeepEqual(got, []string{"c", "a"}) {
		t.Fatalf("expected frozen order [c a], got %v", got)
	}
}

func TestSelectRecordsRejectsDuplicateAndMissingIDs(t *testing.T) {
	records := []LongMemEvalRecord{{QuestionID: "a"}}
	if _, err := SelectRecords(records, []string{"a", "a"}); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("expected duplicate-id rejection, got %v", err)
	}
	if _, err := SelectRecords(records, []string{"missing"}); err == nil || !strings.Contains(err.Error(), "missing") {
		t.Fatalf("expected missing-id rejection, got %v", err)
	}
}

func TestRetrieveSessionsRanksLexicalEvidenceDeterministically(t *testing.T) {
	record := LongMemEvalRecord{
		Question: "Where did I move after relocation?",
		HaystackDates: []string{
			"2024/01/01",
			"2024/02/01",
			"2024/03/01",
		},
		HaystackSessionIDs: []string{"noise", "answer", "tie"},
		HaystackSessions: [][]LongMemEvalTurn{
			{{Role: "user", Content: "I bought a bicycle."}},
			{{Role: "user", Content: "After the relocation I moved to the suburbs."}},
			{{Role: "user", Content: "The relocation paperwork is complete."}},
		},
	}
	retrieved := RetrieveSessions(record, 2)
	got := []string{retrieved[0].ID, retrieved[1].ID}
	if !reflect.DeepEqual(got, []string{"answer", "tie"}) {
		t.Fatalf("expected lexical order [answer tie], got %v", got)
	}
	if strings.Contains(retrieved[0].SemanticText(), "has_answer") {
		t.Fatalf("semantic text leaked benchmark metadata: %q", retrieved[0].SemanticText())
	}
}

func TestScoreAnswerUsesBestExplicitAnswerVariant(t *testing.T) {
	record := LongMemEvalRecord{
		QuestionID: "q1",
		Answer:     "25 minutes and 50 seconds (or 25:50)",
	}
	score := ScoreAnswer(record, "My personal best was 25:50.")
	if !score.ExactMatch {
		t.Fatalf("expected alternate answer exact match, got %#v", score)
	}
	if score.TokenF1 != 1 || score.AnswerTokenRecall != 1 {
		t.Fatalf("expected perfect token metrics, got %#v", score)
	}
}

func TestScoreAnswerReportsPartialTokenMetricsWithoutTurningThemIntoPassFail(t *testing.T) {
	record := LongMemEvalRecord{QuestionID: "q2", Answer: "GPS system not functioning correctly"}
	score := ScoreAnswer(record, "The GPS system failed.")
	if score.ExactMatch {
		t.Fatalf("partial answer must not be exact: %#v", score)
	}
	if score.TokenF1 <= 0 || score.TokenF1 >= 1 {
		t.Fatalf("expected partial token F1, got %#v", score)
	}
	if score.AnswerTokenRecall <= 0 || score.AnswerTokenRecall >= 1 {
		t.Fatalf("expected partial answer recall, got %#v", score)
	}
}

func TestScoreAnswerDetectsAbstentionDeterministically(t *testing.T) {
	record := LongMemEvalRecord{
		QuestionID: "0862e8bf_abs",
		Answer:     "You did not mention this information.",
	}
	score := ScoreAnswer(record, "It cannot be determined.")
	if !score.AbstentionExpected || !score.AbstentionDetected {
		t.Fatalf("expected deterministic abstention detection, got %#v", score)
	}
}

func TestFrozenLongMemEvalFixtureMatchesExecutionManifest(t *testing.T) {
	qualification, err := LoadQualification("../../casebook/benchmarks/qualifications/longmemeval-cleaned-oracle.json")
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := LoadExecution("../../casebook/benchmarks/executions/longmemeval-oracle-sample.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateExecution(qualification, manifest); err != nil {
		t.Fatal(err)
	}
	fixturePath := "../../casebook/benchmarks/fixtures/longmemeval-oracle-sample.json"
	if err := VerifyFileSHA256(fixturePath, manifest.FixtureSHA256); err != nil {
		t.Fatal(err)
	}
	records, err := LoadLongMemEval(fixturePath)
	if err != nil {
		t.Fatal(err)
	}
	selected, err := SelectRecords(records, manifest.SelectedRecordIDs)
	if err != nil {
		t.Fatal(err)
	}
	if len(selected) != len(manifest.SelectedRecordIDs) {
		t.Fatalf("expected %d frozen records, got %d", len(manifest.SelectedRecordIDs), len(selected))
	}
}
