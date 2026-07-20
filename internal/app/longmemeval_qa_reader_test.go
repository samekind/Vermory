package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"vermory/internal/benchmark"
	"vermory/internal/provider"
)

func TestRunLongMemEvalQAReaderBoundsWorkersRetriesAndResumesWithoutCalls(t *testing.T) {
	fixture := writeLongMemEvalQAReaderFixture(t, 50)
	reader := &longMemEvalQARecordingProvider{delay: 2 * time.Millisecond}
	opts := fixture.options(reader)
	opts.RetrySleeper = func(context.Context, time.Duration) error { return nil }

	summary, err := RunLongMemEvalQAReader(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Total != 100 || summary.Completed != 99 || summary.Failed != 1 || summary.Resumed != 0 {
		t.Fatalf("unexpected first-run summary: %#v", summary)
	}
	if reader.maxActive > opts.Execution.Reader.Workers {
		t.Fatalf("provider concurrency reached %d, workers=%d", reader.maxActive, opts.Execution.Reader.Workers)
	}
	if reader.calls != 102 {
		t.Fatalf("provider calls=%d want 102", reader.calls)
	}
	if reader.callsByKey["record-00|"+longMemEvalQAVermoryCondition] != opts.Execution.Reader.MaxAttempts {
		t.Fatalf("planned failure attempts=%d want %d", reader.callsByKey["record-00|"+longMemEvalQAVermoryCondition], opts.Execution.Reader.MaxAttempts)
	}
	if reader.callsByKey["record-01|"+longMemEvalQAPlainCondition] != 1 {
		t.Fatalf("poor completed answer was retried %d times", reader.callsByKey["record-01|"+longMemEvalQAPlainCondition])
	}

	before := readLongMemEvalQACheckpointTree(t, opts.ArtifactRoot, opts.RunID)
	resumedProvider := &longMemEvalQARecordingProvider{}
	resumedOpts := fixture.options(resumedProvider)
	resumedOpts.Resume = true
	resumedOpts.RetrySleeper = func(context.Context, time.Duration) error { return nil }
	resumed, err := RunLongMemEvalQAReader(context.Background(), resumedOpts)
	if err != nil {
		t.Fatal(err)
	}
	if resumedProvider.calls != 0 {
		t.Fatalf("resume made %d provider calls", resumedProvider.calls)
	}
	if resumed.Total != 100 || resumed.Completed != 99 || resumed.Failed != 1 || resumed.Resumed != 100 {
		t.Fatalf("unexpected resume summary: %#v", resumed)
	}
	after := readLongMemEvalQACheckpointTree(t, opts.ArtifactRoot, opts.RunID)
	if !equalStringByteMaps(before, after) {
		t.Fatal("resume changed checkpoint bytes")
	}
}

func TestRunLongMemEvalQAReaderRejectsMismatchedResumeBeforeProviderCall(t *testing.T) {
	fixture := writeLongMemEvalQAReaderFixture(t, 1)
	reader := &longMemEvalQARecordingProvider{}
	opts := fixture.options(reader)
	opts.RetrySleeper = func(context.Context, time.Duration) error { return nil }
	if _, err := RunLongMemEvalQAReader(context.Background(), opts); err != nil {
		t.Fatal(err)
	}

	path, err := longMemEvalQACheckpointPath(opts.ArtifactRoot, opts.RunID, "record-00", longMemEvalQAPlainCondition)
	if err != nil {
		t.Fatal(err)
	}
	checkpoint, err := loadLongMemEvalQACheckpoint(path)
	if err != nil {
		t.Fatal(err)
	}
	checkpoint.ContextSHA256 = strings.Repeat("f", 64)
	if err := writeLongMemEvalQACheckpoint(path, checkpoint); err != nil {
		t.Fatal(err)
	}

	resumedProvider := &longMemEvalQARecordingProvider{}
	resumedOpts := fixture.options(resumedProvider)
	resumedOpts.Resume = true
	if _, err := RunLongMemEvalQAReader(context.Background(), resumedOpts); err == nil || !strings.Contains(err.Error(), "context_sha256") {
		t.Fatalf("expected resume mismatch rejection, got %v", err)
	}
	if resumedProvider.calls != 0 {
		t.Fatalf("mismatched resume made %d provider calls", resumedProvider.calls)
	}
}

func TestRunLongMemEvalQAReaderDoesNotRetryNonRetryableProviderError(t *testing.T) {
	fixture := writeLongMemEvalQAReaderFixture(t, 1)
	reader := &longMemEvalQANonRetryableProvider{}
	opts := fixture.options(reader)
	opts.RetrySleeper = func(context.Context, time.Duration) error { return nil }

	summary, err := RunLongMemEvalQAReader(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Total != 2 || summary.Completed != 1 || summary.Failed != 1 {
		t.Fatalf("unexpected summary: %#v", summary)
	}
	if reader.calls != 2 {
		t.Fatalf("provider calls=%d want 2", reader.calls)
	}

	path, err := longMemEvalQACheckpointPath(opts.ArtifactRoot, opts.RunID, "record-00", longMemEvalQAVermoryCondition)
	if err != nil {
		t.Fatal(err)
	}
	checkpoint, err := loadLongMemEvalQACheckpoint(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(checkpoint.Attempts) != 1 || checkpoint.Attempts[0].Retryable == nil || *checkpoint.Attempts[0].Retryable {
		t.Fatalf("unexpected permanent-failure checkpoint: %#v", checkpoint.Attempts)
	}
}

func TestRunLongMemEvalQAReaderStopsAtFrozenTerminalFailureLimit(t *testing.T) {
	fixture := writeLongMemEvalQAReaderFixture(t, 50)
	fixture.execution.Reader.MaxTerminalFailures = 1
	reader := &longMemEvalQARecordingProvider{delay: 2 * time.Millisecond}
	opts := fixture.options(reader)
	opts.RetrySleeper = func(context.Context, time.Duration) error { return nil }

	summary, err := RunLongMemEvalQAReader(context.Background(), opts)
	if err == nil || !strings.Contains(err.Error(), "reader terminal failure limit 1 reached") {
		t.Fatalf("expected terminal-failure stop, got summary=%#v err=%v", summary, err)
	}
	if summary.Failed < 1 || summary.Total >= 100 {
		t.Fatalf("reader did not stop early: %#v", summary)
	}
	if reader.calls >= 102 {
		t.Fatalf("reader did not bound provider work: calls=%d", reader.calls)
	}
}

type longMemEvalQARecordingProvider struct {
	mu         sync.Mutex
	calls      int
	active     int
	maxActive  int
	callsByKey map[string]int
	delay      time.Duration
}

type longMemEvalQANonRetryableProvider struct {
	mu    sync.Mutex
	calls int
}

func (p *longMemEvalQANonRetryableProvider) Generate(_ context.Context, request provider.GenerateRequest) (provider.GenerateResponse, error) {
	p.mu.Lock()
	p.calls++
	p.mu.Unlock()
	if strings.HasPrefix(request.ContextPacket, "Governed memory:\n") {
		return provider.GenerateResponse{}, &provider.HTTPStatusError{
			StatusCode: http.StatusForbidden,
			Status:     "403 Forbidden",
			Body:       `{"code":30001,"message":"account balance is insufficient"}`,
		}
	}
	return provider.GenerateResponse{Output: "answer record-00", Model: request.Model}, nil
}

func (p *longMemEvalQARecordingProvider) Generate(_ context.Context, request provider.GenerateRequest) (provider.GenerateResponse, error) {
	condition := longMemEvalQAPlainCondition
	if strings.HasPrefix(request.ContextPacket, "Governed memory:\n") {
		condition = longMemEvalQAVermoryCondition
	}
	recordID, _, _ := strings.Cut(request.Prompt, ":")
	key := recordID + "|" + condition

	p.mu.Lock()
	if p.callsByKey == nil {
		p.callsByKey = make(map[string]int)
	}
	p.calls++
	p.active++
	p.callsByKey[key]++
	attempt := p.callsByKey[key]
	if p.active > p.maxActive {
		p.maxActive = p.active
	}
	p.mu.Unlock()

	if p.delay > 0 {
		time.Sleep(p.delay)
	}

	p.mu.Lock()
	p.active--
	p.mu.Unlock()

	if recordID == "record-00" && condition == longMemEvalQAVermoryCondition {
		return provider.GenerateResponse{}, errors.New("planned provider failure")
	}
	answer := "wrong but completed"
	if recordID != "record-01" {
		answer = "answer " + recordID
	}
	raw := []byte(`{"attempt":` + twoDigits(attempt) + `}`)
	return provider.GenerateResponse{
		Output:      answer,
		RawArtifact: raw,
		Model:       request.Model,
		Usage:       &provider.TokenUsage{InputTokens: 10, OutputTokens: 2, TotalTokens: 12},
	}, nil
}

type longMemEvalQAReaderFixture struct {
	qualification benchmark.Qualification
	execution     benchmark.ExecutionManifest
	sourcePath    string
	retrievalPath string
	artifactRoot  string
}

func (fixture longMemEvalQAReaderFixture) options(reader provider.Provider) LongMemEvalQAOptions {
	return LongMemEvalQAOptions{
		Qualification:          fixture.qualification,
		Execution:              fixture.execution,
		SourceDatasetPath:      fixture.sourcePath,
		RetrievalResultsPath:   fixture.retrievalPath,
		ArtifactRoot:           fixture.artifactRoot,
		RunID:                  fixture.execution.RunID,
		ImplementationRevision: fixture.execution.ImplementationRev,
		ReaderProvider:         reader,
	}
}

func writeLongMemEvalQAReaderFixture(t *testing.T, recordCount int) longMemEvalQAReaderFixture {
	t.Helper()
	directory := t.TempDir()
	records := make([]benchmark.LongMemEvalRecord, 0, recordCount)
	for index := 0; index < recordCount; index++ {
		id := "record-" + twoDigits(index)
		record := longMemEvalQAPlaybackRecord(id)
		record.Question = id + ": what is the answer?"
		record.Answer = "answer " + id
		records = append(records, record)
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

	execution := longMemEvalQAPlaybackExecution()
	execution.DatasetSHA256 = hex.EncodeToString(sourceDigest[:])
	execution.RecordSetSHA256 = summary.RecordSetSHA256
	execution.ExpectedSessionCount = summary.SessionCount
	execution.ExpectedTurnCount = summary.TurnCount
	execution.ExpectedScoredRecordCount = summary.ScoredRecordCount
	execution.RunID = "qa-reader-test"
	execution.ImplementationRev = strings.Repeat("a", 40)
	execution.Reader = &benchmark.ExecutionModelConfig{Provider: "test", Model: "test-reader", Interface: "test", MaxOutputTokens: 64, TimeoutSeconds: 5, Workers: 4, MaxAttempts: 3}
	execution.Judge = &benchmark.ExecutionModelConfig{Provider: "test", Model: "test-judge", Interface: "test", ScorerClass: benchmark.ScorerClassCustomModelJudge, MaxOutputTokens: 10, TimeoutSeconds: 5, Workers: 2, MaxAttempts: 2}

	results := make([]LongMemEvalRetrievalRecordResult, 0, recordCount)
	for _, record := range records {
		result := longMemEvalQAPlaybackResult(record)
		result.RunID = execution.RetrievalInput.RunID
		result.ImplementationRevision = execution.RetrievalInput.ImplementationRevision
		result.DatasetSHA256 = execution.DatasetSHA256
		result.RecordSetSHA256 = execution.RecordSetSHA256
		results = append(results, result)
	}
	retrievalPath := filepath.Join(directory, "retrieval-results.jsonl")
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
	if err := os.WriteFile(retrievalPath, retrievalData, 0o600); err != nil {
		t.Fatal(err)
	}
	retrievalDigest := sha256.Sum256(retrievalData)
	execution.RetrievalInput.SHA256 = hex.EncodeToString(retrievalDigest[:])

	qualification := benchmark.Qualification{
		SchemaVersion: "benchmark-qualification/v1",
		Benchmark:     "LongMemEval",
		SourceClass:   benchmark.SourceClassOfficialDataset,
		Repository:    benchmark.SourceReference{URL: "https://example.test/repo", Revision: strings.Repeat("b", 40)},
		License:       "MIT",
		Dataset: benchmark.DatasetSource{
			URL:         "https://example.test/dataset",
			Path:        "source.json",
			Revision:    strings.Repeat("c", 40),
			SHA256:      execution.DatasetSHA256,
			SizeBytes:   int64(len(sourceData)),
			RecordCount: recordCount,
		},
		OfficialScorer: benchmark.ScorerSource{
			URL:      "https://example.test/scorer",
			Path:     "scorer.py",
			Revision: strings.Repeat("d", 40),
			SHA256:   strings.Repeat("e", 64),
			Class:    benchmark.ScorerClassOfficialModelJudge,
		},
	}
	return longMemEvalQAReaderFixture{
		qualification: qualification,
		execution:     execution,
		sourcePath:    sourcePath,
		retrievalPath: retrievalPath,
		artifactRoot:  filepath.Join(directory, "artifacts"),
	}
}

func readLongMemEvalQACheckpointTree(t *testing.T, artifactRoot, runID string) map[string][]byte {
	t.Helper()
	root := filepath.Join(artifactRoot, "benchmarks", runID, "checkpoints")
	result := make(map[string][]byte)
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		result[relative] = data
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func equalStringByteMaps(left, right map[string][]byte) bool {
	if len(left) != len(right) {
		return false
	}
	keys := make([]string, 0, len(left))
	for key := range left {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		other, exists := right[key]
		if !exists || string(left[key]) != string(other) {
			return false
		}
	}
	return true
}
