package utilityeval

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"vermory/internal/artifact"
	"vermory/internal/provider"
	"vermory/internal/reality"
)

type recordingProvider struct {
	calls int
}

func (p *recordingProvider) Generate(_ context.Context, request provider.GenerateRequest) (provider.GenerateResponse, error) {
	p.calls++
	if strings.Contains(request.ContextPacket, string(ConditionMem0OSS)) {
		return provider.GenerateResponse{}, errors.New("mem0 provider timeout")
	}
	return provider.GenerateResponse{Output: "alpha", Model: request.Model}, nil
}

func testInput(t *testing.T) CaseInput {
	t.Helper()
	input := CaseInput{
		ID:   "case-1",
		Task: "return the current fact",
		Checks: reality.DownstreamTask{DeterministicChecks: []string{
			"contains:alpha",
			"not_contains:bad",
		}},
		Context: make(map[ConditionID]ContextEvidence, len(FrozenConditions)),
	}
	for _, condition := range FrozenConditions {
		body := ""
		if condition != ConditionNoContext {
			body = string(condition)
		}
		input.Context[condition] = NewContextEvidence(body, string(condition))
	}
	if err := input.Validate(); err != nil {
		t.Fatal(err)
	}
	return input
}

func TestContextEvidenceRejectsTampering(t *testing.T) {
	evidence := NewContextEvidence("current fact", "fixture")
	evidence.Body = "changed fact"
	if err := evidence.Validate(ConditionVermoryNative); err == nil {
		t.Fatal("expected changed context body to fail its frozen digest")
	}
}

func TestCaseInputRequiresEveryFrozenCondition(t *testing.T) {
	input := testInput(t)
	delete(input.Context, ConditionMem0OSS)
	if err := input.Validate(); err == nil || !strings.Contains(err.Error(), "mem0_oss") {
		t.Fatalf("expected missing condition error, got %v", err)
	}
}

func TestRunPreservesProviderFailureAndComputesAggregates(t *testing.T) {
	provider := &recordingProvider{}
	report, err := Run(context.Background(), RunOptions{
		RunID:         "utility-test",
		ProfileID:     "test-profile",
		ProfileSHA256: strings.Repeat("a", 64),
		ProviderName:  "test-provider",
		ProviderMode:  "test",
		Model:         "test-model",
		ScorerVersion: ScorerVersion,
		Workers:       1,
		Inputs:        []CaseInput{testInput(t)},
		Provider:      provider,
		Artifacts:     artifact.NewLocalStore(t.TempDir()),
	})
	if err != nil {
		t.Fatal(err)
	}
	if provider.calls != len(FrozenConditions) {
		t.Fatalf("expected one provider call per condition, got %d", provider.calls)
	}
	if len(report.Results) != len(FrozenConditions) {
		t.Fatalf("expected %d results, got %d", len(FrozenConditions), len(report.Results))
	}
	if report.Aggregates[ConditionMem0OSS].Failed != 1 || report.Aggregates[ConditionMem0OSS].Completed != 0 {
		t.Fatalf("expected mem0 failure to remain explicit: %#v", report.Aggregates[ConditionMem0OSS])
	}
	if report.Aggregates[ConditionNoContext].Successful != 1 || report.Aggregates[ConditionNoContext].ForbiddenHits != 0 {
		t.Fatalf("unexpected no-context aggregate: %#v", report.Aggregates[ConditionNoContext])
	}
	if report.Aggregates[ConditionVermoryNative].ContextBytes == 0 {
		t.Fatalf("expected frozen native context bytes to be counted, got %#v", report.Aggregates[ConditionVermoryNative])
	}
	if err := report.Validate(); err != nil {
		t.Fatalf("generated report did not validate: %v", err)
	}
	report.Aggregates[ConditionVermoryNative] = Aggregate{Calls: 99}
	if err := report.Validate(); err == nil {
		t.Fatal("expected changed report aggregate to be rejected")
	}
}

type concurrencyProvider struct {
	active atomic.Int32
	max    atomic.Int32
}

func (p *concurrencyProvider) Generate(_ context.Context, request provider.GenerateRequest) (provider.GenerateResponse, error) {
	active := p.active.Add(1)
	defer p.active.Add(-1)
	for {
		maximum := p.max.Load()
		if active <= maximum || p.max.CompareAndSwap(maximum, active) {
			break
		}
	}
	time.Sleep(20 * time.Millisecond)
	return provider.GenerateResponse{Output: "alpha", Model: request.Model}, nil
}

func TestRunUsesBoundedWorkersAndPreservesFrozenResultOrder(t *testing.T) {
	provider := &concurrencyProvider{}
	report, err := Run(context.Background(), RunOptions{
		RunID:         "utility-concurrency-test",
		ProfileID:     "test-profile",
		ProfileSHA256: strings.Repeat("b", 64),
		ProviderName:  "test-provider",
		ProviderMode:  "test",
		Model:         "test-model",
		ScorerVersion: ScorerVersion,
		Workers:       3,
		Inputs:        []CaseInput{testInput(t)},
		Provider:      provider,
		Artifacts:     artifact.NewLocalStore(t.TempDir()),
	})
	if err != nil {
		t.Fatal(err)
	}
	if provider.max.Load() != 3 {
		t.Fatalf("maximum concurrency=%d, want 3", provider.max.Load())
	}
	for index, condition := range FrozenConditions {
		if report.Results[index].Condition != condition {
			t.Fatalf("result %d condition=%q, want %q", index, report.Results[index].Condition, condition)
		}
	}
}

