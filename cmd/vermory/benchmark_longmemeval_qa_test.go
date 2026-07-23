package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"vermory/internal/app"
	"vermory/internal/benchmark"
	"vermory/internal/provider"
)

func TestBenchmarkLongMemEvalQACommandRejectsUnknownPhaseAndPositionalArguments(t *testing.T) {
	command := newBenchmarkLongMemEvalQACommand()
	command.SetArgs([]string{"--phase", "unknown"})
	if err := command.Execute(); err == nil || !strings.Contains(err.Error(), "phase") {
		t.Fatalf("expected unknown phase rejection, got %v", err)
	}

	command = newBenchmarkLongMemEvalQACommand()
	command.SetArgs([]string{"unexpected"})
	if err := command.Execute(); err == nil {
		t.Fatal("expected positional argument rejection")
	}
}

func TestBenchmarkLongMemEvalQAAllPhaseRunsMiniatureAndResumesWithZeroCalls(t *testing.T) {
	fixture := writeBenchmarkLongMemEvalQACLIData(t)
	reader := &benchmarkLongMemEvalQACLIReader{}
	judge := &benchmarkLongMemEvalQACLIJudge{}
	command := newBenchmarkLongMemEvalQACommandWithProviders(reader, judge)
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetArgs(fixture.args(false))
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if reader.calls != 5 || judge.calls != 5 {
		t.Fatalf("unexpected first-run calls: reader=%d judge=%d", reader.calls, judge.calls)
	}
	if !strings.Contains(output.String(), "phase=all") || !strings.Contains(output.String(), "reader_failed=1") || !strings.Contains(output.String(), "judge_invalid=2") {
		t.Fatalf("unexpected bounded CLI summary: %q", output.String())
	}
	if strings.Contains(output.String(), "answer cli-") {
		t.Fatalf("CLI summary leaked answers: %q", output.String())
	}
	before := benchmarkLongMemEvalQAResultHashes(t, fixture.artifactRoot, fixture.runID)

	resumedReader := &benchmarkLongMemEvalQACLIReader{}
	resumedJudge := &benchmarkLongMemEvalQACLIJudge{}
	resumedCommand := newBenchmarkLongMemEvalQACommandWithProviders(resumedReader, resumedJudge)
	output.Reset()
	resumedCommand.SetOut(&output)
	resumedCommand.SetArgs(fixture.args(true))
	if err := resumedCommand.Execute(); err != nil {
		t.Fatal(err)
	}
	if resumedReader.calls != 0 || resumedJudge.calls != 0 {
		t.Fatalf("resume made provider calls: reader=%d judge=%d", resumedReader.calls, resumedJudge.calls)
	}
	after := benchmarkLongMemEvalQAResultHashes(t, fixture.artifactRoot, fixture.runID)
	if len(before) != len(after) {
		t.Fatalf("result hash set changed: before=%v after=%v", before, after)
	}
	for name, digest := range before {
		if after[name] != digest {
			t.Fatalf("resume changed %s hash: before=%s after=%s", name, digest, after[name])
		}
	}
}

type benchmarkLongMemEvalQACLIReader struct {
	mu    sync.Mutex
	calls int
}

func (p *benchmarkLongMemEvalQACLIReader) Generate(_ context.Context, request provider.GenerateRequest) (provider.GenerateResponse, error) {
	p.mu.Lock()
	p.calls++
	p.mu.Unlock()
	if strings.Contains(request.Prompt, "cli-00:") && strings.HasPrefix(request.ContextPacket, "Governed memory:\n") {
		return provider.GenerateResponse{}, errors.New("planned reader failure")
	}
	recordID := "cli-00"
	if strings.Contains(request.Prompt, "cli-01:") {
		recordID = "cli-01"
	}
	return provider.GenerateResponse{Output: "answer " + recordID, Model: request.Model, RawArtifact: []byte(`{"reader":true}`)}, nil
}

type benchmarkLongMemEvalQACLIJudge struct {
	mu    sync.Mutex
	calls int
}

func (p *benchmarkLongMemEvalQACLIJudge) Generate(_ context.Context, request provider.GenerateRequest) (provider.GenerateResponse, error) {
	p.mu.Lock()
	p.calls++
	p.mu.Unlock()
	if request.ContextPacket != "" || request.System != "" {
		return provider.GenerateResponse{}, errors.New("judge received forbidden context")
	}
	if strings.Contains(request.Prompt, "Question: cli-01:") {
		return provider.GenerateResponse{Output: "yes because it matches", Model: request.Model, RawArtifact: []byte(`{"judge":"invalid"}`)}, nil
	}
	return provider.GenerateResponse{Output: "yes", Model: request.Model, RawArtifact: []byte(`{"judge":"yes"}`)}, nil
}

