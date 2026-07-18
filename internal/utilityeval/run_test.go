package utilityeval

import (
	"context"
	"errors"
	"strings"
	"testing"

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
		RunID:        "utility-test",
		ProviderName: "test-provider",
		ProviderMode: "test",
		Model:        "test-model",
		Inputs:       []CaseInput{testInput(t)},
		Provider:     provider,
		Artifacts:    artifact.NewLocalStore(t.TempDir()),
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
}

func TestScoreTaskRequiresChineseOutputAndRejectsForbiddenFact(t *testing.T) {
	task := reality.DownstreamTask{DeterministicChecks: []string{
		"language:zh",
		"contains:Chinese",
		"not_contains:secret",
	}}
	score := scoreTask(task, "Chinese 结果")
	if !score.Success {
		t.Fatalf("expected Chinese output to pass: %#v", score)
	}
	score = scoreTask(task, "Chinese secret")
	if score.Success || score.ForbiddenHits != 1 {
		t.Fatalf("expected forbidden content to fail: %#v", score)
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
