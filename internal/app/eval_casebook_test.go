package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"vermory/internal/casebook"
	"vermory/internal/domain"
	"vermory/internal/eval"
	"vermory/internal/runner"
)

func TestEvalCasebookRequiresLineAndCaseDir(t *testing.T) {
	t.Run("missing line", func(t *testing.T) {
		_, err := EvalCasebook(context.Background(), EvalCasebookOptions{
			CaseDir:      t.TempDir(),
			ArtifactRoot: t.TempDir(),
			Provider:     "mock",
			RunID:        "missing-line",
		})
		if err == nil {
			t.Fatal("expected missing line error")
		}
		if !strings.Contains(err.Error(), "line") {
			t.Fatalf("expected line validation error, got %v", err)
		}
	})

	t.Run("missing case-dir", func(t *testing.T) {
		_, err := EvalCasebook(context.Background(), EvalCasebookOptions{
			Line:         "workspace",
			ArtifactRoot: t.TempDir(),
			Provider:     "mock",
			RunID:        "missing-case-dir",
		})
		if err == nil {
			t.Fatal("expected missing case-dir error")
		}
		if !strings.Contains(err.Error(), "case-dir") {
			t.Fatalf("expected case-dir validation error, got %v", err)
		}
	})
}

func TestConversationHistoryDoesNotInjectWorkspaceFraming(t *testing.T) {
	history := conversationHistory(casebook.Case{SourceMD: "# Apartment search"})
	joined := strings.Join(history, "\n")
	if strings.Contains(strings.ToLower(joined), "repository") || strings.Contains(strings.ToLower(joined), "workspace") {
		t.Fatalf("conversation history must not inject workspace framing: %q", joined)
	}
	if !strings.Contains(joined, "provided continuity facts") {
		t.Fatalf("expected neutral continuity framing, got %q", joined)
	}
}

func TestEvalCasebookWorkspaceMockWritesArtifacts(t *testing.T) {
	caseDir := writeCasebookFixture(t, "workspace-case", []fixtureSpecTask{{
		ID:             "workspace-task",
		Prompt:         "Summarize the workspace continuity for ContextMesh.",
		MustInclude:    []string{"ContextMesh", "workspace"},
		MustNotInclude: []string{"jstarctl"},
	}})
	artifactRoot := t.TempDir()

	report, err := EvalCasebook(context.Background(), EvalCasebookOptions{
		CaseDir:      caseDir,
		Line:         "workspace",
		ArtifactRoot: artifactRoot,
		Provider:     "mock",
		Model:        "mock-model",
		RunID:        "workspace-run",
	})
	if err != nil {
		t.Fatalf("EvalCasebook returned error: %v", err)
	}

	if report.Line != "workspace" {
		t.Fatalf("expected workspace line, got %q", report.Line)
	}
	if report.Evaluation.ReportURI == "" {
		t.Fatalf("expected workspace evaluation report URI, got %#v", report)
	}

	expectedFiles := []string{
		"casebook-runs/workspace-run/workspace/report.json",
		"casebook-runs/workspace-run/workspace/report.md",
		"casebook-runs/workspace-run/workspace/platform-report.md",
		"casebook-runs/workspace-run/workspace/contextmesh_packet/packet.md",
	}
	for _, rel := range expectedFiles {
		if _, err := os.Stat(filepath.Join(artifactRoot, rel)); err != nil {
			t.Fatalf("expected artifact %s: %v", rel, err)
		}
	}

	var persisted EvalCasebookReport
	data, err := os.ReadFile(filepath.Join(artifactRoot, "casebook-runs", "workspace-run", "workspace", "report.json"))
	if err != nil {
		t.Fatalf("read report json: %v", err)
	}
	if err := json.Unmarshal(data, &persisted); err != nil {
		t.Fatalf("unmarshal report json: %v", err)
	}
	if persisted.Artifacts.JSONURI == "" || persisted.Artifacts.MDURI == "" {
		t.Fatalf("expected persisted report to include artifact URIs, got %#v", persisted.Artifacts)
	}
	if persisted.Continuity.Status != "resolved" {
		t.Fatalf("expected persisted workspace continuity resolution, got %#v", persisted.Continuity)
	}
	if persisted.Continuity.Line != "workspace" {
		t.Fatalf("expected workspace continuity line, got %#v", persisted.Continuity)
	}
}