type benchmarkLongMemEvalQACLIData struct {
	sourcePath        string
	retrievalPath     string
	qualificationPath string
	executionPath     string
	artifactRoot      string
	runID             string
	revision          string
}

func (data benchmarkLongMemEvalQACLIData) args(resume bool) []string {
	args := []string{
		"--source-dataset", data.sourcePath,
		"--retrieval-results", data.retrievalPath,
		"--qualification", data.qualificationPath,
		"--execution", data.executionPath,
		"--artifact-root", data.artifactRoot,
		"--run-id", data.runID,
		"--implementation-revision", data.revision,
		"--phase", "all",
	}
	if resume {
		args = append(args, "--resume")
	}
	return args
}

func writeBenchmarkLongMemEvalQACLIData(t *testing.T) benchmarkLongMemEvalQACLIData {
	t.Helper()
	directory := t.TempDir()
	records := []benchmark.LongMemEvalRecord{
		benchmarkLongMemEvalQACLIRecord("cli-00"),
		benchmarkLongMemEvalQACLIRecord("cli-01"),
	}
	sourceData, err := json.Marshal(records)
	if err != nil {
		t.Fatal(err)
	}
	sourcePath := filepath.Join(directory, "source.json")
	if err := os.WriteFile(sourcePath, sourceData, 0o600); err != nil {
		t.Fatal(err)
	}
	sourceDigest := sha256.Sum256(sourceData)
	summary, err := benchmark.ScanLongMemEval(sourcePath, nil)
	if err != nil {
		t.Fatal(err)
	}
	runID := "longmemeval-qa-cli-miniature"
	revision := strings.Repeat("a", 40)
	execution := benchmark.ExecutionManifest{
		SchemaVersion:             "benchmark-execution/v1",
		Benchmark:                 "LongMemEval",
		DatasetSHA256:             hex.EncodeToString(sourceDigest[:]),
		EvaluationTarget:          benchmark.EvaluationTargetQA,
		ExecutionScope:            benchmark.ExecutionScopeFull,
		ClaimScope:                benchmark.ClaimScopeQualifiedDatasetFull,
		SelectionMode:             benchmark.SelectionModeAllRecords,
		RecordSetSHA256:           summary.RecordSetSHA256,
		ExpectedSessionCount:      summary.SessionCount,
		ExpectedTurnCount:         summary.TurnCount,
		ExpectedScoredRecordCount: summary.ScoredRecordCount,
		HardFactual:               true,
		Scorers: []benchmark.ExecutionScorer{
			{Name: "normalized_exact_match", Class: benchmark.ScorerClassDeterministic},
			{Name: "custom_judge", Class: benchmark.ScorerClassCustomModelJudge},
		},
		RunID:             runID,
		ImplementationRev: revision,
		Conditions:        []string{"plain_token_overlap_k10", "vermory_lexical_k10"},
		Reader:            &benchmark.ExecutionModelConfig{Provider: "test", Model: "reader", Interface: "test", MaxOutputTokens: 64, TimeoutSeconds: 5, Workers: 2, MaxAttempts: 2},
		Judge:             &benchmark.ExecutionModelConfig{Provider: "test", Model: "judge", Interface: "test", ScorerClass: benchmark.ScorerClassCustomModelJudge, MaxOutputTokens: 10, TimeoutSeconds: 5, Workers: 2, MaxAttempts: 2},
		RetrievalInput:    &benchmark.RetrievalExecutionInput{Path: "retrieval-results.jsonl", RunID: "retrieval-miniature", ImplementationRevision: strings.Repeat("b", 40), K: 10},
	}
	results := make([]app.LongMemEvalRetrievalRecordResult, 0, len(records))
	for _, record := range records {
		keys := []string{"000000:" + record.HaystackSessionIDs[0], "000001:" + record.HaystackSessionIDs[1]}
		ids := append([]string(nil), record.HaystackSessionIDs...)
		results = append(results, app.LongMemEvalRetrievalRecordResult{
			SchemaVersion:          "longmemeval-retrieval-checkpoint/v1",
			RunID:                  execution.RetrievalInput.RunID,
			ImplementationRevision: execution.RetrievalInput.ImplementationRevision,
			DatasetSHA256:          execution.DatasetSHA256,
			RecordSetSHA256:        execution.RecordSetSHA256,
			RecordID:               record.QuestionID,
			QuestionType:           record.QuestionType,
			Status:                 "completed",
			Conditions: []app.LongMemEvalRetrievalConditionResult{
				{Condition: "plain_token_overlap", Status: "completed", RankedOccurrenceKeys: keys, RankedSessionIDs: ids, MetricAt10: &benchmark.SessionRetrievalMetric{K: 10, RecallAny: 1, RecallAll: 1}},
				{Condition: "vermory_lexical", Status: "completed", RankedOccurrenceKeys: keys, RankedSessionIDs: ids, MetricAt10: &benchmark.SessionRetrievalMetric{K: 10, RecallAny: 1, RecallAll: 1}},
			},
		})
	}
	var retrieval strings.Builder
	for _, result := range results {
		data, err := json.Marshal(result)
		if err != nil {
			t.Fatal(err)
		}
		retrieval.Write(data)
		retrieval.WriteByte('\n')
	}
	retrievalData := []byte(retrieval.String())
	retrievalPath := filepath.Join(directory, "retrieval-results.jsonl")
	if err := os.WriteFile(retrievalPath, retrievalData, 0o600); err != nil {
		t.Fatal(err)
	}
	retrievalDigest := sha256.Sum256(retrievalData)
	execution.RetrievalInput.SHA256 = hex.EncodeToString(retrievalDigest[:])

	qualification := benchmark.Qualification{
		SchemaVersion:  "benchmark-qualification/v1",
		Benchmark:      "LongMemEval",
		SourceClass:    benchmark.SourceClassOfficialDataset,
		Repository:     benchmark.SourceReference{URL: "https://example.test/repo", Revision: strings.Repeat("c", 40)},
		License:        "MIT",
		Dataset:        benchmark.DatasetSource{URL: "https://example.test/source", Path: "source.json", Revision: strings.Repeat("d", 40), SHA256: execution.DatasetSHA256, SizeBytes: int64(len(sourceData)), RecordCount: len(records)},
		OfficialScorer: benchmark.ScorerSource{URL: "https://example.test/scorer", Path: "evaluate_qa.py", Revision: strings.Repeat("e", 40), SHA256: strings.Repeat("f", 64), Class: benchmark.ScorerClassOfficialModelJudge},
	}
	qualificationPath := filepath.Join(directory, "qualification.json")
	executionPath := filepath.Join(directory, "execution.json")
	execution.QualificationPath = qualificationPath
	writeBenchmarkLongMemEvalQACLIJSON(t, qualificationPath, qualification)
	writeBenchmarkLongMemEvalQACLIJSON(t, executionPath, execution)
	return benchmarkLongMemEvalQACLIData{
		sourcePath: sourcePath, retrievalPath: retrievalPath, qualificationPath: qualificationPath,
		executionPath: executionPath, artifactRoot: filepath.Join(directory, "artifacts"), runID: runID, revision: revision,
	}
}