func TestScoreTaskRequiresChineseOutputAndRejectsForbiddenFact(t *testing.T) {
	task := reality.DownstreamTask{DeterministicChecks: []string{
		"language:zh",
		"contains:Chinese",
		"not_contains:secret",
	}}
	score := scoreTask(task, nil, "Chinese 结果")
	if !score.Success {
		t.Fatalf("expected Chinese output to pass: %#v", score)
	}
	score = scoreTask(task, nil, "Chinese secret")
	if score.Success || score.ForbiddenHits != 1 {
		t.Fatalf("expected forbidden content to fail: %#v", score)
	}
}

func TestScoreTaskAcceptsOnlyDeclaredEquivalentPhrases(t *testing.T) {
	task := reality.DownstreamTask{DeterministicChecks: []string{
		"contains:82 percent",
		"contains:local-scope",
		"contains:Chinese",
	}}
	aliases := map[string][]string{
		"82 percent":  {"82%"},
		"local-scope": {"任务局部", "局部覆盖"},
		"Chinese":     {"中文"},
	}
	score := scoreTask(task, aliases, "当前使用率为 82%。英语要求只在任务局部有效，全局语言仍为中文。")
	if !score.Success {
		t.Fatalf("expected declared equivalents to pass: %#v", score)
	}
	for _, check := range score.RequiredChecks {
		if !strings.Contains(check.Reason, "declared equivalent") {
			t.Fatalf("expected auditable alias match reason: %#v", check)
		}
	}
	if score.NormalizedOutput == "" {
		t.Fatal("expected normalized output to be retained")
	}
}

func TestScoreTaskAppliesAliasesToForbiddenChecks(t *testing.T) {
	task := reality.DownstreamTask{DeterministicChecks: []string{"not_contains:global default is English"}}
	aliases := map[string][]string{"global default is English": {"全局默认是英语"}}
	score := scoreTask(task, aliases, "全局默认是英语")
	if score.Success || score.ForbiddenHits != 1 {
		t.Fatalf("expected declared forbidden equivalent to fail: %#v", score)
	}
}

func TestScoreTaskAcceptsDeclaredLifecycleVerbForms(t *testing.T) {
	task := reality.DownstreamTask{DeterministicChecks: []string{
		"contains:already been deleted",
		"contains:rotated after use",
	}}
	aliases := map[string][]string{
		"already been deleted": {"has been deleted", "已删除"},
		"rotated after use":    {"rotate recovery codes after each use", "每次使用后轮换"},
	}
	score := scoreTask(task, aliases, "The bundle has been deleted. Rotate recovery codes after each use.")
	if !score.Success {
		t.Fatalf("expected declared lifecycle verb forms to pass: %#v", score)
	}
}

func TestScoreTaskAcceptsDeclaredRegexEquivalentWithoutIgnoringNegation(t *testing.T) {
	task := reality.DownstreamTask{DeterministicChecks: []string{"contains:already been deleted"}}
	aliases := map[string][]string{
		"already been deleted": {`regex:\bgame a resource bundle\b.{0,80}\b(?:has been|was|is)\s+(?:(?:successfully|already)\s+)*(?:deleted|removed)\b`},
	}
	positive := scoreTask(task, aliases, "The Game A resource bundle has been successfully deleted.")
	if !positive.Success {
		t.Fatalf("expected declared regex equivalent to pass: %#v", positive)
	}
	negative := scoreTask(task, aliases, "The Game A resource bundle has not been deleted.")
	if negative.Success {
		t.Fatalf("negated lifecycle statement matched positive regex: %#v", negative)
	}
}

func TestBuildComparableContextsKeepsBackendBodiesIndependent(t *testing.T) {
	caseFixture := reality.Case{
		Manifest: reality.Manifest{
			ID:   "case-1",
			Task: reality.DownstreamTask{Prompt: "current alpha"},
		},
		Events: []reality.Event{
			{ID: "old", Actor: "user", Channel: "chat", Content: "old alpha"},
			{ID: "new", Actor: "assistant", Channel: "chat", Content: "current alpha"},
		},
	}
	contexts, err := BuildComparableContexts(caseFixture, "native alpha", "mem0 alpha")
	if err != nil {
		t.Fatal(err)
	}
	if contexts[ConditionVermoryNative] != "native alpha" || contexts[ConditionMem0OSS] != "mem0 alpha" {
		t.Fatalf("backend context bodies were changed: %#v", contexts)
	}
	if !strings.Contains(contexts[ConditionFullHistory], "old alpha") || !strings.Contains(contexts[ConditionFullHistory], "current alpha") {
		t.Fatalf("full history omitted an event: %q", contexts[ConditionFullHistory])
	}
	if !strings.Contains(contexts[ConditionPlainRetrieval], "current alpha") {
		t.Fatalf("plain retrieval missed the matching event: %q", contexts[ConditionPlainRetrieval])
	}
}