func TestEvalCasebookConversationMockWritesConversationArtifacts(t *testing.T) {
	caseDir := writeCasebookFixture(t, "conversation-case", []fixtureSpecTask{{
		ID:             "conversation-task",
		Prompt:         "Answer the current conversation turn about ContextMesh workspace limits.",
		MustInclude:    []string{"ContextMesh", "workspace"},
		MustNotInclude: []string{"jstarctl"},
	}})
	artifactRoot := t.TempDir()

	report, err := EvalCasebook(context.Background(), EvalCasebookOptions{
		CaseDir:      caseDir,
		Line:         "conversation",
		ArtifactRoot: artifactRoot,
		Provider:     "mock",
		Model:        "mock-model",
		RunID:        "conversation-run",
	})
	if err != nil {
		t.Fatalf("EvalCasebook returned error: %v", err)
	}

	if report.Line != "conversation" {
		t.Fatalf("expected conversation line, got %q", report.Line)
	}
	if report.Conversation.ThreadID == "" {
		t.Fatalf("expected conversation thread id, got %#v", report)
	}
	if report.Continuity.Status != "resolved" {
		t.Fatalf("expected conversation continuity resolution, got %#v", report.Continuity)
	}
	if report.Continuity.SpaceID != "thread:casebook:conversation-task" {
		t.Fatalf("expected thread continuity id, got %#v", report.Continuity)
	}

	expectedFiles := []string{
		"casebook-runs/conversation-run/conversation/report.json",
		"casebook-runs/conversation-run/conversation/report.md",
		"casebook-runs/conversation-run/conversation/conversation/conversation-task/input.md",
		"casebook-runs/conversation-run/conversation/conversation/conversation-task/output.md",
		"casebook-runs/conversation-run/conversation/conversation/conversation-task/score.json",
	}
	for _, rel := range expectedFiles {
		if _, err := os.Stat(filepath.Join(artifactRoot, rel)); err != nil {
			t.Fatalf("expected artifact %s: %v", rel, err)
		}
	}
}

func TestEvalCasebookBridgeMockWritesBridgeActionReport(t *testing.T) {
	caseDir := writeCasebookFixture(t, "303-bridge-multichannel-link", []fixtureSpecTask{{
		ID:             "bridge-link-task",
		Prompt:         "Link the OpenClaw and phone chat continuity spaces for the same housing search.",
		MustInclude:    []string{"ContextMesh", "workspace"},
		MustNotInclude: []string{"jstarctl"},
	}})
	artifactRoot := t.TempDir()

	report, err := EvalCasebook(context.Background(), EvalCasebookOptions{
		CaseDir:      caseDir,
		Line:         "bridge",
		ArtifactRoot: artifactRoot,
		Provider:     "mock",
		Model:        "mock-model",
		RunID:        "bridge-link-run",
	})
	if err != nil {
		t.Fatalf("EvalCasebook returned error: %v", err)
	}

	if report.Bridge.Action != "link" {
		t.Fatalf("expected link bridge action, got %#v", report.Bridge)
	}
	if report.Bridge.ContinuityID == "" || report.Bridge.TargetID == "" {
		t.Fatalf("expected bridge continuity and target ids, got %#v", report.Bridge)
	}
	if len(report.Bridge.Audit) == 0 {
		t.Fatalf("expected bridge audit records, got %#v", report.Bridge)
	}
	if report.Continuity.Line != "workspace" {
		t.Fatalf("expected bridge run to resolve workspace continuity, got %#v", report.Continuity)
	}
}