func benchmarkLongMemEvalQACLIRecord(id string) benchmark.LongMemEvalRecord {
	return benchmark.LongMemEvalRecord{
		QuestionID:         id,
		QuestionType:       "multi-session",
		Question:           id + ": what is the answer?",
		Answer:             "answer " + id,
		QuestionDate:       "2026-07-15",
		HaystackDates:      []string{"2026-07-14", "2026-07-15"},
		HaystackSessionIDs: []string{id + "-session-0", id + "-session-1"},
		HaystackSessions: [][]benchmark.LongMemEvalTurn{
			{{Role: "user", Content: "earlier context"}},
			{{Role: "assistant", Content: "answer " + id, HasAnswer: true}},
		},
		AnswerSessionIDs: []string{id + "-session-1"},
	}
}

func writeBenchmarkLongMemEvalQACLIJSON(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func benchmarkLongMemEvalQAResultHashes(t *testing.T, artifactRoot, runID string) map[string]string {
	t.Helper()
	root := filepath.Join(artifactRoot, "benchmarks", runID)
	result := make(map[string]string)
	for _, name := range []string{"reader-results.jsonl", "judge-results.jsonl", "scores.json", "failure-ledger.json"} {
		data, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256(data)
		result[name] = hex.EncodeToString(digest[:])
	}
	return result
}
