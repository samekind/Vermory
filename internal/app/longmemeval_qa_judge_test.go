package app

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"vermory/internal/provider"
)

func TestRunLongMemEvalQAJudgeRetainsAllTerminalStatesAndResumesWithoutCalls(t *testing.T) {
	fixture := writeLongMemEvalQAReaderFixture(t, 3)
	reader := &longMemEvalQARecordingProvider{}
	readerOpts := fixture.options(reader)
	readerOpts.RetrySleeper = func(context.Context, time.Duration) error { return nil }
	if _, err := RunLongMemEvalQAReader(context.Background(), readerOpts); err != nil {
		t.Fatal(err)
	}

	judge := &longMemEvalQAJudgeRecordingProvider{}
	opts := fixture.options(nil)
	opts.JudgeProvider = judge
	opts.RetrySleeper = func(context.Context, time.Duration) error { return nil }
	summary, err := RunLongMemEvalQAJudge(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Total != 6 || summary.Judged != 1 || summary.NotRun != 1 || summary.Invalid != 2 || summary.Failed != 2 || summary.Resumed != 0 {
		t.Fatalf("unexpected judge summary: %#v", summary)
	}
	if judge.calls != 9 {
		t.Fatalf("judge calls=%d want 9", judge.calls)
	}
	if judge.maxActive > opts.Execution.Judge.Workers {
		t.Fatalf("judge concurrency reached %d, workers=%d", judge.maxActive, opts.Execution.Judge.Workers)
	}
	for _, request := range judge.requests {
		if request.ContextPacket != "" || request.System != "" {
			t.Fatalf("judge received reader context or system prompt: %#v", request)
		}
		for _, forbidden := range []string{longMemEvalQAPlainCondition, longMemEvalQAVermoryCondition, "Retrieved conversation memory:", "Governed memory:"} {
			if strings.Contains(request.Prompt, forbidden) {
				t.Fatalf("judge prompt leaked %q: %s", forbidden, request.Prompt)
			}
		}
	}

	before := readLongMemEvalQACheckpointTree(t, opts.ArtifactRoot, opts.RunID)
	resumedJudge := &longMemEvalQAJudgeRecordingProvider{}
	resumedOpts := fixture.options(nil)
	resumedOpts.JudgeProvider = resumedJudge
	resumedOpts.Resume = true
	resumedOpts.RetrySleeper = func(context.Context, time.Duration) error { return nil }
	resumed, err := RunLongMemEvalQAJudge(context.Background(), resumedOpts)
	if err != nil {
		t.Fatal(err)
	}
	if resumedJudge.calls != 0 {
		t.Fatalf("judge resume made %d provider calls", resumedJudge.calls)
	}
	if resumed.Resumed != 6 || resumed.Total != 6 || resumed.Judged != 1 || resumed.NotRun != 1 || resumed.Invalid != 2 || resumed.Failed != 2 {
		t.Fatalf("unexpected resumed judge summary: %#v", resumed)
	}
	after := readLongMemEvalQACheckpointTree(t, opts.ArtifactRoot, opts.RunID)
	if !equalStringByteMaps(before, after) {
		t.Fatal("judge resume changed checkpoint bytes")
	}
}

func TestRunLongMemEvalQAJudgeRejectsPromptDigestMismatchBeforeProviderCall(t *testing.T) {
	fixture := writeLongMemEvalQAReaderFixture(t, 1)
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

	path, err := longMemEvalQACheckpointPath(judgeOpts.ArtifactRoot, judgeOpts.RunID, "record-00", longMemEvalQAPlainCondition)
	if err != nil {
		t.Fatal(err)
	}
	checkpoint, err := loadLongMemEvalQACheckpoint(path)
	if err != nil {
		t.Fatal(err)
	}
	checkpoint.Judge.PromptSHA256 = strings.Repeat("f", 64)
	if err := writeLongMemEvalQACheckpoint(path, checkpoint); err != nil {
		t.Fatal(err)
	}

	resumedProvider := &longMemEvalQAJudgeRecordingProvider{}
	resumedOpts := fixture.options(nil)
	resumedOpts.JudgeProvider = resumedProvider
	resumedOpts.Resume = true
	if _, err := RunLongMemEvalQAJudge(context.Background(), resumedOpts); err == nil || !strings.Contains(err.Error(), "prompt_sha256") {
		t.Fatalf("expected judge prompt mismatch rejection, got %v", err)
	}
	if resumedProvider.calls != 0 {
		t.Fatalf("mismatched judge resume made %d provider calls", resumedProvider.calls)
	}
}

func TestRunLongMemEvalQAJudgeStopsAtFrozenTerminalFailureLimit(t *testing.T) {
	fixture := writeLongMemEvalQAReaderFixture(t, 3)
	readerOpts := fixture.options(&longMemEvalQARecordingProvider{})
	readerOpts.RetrySleeper = func(context.Context, time.Duration) error { return nil }
	if _, err := RunLongMemEvalQAReader(context.Background(), readerOpts); err != nil {
		t.Fatal(err)
	}

	fixture.execution.Judge.MaxTerminalFailures = 1
	judge := &longMemEvalQAJudgeRecordingProvider{}
	opts := fixture.options(nil)
	opts.JudgeProvider = judge
	opts.RetrySleeper = func(context.Context, time.Duration) error { return nil }
	summary, err := RunLongMemEvalQAJudge(context.Background(), opts)
	if err == nil || !strings.Contains(err.Error(), "judge terminal failure limit 1 reached") {
		t.Fatalf("expected terminal-failure stop, got summary=%#v err=%v", summary, err)
	}
	if summary.Failed+summary.Invalid+summary.NotRun < 1 || summary.Total >= 6 {
		t.Fatalf("judge did not stop early: %#v", summary)
	}
}

func TestRunLongMemEvalQAJudgeDoesNotRetryNonRetryableProviderError(t *testing.T) {
	fixture := writeLongMemEvalQAReaderFixture(t, 1)
	readerOpts := fixture.options(&longMemEvalQARecordingProvider{})
	readerOpts.RetrySleeper = func(context.Context, time.Duration) error { return nil }
	if _, err := RunLongMemEvalQAReader(context.Background(), readerOpts); err != nil {
		t.Fatal(err)
	}

	judge := &longMemEvalQANonRetryableJudgeProvider{}
	opts := fixture.options(nil)
	opts.JudgeProvider = judge
	opts.RetrySleeper = func(context.Context, time.Duration) error { return nil }
	summary, err := RunLongMemEvalQAJudge(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Total != 2 || summary.Failed != 1 || summary.NotRun != 1 || summary.ProviderCalls != 1 {
		t.Fatalf("unexpected judge summary: %#v", summary)
	}
	if judge.calls != 1 {
		t.Fatalf("judge provider calls=%d want 1", judge.calls)
	}
}

type longMemEvalQAJudgeRecordingProvider struct {
	mu        sync.Mutex
	calls     int
	active    int
	maxActive int
	requests  []provider.GenerateRequest
}

type longMemEvalQANonRetryableJudgeProvider struct {
	mu    sync.Mutex
	calls int
}

func (p *longMemEvalQANonRetryableJudgeProvider) Generate(_ context.Context, _ provider.GenerateRequest) (provider.GenerateResponse, error) {
	p.mu.Lock()
	p.calls++
	p.mu.Unlock()
	return provider.GenerateResponse{}, &provider.HTTPStatusError{
		StatusCode: http.StatusForbidden,
		Status:     "403 Forbidden",
		Body:       `{"code":30001,"message":"account balance is insufficient"}`,
	}
}

func (p *longMemEvalQAJudgeRecordingProvider) Generate(_ context.Context, request provider.GenerateRequest) (provider.GenerateResponse, error) {
	p.mu.Lock()
	p.calls++
	p.active++
	if p.active > p.maxActive {
		p.maxActive = p.active
	}
	p.requests = append(p.requests, request)
	p.mu.Unlock()
	time.Sleep(time.Millisecond)
	p.mu.Lock()
	p.active--
	p.mu.Unlock()

	switch {
	case strings.Contains(request.Prompt, "Question: record-01:"):
		return provider.GenerateResponse{Output: "yes because it looks correct", Model: request.Model, RawArtifact: []byte(`{"output":"invalid"}`)}, nil
	case strings.Contains(request.Prompt, "Question: record-02:"):
		return provider.GenerateResponse{}, errors.New("planned judge failure")
	default:
		return provider.GenerateResponse{Output: "yes", Model: request.Model, RawArtifact: []byte(`{"output":"yes"}`)}, nil
	}
}