func TestAcceptanceReportWritesMinimalArtifacts(t *testing.T) {
	caseDir := writeCasebookFixture(t, "workspace-acceptance-case", []fixtureSpecTask{{
		ID:             "workspace-acceptance",
		Prompt:         "Summarize the ContextMesh workspace continuity.",
		MustInclude:    []string{"ContextMesh", "workspace"},
		MustNotInclude: []string{"jstarctl"},
	}})
	artifactRoot := t.TempDir()

	runReport, err := EvalCasebook(context.Background(), EvalCasebookOptions{
		CaseDir:      caseDir,
		Line:         "workspace",
		ArtifactRoot: artifactRoot,
		Provider:     "mock",
		Model:        "mock-model",
		RunID:        "acceptance-source-run",
	})
	if err != nil {
		t.Fatalf("EvalCasebook returned error: %v", err)
	}

	acceptance, err := AcceptanceReport(context.Background(), AcceptanceReportOptions{
		ArtifactRoot: artifactRoot,
		RunID:        "acceptance-run",
		CasebookRun:  runReport,
	})
	if err != nil {
		t.Fatalf("AcceptanceReport returned error: %v", err)
	}

	if acceptance.Line != "workspace" {
		t.Fatalf("expected workspace line, got %q", acceptance.Line)
	}
	if acceptance.Workspace == nil {
		t.Fatalf("expected workspace acceptance payload, got %#v", acceptance)
	}
	if acceptance.Workspace.Pass != true {
		t.Fatalf("expected workspace acceptance pass field, got %#v", acceptance.Workspace)
	}

	jsonPath := filepath.Join(artifactRoot, "acceptance-reports", "acceptance-run", "report.json")
	mdPath := filepath.Join(artifactRoot, "acceptance-reports", "acceptance-run", "report.md")
	for _, path := range []string{jsonPath, mdPath} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected artifact %s: %v", path, err)
		}
	}

	var persisted AcceptanceReportArtifact
	data, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Fatalf("read report json: %v", err)
	}
	if err := json.Unmarshal(data, &persisted); err != nil {
		t.Fatalf("unmarshal report json: %v", err)
	}
	if persisted.Workspace == nil || !persisted.Workspace.Pass {
		t.Fatalf("expected persisted workspace pass/fail fields, got %#v", persisted)
	}
	if persisted.Artifacts.JSONURI == "" || persisted.Artifacts.MDURI == "" {
		t.Fatalf("expected persisted acceptance report to include artifact URIs, got %#v", persisted.Artifacts)
	}
}

func TestAcceptanceReportConversationUsesConversationScore(t *testing.T) {
	artifactRoot := t.TempDir()
	runReport := EvalCasebookReport{
		RunID:  "conversation-source-run",
		CaseID: "conversation-case",
		Line:   "conversation",
		Conversation: EvalCasebookConversationReport{
			ThreadID: "thread-1",
			Evaluation: runner.ConversationEvaluationResult{
				Score: eval.Score{
					Continuation:  0.86,
					Groundedness:  0.94,
					TargetFitness: 0.86,
				},
			},
		},
	}

	acceptance, err := AcceptanceReport(context.Background(), AcceptanceReportOptions{
		ArtifactRoot: artifactRoot,
		RunID:        "conversation-acceptance-run",
		CasebookRun:  runReport,
	})
	if err != nil {
		t.Fatalf("AcceptanceReport returned error: %v", err)
	}

	if acceptance.Conversation == nil {
		t.Fatalf("expected conversation acceptance payload, got %#v", acceptance)
	}
	if acceptance.Conversation.Status != "ok" {
		t.Fatalf("expected conversation acceptance to be implemented, got status %q", acceptance.Conversation.Status)
	}
	if acceptance.Conversation.Score == nil {
		t.Fatalf("expected conversation acceptance score, got %#v", acceptance.Conversation)
	}
	if !acceptance.Conversation.Pass {
		t.Fatalf("expected conversation acceptance to pass, got %#v", acceptance.Conversation)
	}
	if acceptance.Conversation.Score.Continuation != 0.86 {
		t.Fatalf("expected conversation continuation score from conversation runner, got %f", acceptance.Conversation.Score.Continuation)
	}
}

func TestAcceptanceReportBridgeUsesContextMeshPacketBaseline(t *testing.T) {
	artifactRoot := t.TempDir()
	runReport := EvalCasebookReport{
		RunID:  "bridge-source-run",
		CaseID: "bridge-case",
		Line:   "bridge",
		Evaluation: runner.EvaluationReport{
			Results: []runner.BaselineResult{
				{
					Baseline: runner.BaselineNoContext,
					Score: eval.Score{
						Continuation:  1.00,
						Groundedness:  1.00,
						TargetFitness: 1.00,
					},
				},
				{
					Baseline: runner.BaselineContextMeshPacket,
					Score: eval.Score{
						Continuation:  0.25,
						Groundedness:  0.50,
						TargetFitness: 0.25,
					},
				},
			},
		},
	}

	acceptance, err := AcceptanceReport(context.Background(), AcceptanceReportOptions{
		ArtifactRoot: artifactRoot,
		RunID:        "bridge-acceptance-run",
		CasebookRun:  runReport,
	})
	if err != nil {
		t.Fatalf("AcceptanceReport returned error: %v", err)
	}

	if acceptance.Bridge == nil {
		t.Fatalf("expected bridge acceptance payload, got %#v", acceptance)
	}
	if acceptance.Bridge.Status != "ok" {
		t.Fatalf("expected bridge acceptance to be implemented, got status %q", acceptance.Bridge.Status)
	}
	if acceptance.Bridge.Score == nil {
		t.Fatalf("expected bridge acceptance score, got %#v", acceptance.Bridge)
	}
	if acceptance.Bridge.Pass {
		t.Fatalf("expected bridge acceptance to fail with low contextmesh_packet score")
	}
	if acceptance.Bridge.Score.Continuation != 0.25 {
		t.Fatalf("expected bridge acceptance to use contextmesh_packet continuation, got %f", acceptance.Bridge.Score.Continuation)
	}
}

func TestScoreToAcceptanceUsesContextMeshPacketBaseline(t *testing.T) {
	score := scoreToAcceptance([]runner.BaselineResult{
		{
			Baseline: runner.BaselineNoContext,
			Score: eval.Score{
				Continuation:  1.00,
				Groundedness:  1.00,
				TargetFitness: 1.00,
			},
		},
		{
			Baseline: runner.BaselineContextMeshPacket,
			Score: eval.Score{
				Continuation:  0.25,
				Groundedness:  0.50,
				TargetFitness: 0.25,
			},
		},
	})

	if score.Continuation != 0.25 {
		t.Fatalf("expected acceptance to use contextmesh_packet continuation, got %f", score.Continuation)
	}
	if score.Groundedness != 0.50 {
		t.Fatalf("expected acceptance to use contextmesh_packet groundedness, got %f", score.Groundedness)
	}
}

func TestCasebookPlainSummaryDoesNotAppendFullSource(t *testing.T) {
	claims := []domain.Claim{
		{
			Content:        "ContextMesh keeps workspace continuity grounded in claims.",
			Status:         domain.ClaimStatusConfirmed,
			VerifiedByUser: true,
		},
	}
	source := "Full source should stay out of the plain summary baseline."

	summary := casebookPlainSummary(claims, source)

	if !strings.Contains(summary, "ContextMesh keeps workspace continuity grounded in claims.") {
		t.Fatalf("expected claim summary, got %q", summary)
	}
	if strings.Contains(summary, "Full source should stay out") || strings.Contains(summary, "Source excerpt") {
		t.Fatalf("expected plain summary to avoid full source injection, got %q", summary)
	}
}

type fixtureSpecTask struct {
	ID             string
	Prompt         string
	MustInclude    []string
	MustNotInclude []string
}

func writeCasebookFixture(t *testing.T, caseID string, tasks []fixtureSpecTask) string {
	t.Helper()

	caseDir := filepath.Join(t.TempDir(), caseID)
	if err := os.MkdirAll(caseDir, 0o755); err != nil {
		t.Fatalf("mkdir fixture dir: %v", err)
	}

	source := strings.Join([]string{
		"# ContextMesh fixture",
		"",
		"ContextMesh workspace continuity is grounded in the current repository state.",
		"Conversation continuity should route through the chat contract runner.",
	}, "\n")
	if err := os.WriteFile(filepath.Join(caseDir, "source.md"), []byte(source), 0o644); err != nil {
		t.Fatalf("write source.md: %v", err)
	}

	type claim struct {
		Type    string `json:"type"`
		Content string `json:"content"`
	}
	claims := []claim{
		{Type: "goal", Content: "ContextMesh keeps workspace continuity grounded in repository state."},
		{Type: "constraint", Content: "Conversation evaluations must use the chat contract runner."},
		{Type: "fact", Content: "Bridge and workspace lines can reuse the baseline evaluation runner."},
	}
	claimBytes, err := json.MarshalIndent(claims, "", "  ")
	if err != nil {
		t.Fatalf("marshal claims: %v", err)
	}
	if err := os.WriteFile(filepath.Join(caseDir, "claims.json"), claimBytes, 0o644); err != nil {
		t.Fatalf("write claims.json: %v", err)
	}

	taskBytes, err := json.MarshalIndent(tasks, "", "  ")
	if err != nil {
		t.Fatalf("marshal tasks: %v", err)
	}
	if err := os.WriteFile(filepath.Join(caseDir, "tasks.json"), taskBytes, 0o644); err != nil {
		t.Fatalf("write tasks.json: %v", err)
	}

	return caseDir
}
