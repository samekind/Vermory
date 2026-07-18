package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"vermory/internal/artifact"
	"vermory/internal/bridge"
	"vermory/internal/casebook"
	"vermory/internal/domain"
	"vermory/internal/eval"
	"vermory/internal/packet"
	"vermory/internal/resolver"
	"vermory/internal/runner"
)

const (
	casebookLineWorkspace    = "workspace"
	casebookLineConversation = "conversation"
	casebookLineBridge       = "bridge"
)

type EvalCasebookOptions struct {
	CaseDir      string
	Line         string
	ArtifactRoot string
	Provider     string
	BaseURL      string
	APIKeyEnv    string
	Model        string
	RunID        string
	MaxTokens    int
}

type EvalCasebookReport struct {
	RunID        string                         `json:"run_id"`
	CaseID       string                         `json:"case_id"`
	CaseDir      string                         `json:"case_dir"`
	Line         string                         `json:"line"`
	TaskID       string                         `json:"task_id"`
	TaskSlice    string                         `json:"task_slice"`
	ProviderMode string                         `json:"provider_mode"`
	ProviderName string                         `json:"provider_name"`
	Model        string                         `json:"model"`
	Continuity   EvalCasebookContinuityReport   `json:"continuity"`
	Evaluation   runner.EvaluationReport        `json:"evaluation,omitempty"`
	Conversation EvalCasebookConversationReport `json:"conversation,omitempty"`
	Bridge       *EvalCasebookBridgeReport      `json:"bridge,omitempty"`
	Artifacts    EvalCasebookArtifactReport     `json:"artifacts"`
}

type EvalCasebookContinuityReport struct {
	Status         string `json:"status,omitempty"`
	Line           string `json:"line,omitempty"`
	SpaceID        string `json:"space_id,omitempty"`
	Name           string `json:"name,omitempty"`
	Anchor         string `json:"anchor,omitempty"`
	AnchorStrength string `json:"anchor_strength,omitempty"`
}

type EvalCasebookConversationReport struct {
	ThreadID   string                              `json:"thread_id"`
	Evaluation runner.ConversationEvaluationResult `json:"evaluation,omitempty"`
}

type EvalCasebookBridgeReport struct {
	Action          string                    `json:"action,omitempty"`
	ContinuityID    string                    `json:"continuity_id,omitempty"`
	SourceID        string                    `json:"source_id,omitempty"`
	TargetID        string                    `json:"target_id,omitempty"`
	TargetProfileID string                    `json:"target_profile_id,omitempty"`
	Title           string                    `json:"title,omitempty"`
	OldAnchor       string                    `json:"old_anchor,omitempty"`
	NewAnchor       string                    `json:"new_anchor,omitempty"`
	ClaimCount      int                       `json:"claim_count"`
	Summary         string                    `json:"summary,omitempty"`
	Audit           []EvalCasebookAuditRecord `json:"audit,omitempty"`
}

type EvalCasebookAuditRecord struct {
	Action   string `json:"action"`
	SourceID string `json:"source_id"`
	TargetID string `json:"target_id"`
}

type EvalCasebookArtifactReport struct {
	JSONURI string `json:"json_uri,omitempty"`
	MDURI   string `json:"md_uri,omitempty"`
}

type AcceptanceReportOptions struct {
	ArtifactRoot string
	RunID        string
	CasebookRun  EvalCasebookReport
}

type AcceptanceLineStatus struct {
	Status  string                `json:"status"`
	Pass    bool                  `json:"pass"`
	Failed  []string              `json:"failed,omitempty"`
	Score   *eval.AcceptanceScore `json:"score,omitempty"`
	Message string                `json:"message,omitempty"`
}

type AcceptanceReportArtifact struct {
	RunID        string                     `json:"run_id"`
	SourceRunID  string                     `json:"source_run_id"`
	CaseID       string                     `json:"case_id"`
	Line         string                     `json:"line"`
	Workspace    *AcceptanceLineStatus      `json:"workspace,omitempty"`
	Conversation *AcceptanceLineStatus      `json:"conversation,omitempty"`
	Bridge       *AcceptanceLineStatus      `json:"bridge,omitempty"`
	Artifacts    EvalCasebookArtifactReport `json:"artifacts"`
}

func EvalCasebook(ctx context.Context, opts EvalCasebookOptions) (EvalCasebookReport, error) {
	line := strings.TrimSpace(opts.Line)
	if line == "" {
		return EvalCasebookReport{}, errors.New("eval-casebook requires line")
	}
	if line != casebookLineWorkspace && line != casebookLineConversation && line != casebookLineBridge {
		return EvalCasebookReport{}, fmt.Errorf("eval-casebook line must be one of %s, %s, %s", casebookLineWorkspace, casebookLineConversation, casebookLineBridge)
	}
	caseDir := strings.TrimSpace(opts.CaseDir)
	if caseDir == "" {
		return EvalCasebookReport{}, errors.New("eval-casebook requires case-dir")
	}
	if strings.TrimSpace(opts.ArtifactRoot) == "" {
		opts.ArtifactRoot = "./artifacts"
	}
	if opts.MaxTokens <= 0 {
		opts.MaxTokens = 1024
	}

	runID := chooseRunID(opts.RunID, "casebook")
	loadedCase, err := casebook.LoadCase(caseDir)
	if err != nil {
		return EvalCasebookReport{}, err
	}
	if len(loadedCase.Tasks) == 0 {
		return EvalCasebookReport{}, errors.New("casebook case has no tasks")
	}

	task := toEvalTask(loadedCase.Tasks[0])
	claims := toCasebookDomainClaims(loadedCase.Claims)
	packetClaims := preparePacketClaims(claims)
	contextPacket := packet.Build(packet.ProfileCodingAgent, "ContextMesh", "Casebook evaluation", packetClaims)
	store := artifact.NewLocalStore(opts.ArtifactRoot)
	llm, providerMode, providerName, model, err := buildProvider(EvalSelfCaseOptions{
		ArtifactRoot: opts.ArtifactRoot,
		Provider:     opts.Provider,
		BaseURL:      opts.BaseURL,
		APIKeyEnv:    opts.APIKeyEnv,
		Model:        opts.Model,
		RunID:        runID,
		MaxTokens:    opts.MaxTokens,
	})
	if err != nil {
		return EvalCasebookReport{}, err
	}

	report := EvalCasebookReport{
		RunID:        runID,
		CaseID:       loadedCase.ID,
		CaseDir:      loadedCase.Directory,
		Line:         line,
		TaskID:       task.ID,
		TaskSlice:    "first_task_only",
		ProviderMode: providerMode,
		ProviderName: providerName,
		Model:        model,
		Continuity:   casebookContinuityReport(resolveCasebookContinuity(line, loadedCase, task, caseDir)),
	}

	runnerRunID := strings.Join([]string{runID, line}, "/")
	switch line {
	case casebookLineConversation:
		threadID := task.ID
		conversationResult, err := runner.RunConversationEvaluation(ctx, llm, runner.ConversationEvaluationOptions{
			RunID:          runnerRunID,
			ProviderMode:   providerMode,
			ProviderName:   providerName,
			Model:          model,
			SystemFrame:    "你是一个真实模型评测对象。请只根据当前对话任务和连续性视图作答，不要编造未给出的项目事实。",
			ThreadID:       threadID,
			ThreadHistory:  conversationHistory(loadedCase),
			ContinuityView: conversationContinuityView(loadedCase, claims),
			CurrentTurn:    task.Prompt,
			MaxTokens:      opts.MaxTokens,
			ArtifactPrefix: "casebook-runs",
			ArtifactStore:  store,
			Task:           task,
		})
		if err != nil {
			return EvalCasebookReport{}, err
		}
		report.Conversation = EvalCasebookConversationReport{
			ThreadID:   threadID,
			Evaluation: conversationResult,
		}
	default:
		if line == casebookLineBridge {
			bridgeReport := casebookBridgeReport(runCasebookBridgeAction(loadedCase, task, claims))
			report.Bridge = &bridgeReport
		}
		evaluationReport, err := runner.RunEvaluation(ctx, llm, runner.EvaluationOptions{
			RunID:          runnerRunID,
			ProviderMode:   providerMode,
			ProviderName:   providerName,
			Model:          model,
			Task:           task,
			StaleContext:   defaultStaleContext(),
			PlainSummary:   casebookPlainSummary(claims, loadedCase.SourceMD),
			ContextPacket:  contextPacket.Body,
			MaxTokens:      opts.MaxTokens,
			SystemPrompt:   "你是一个真实模型评测对象。请只根据用户任务和提供的上下文作答，不要编造未给出的项目事实。",
			ArtifactPrefix: "casebook-runs",
			ArtifactStore:  store,
		})
		if err != nil {
			return EvalCasebookReport{}, err
		}
		if err := preserveArtifactFromURI(ctx, store, evaluationReport.ReportURI, strings.Join([]string{"casebook-runs", runID, line, "platform-report.md"}, "/")); err != nil {
			return EvalCasebookReport{}, err
		}
		report.Evaluation = evaluationReport
	}

	jsonKey := strings.Join([]string{"casebook-runs", runID, line, "report.json"}, "/")
	mdKey := strings.Join([]string{"casebook-runs", runID, line, "report.md"}, "/")
	jsonURI, err := localArtifactURI(opts.ArtifactRoot, jsonKey)
	if err != nil {
		return EvalCasebookReport{}, err
	}
	mdURI, err := localArtifactURI(opts.ArtifactRoot, mdKey)
	if err != nil {
		return EvalCasebookReport{}, err
	}
	report.Artifacts = EvalCasebookArtifactReport{
		JSONURI: jsonURI,
		MDURI:   mdURI,
	}

	jsonBytes, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return EvalCasebookReport{}, err
	}
	if _, err := store.Put(ctx, jsonKey, jsonBytes); err != nil {
		return EvalCasebookReport{}, err
	}
	if _, err := store.Put(ctx, mdKey, []byte(markdownCasebookReport(report))); err != nil {
		return EvalCasebookReport{}, err
	}

	return report, nil
}

func AcceptanceReport(ctx context.Context, opts AcceptanceReportOptions) (AcceptanceReportArtifact, error) {
	if strings.TrimSpace(opts.ArtifactRoot) == "" {
		opts.ArtifactRoot = "./artifacts"
	}
	if strings.TrimSpace(opts.CasebookRun.RunID) == "" {
		return AcceptanceReportArtifact{}, errors.New("acceptance-report requires a casebook run")
	}
	runID := chooseRunID(opts.RunID, "acceptance")
	store := artifact.NewLocalStore(opts.ArtifactRoot)

	report := AcceptanceReportArtifact{
		RunID:       runID,
		SourceRunID: opts.CasebookRun.RunID,
		CaseID:      opts.CasebookRun.CaseID,
		Line:        opts.CasebookRun.Line,
	}

	switch opts.CasebookRun.Line {
	case casebookLineWorkspace:
		score := scoreToAcceptance(opts.CasebookRun.Evaluation.Results)
		result := eval.WorkspaceInternalReady(score)
		report.Workspace = &AcceptanceLineStatus{
			Status: "ok",
			Pass:   result.Pass,
			Failed: result.Failed,
			Score:  &score,
		}
	case casebookLineConversation:
		score := scoreToAcceptanceScore(opts.CasebookRun.Conversation.Evaluation.Score)
		result := eval.ConversationInternalReady(score)
		report.Conversation = &AcceptanceLineStatus{
			Status: "ok",
			Pass:   result.Pass,
			Failed: result.Failed,
			Score:  &score,
		}
	case casebookLineBridge:
		score := scoreToAcceptance(opts.CasebookRun.Evaluation.Results)
		result := eval.BridgeInternalReady(score)
		report.Bridge = &AcceptanceLineStatus{
			Status: "ok",
			Pass:   result.Pass,
			Failed: result.Failed,
			Score:  &score,
		}
	default:
		return AcceptanceReportArtifact{}, fmt.Errorf("unsupported casebook line %q", opts.CasebookRun.Line)
	}

	jsonKey := strings.Join([]string{"acceptance-reports", runID, "report.json"}, "/")
	mdKey := strings.Join([]string{"acceptance-reports", runID, "report.md"}, "/")
	jsonURI, err := localArtifactURI(opts.ArtifactRoot, jsonKey)
	if err != nil {
		return AcceptanceReportArtifact{}, err
	}
	mdURI, err := localArtifactURI(opts.ArtifactRoot, mdKey)
	if err != nil {
		return AcceptanceReportArtifact{}, err
	}
	report.Artifacts = EvalCasebookArtifactReport{
		JSONURI: jsonURI,
		MDURI:   mdURI,
	}

	jsonBytes, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return AcceptanceReportArtifact{}, err
	}
	if _, err := store.Put(ctx, jsonKey, jsonBytes); err != nil {
		return AcceptanceReportArtifact{}, err
	}
	if _, err := store.Put(ctx, mdKey, []byte(markdownAcceptanceReport(report))); err != nil {
		return AcceptanceReportArtifact{}, err
	}

	return report, nil
}

func toEvalTask(task casebook.Task) eval.Task {
	return eval.Task{
		ID:             task.ID,
		Prompt:         task.Prompt,
		MustInclude:    append([]string(nil), task.MustInclude...),
		MustIncludeAny: append([][]string(nil), task.MustIncludeAny...),
		MustNotInclude: append([]string(nil), task.MustNotInclude...),
	}
}

func toCasebookDomainClaims(claims []casebook.Claim) []domain.Claim {
	out := make([]domain.Claim, 0, len(claims))
	for _, claim := range claims {
		out = append(out, domain.Claim{
			Type:           claim.Type,
			Content:        claim.Content,
			Status:         domain.ClaimStatusConfirmed,
			VerifiedByUser: true,
		})
	}
	return out
}

func casebookPlainSummary(claims []domain.Claim, source string) string {
	return plainSummary(claims)
}

func conversationHistory(loadedCase casebook.Case) []string {
	lines := []string{
		"User: Please keep the answer grounded in the provided continuity facts.",
		"Assistant: I will only rely on the provided continuity view.",
	}
	if snippet := firstNonEmptyLine(loadedCase.SourceMD); snippet != "" {
		lines = append(lines, "User: Source anchor: "+snippet)
	}
	return lines
}

func conversationContinuityView(loadedCase casebook.Case, claims []domain.Claim) string {
	parts := []string{
		"Case ID: " + loadedCase.ID,
		"Task slice: first task only for the current minimum vertical slice.",
	}
	for _, claim := range claims {
		parts = append(parts, "- "+claim.Content)
	}
	return strings.Join(parts, "\n")
}

func resolveCasebookContinuity(line string, loadedCase casebook.Case, task eval.Task, caseDir string) resolver.Resolution {
	if line == casebookLineConversation {
		return resolver.ResolveConversation(resolver.ConversationInput{
			ThreadID: task.ID,
			Channel:  "casebook",
		})
	}

	return resolver.ResolveWorkspace(resolver.WorkspaceInput{
		CWD:                caseDir,
		ExplicitBindingID:  "workspace:" + loadedCase.ID,
		CandidateRepoRoot:  caseDir,
		KnownWorkspaceName: loadedCase.ID,
		Candidates: []resolver.WorkspaceCandidate{
			{ID: "workspace:" + loadedCase.ID, Path: caseDir},
		},
	})
}

func casebookContinuityReport(resolution resolver.Resolution) EvalCasebookContinuityReport {
	return EvalCasebookContinuityReport{
		Status:         string(resolution.Status),
		Line:           string(resolution.Line),
		SpaceID:        resolution.SpaceID,
		Name:           resolution.Name,
		Anchor:         resolution.Anchor,
		AnchorStrength: string(resolution.AnchorStrength),
	}
}

func runCasebookBridgeAction(loadedCase casebook.Case, task eval.Task, claims []domain.Claim) bridge.BridgeResult {
	workspaceID := "workspace:" + loadedCase.ID
	conversationID := "conversation:casebook:" + loadedCase.ID
	switch inferBridgeAction(loadedCase, task) {
	case domain.BridgeActionLink:
		return bridge.Link(bridge.LinkRequest{
			PrimaryConversationID: conversationID,
			LinkedConversationIDs: []string{
				"conversation:openclaw:" + loadedCase.ID,
				"conversation:phone:" + loadedCase.ID,
			},
			Claims: claims,
		})
	case domain.BridgeActionExport:
		return bridge.Export(bridge.ExportRequest{
			WorkspaceID:      workspaceID,
			TargetProfileID:  inferTargetProfileID(loadedCase, task),
			ExportTitle:      loadedCase.ID + " export",
			ContinuityClaims: claims,
		})
	case domain.BridgeActionAdopt:
		return bridge.Adopt(bridge.AdoptRequest{
			ContinuityID:    workspaceID,
			CandidateAnchor: "candidate:" + loadedCase.ID,
			ConfirmedAnchor: loadedCase.Directory,
		})
	case domain.BridgeActionRebind:
		return bridge.Rebind(bridge.RebindRequest{
			ContinuityID: workspaceID,
			OldAnchor:    "historical:" + loadedCase.ID,
			NewAnchor:    loadedCase.Directory,
		})
	default:
		return bridge.Promote(bridge.PromoteRequest{
			ConversationID: conversationID,
			WorkspaceID:    workspaceID,
			Claims:         claims,
		})
	}
}

func inferBridgeAction(loadedCase casebook.Case, task eval.Task) domain.BridgeAction {
	text := strings.ToLower(strings.Join([]string{loadedCase.ID, task.ID, task.Prompt}, " "))
	switch {
	case strings.Contains(text, "link"):
		return domain.BridgeActionLink
	case strings.Contains(text, "rebind"):
		return domain.BridgeActionRebind
	case strings.Contains(text, "adopt"):
		return domain.BridgeActionAdopt
	case strings.Contains(text, "export") || strings.Contains(text, "report") || strings.Contains(text, "handoff"):
		return domain.BridgeActionExport
	default:
		return domain.BridgeActionPromote
	}
}

func inferTargetProfileID(loadedCase casebook.Case, task eval.Task) domain.TargetProfileID {
	text := strings.ToLower(strings.Join([]string{loadedCase.ID, task.ID, task.Prompt}, " "))
	switch {
	case strings.Contains(text, "handoff"):
		return "team_handoff"
	case strings.Contains(text, "review") || strings.Contains(text, "report"):
		return "review_committee"
	default:
		return "general_chat"
	}
}

func casebookBridgeReport(result bridge.BridgeResult) EvalCasebookBridgeReport {
	audit := make([]EvalCasebookAuditRecord, 0, len(result.Audit))
	for _, record := range result.Audit {
		audit = append(audit, EvalCasebookAuditRecord{
			Action:   string(record.Action),
			SourceID: record.SourceID,
			TargetID: record.TargetID,
		})
	}
	return EvalCasebookBridgeReport{
		Action:          string(result.Action),
		ContinuityID:    result.ContinuityID,
		SourceID:        result.SourceID,
		TargetID:        result.TargetID,
		TargetProfileID: string(result.TargetProfileID),
		Title:           result.Title,
		OldAnchor:       result.OldAnchor,
		NewAnchor:       result.NewAnchor,
		ClaimCount:      len(result.Claims),
		Summary:         result.Summary,
		Audit:           audit,
	}
}

func firstNonEmptyLine(body string) string {
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			return line
		}
	}
	return ""
}

func scoreToAcceptance(results []runner.BaselineResult) eval.AcceptanceScore {
	for _, result := range results {
		if result.Baseline == runner.BaselineContextMeshPacket {
			return scoreToAcceptanceScore(result.Score)
		}
	}
	return eval.AcceptanceScore{}
}

func scoreToAcceptanceScore(score eval.Score) eval.AcceptanceScore {
	return eval.AcceptanceScore{
		Continuation:  score.Continuation,
		Isolation:     score.Groundedness,
		Groundedness:  score.Groundedness,
		Governance:    0,
		TargetFitness: score.TargetFitness,
		CostFriction:  0,
	}
}

func markdownCasebookReport(report EvalCasebookReport) string {
	var b strings.Builder
	b.WriteString("# ContextMesh Casebook Evaluation Report\n\n")
	b.WriteString(fmt.Sprintf("- Run ID: `%s`\n", report.RunID))
	b.WriteString(fmt.Sprintf("- Case ID: `%s`\n", report.CaseID))
	b.WriteString(fmt.Sprintf("- Line: `%s`\n", report.Line))
	b.WriteString(fmt.Sprintf("- Task ID: `%s`\n", report.TaskID))
	b.WriteString("- Task slice: `first_task_only` (minimum slice for current implementation)\n")
	b.WriteString(fmt.Sprintf("- Provider mode: `%s`\n", report.ProviderMode))
	b.WriteString(fmt.Sprintf("- Provider: `%s`\n", report.ProviderName))
	b.WriteString(fmt.Sprintf("- Model: `%s`\n\n", report.Model))
	b.WriteString(fmt.Sprintf("- Continuity: `%s` `%s` `%s`\n\n", report.Continuity.Line, report.Continuity.Status, report.Continuity.SpaceID))

	switch report.Line {
	case casebookLineConversation:
		b.WriteString("This run evaluates the case through the chat contract conversation runner.\n\n")
		b.WriteString(fmt.Sprintf("- Thread ID: `%s`\n", report.Conversation.ThreadID))
		if report.Conversation.Evaluation.OutputURI != "" {
			b.WriteString(fmt.Sprintf("- Output artifact: `%s`\n", report.Conversation.Evaluation.OutputURI))
		}
	case casebookLineBridge:
		b.WriteString("This run evaluates the case through the four-baseline runner and records a governed bridge action.\n\n")
		if report.Bridge != nil {
			b.WriteString(fmt.Sprintf("- Bridge action: `%s`\n", report.Bridge.Action))
			b.WriteString(fmt.Sprintf("- Bridge continuity: `%s`\n", report.Bridge.ContinuityID))
			b.WriteString(fmt.Sprintf("- Bridge target: `%s`\n", report.Bridge.TargetID))
			b.WriteString(fmt.Sprintf("- Bridge audit records: `%d`\n", len(report.Bridge.Audit)))
		}
		if report.Evaluation.ReportURI != "" {
			b.WriteString(fmt.Sprintf("- Platform report: `%s`\n", report.Evaluation.ReportURI))
		}
	default:
		b.WriteString("This run evaluates the case through the four-baseline workspace/bridge runner.\n\n")
		if report.Evaluation.ReportURI != "" {
			b.WriteString(fmt.Sprintf("- Platform report: `%s`\n", report.Evaluation.ReportURI))
		}
	}
	return b.String()
}

func markdownAcceptanceReport(report AcceptanceReportArtifact) string {
	var b strings.Builder
	b.WriteString("# ContextMesh Acceptance Report\n\n")
	b.WriteString(fmt.Sprintf("- Run ID: `%s`\n", report.RunID))
	b.WriteString(fmt.Sprintf("- Source casebook run: `%s`\n", report.SourceRunID))
	b.WriteString(fmt.Sprintf("- Case ID: `%s`\n", report.CaseID))
	b.WriteString(fmt.Sprintf("- Line: `%s`\n\n", report.Line))

	if report.Workspace != nil {
		b.WriteString("## Workspace\n\n")
		writeAcceptanceLineMarkdown(&b, report.Workspace)
	}
	if report.Conversation != nil {
		b.WriteString("## Conversation\n\n")
		writeAcceptanceLineMarkdown(&b, report.Conversation)
	}
	if report.Bridge != nil {
		b.WriteString("## Bridge\n\n")
		writeAcceptanceLineMarkdown(&b, report.Bridge)
	}
	return b.String()
}

func writeAcceptanceLineMarkdown(b *strings.Builder, status *AcceptanceLineStatus) {
	b.WriteString(fmt.Sprintf("- Status: `%s`\n", status.Status))
	b.WriteString(fmt.Sprintf("- Pass: `%t`\n", status.Pass))
	if len(status.Failed) > 0 {
		b.WriteString(fmt.Sprintf("- Failed gates: `%s`\n", strings.Join(status.Failed, ", ")))
	}
	if status.Message != "" {
		b.WriteString(fmt.Sprintf("- Note: %s\n", status.Message))
	}
}

func LoadCasebookReport(path string) (EvalCasebookReport, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return EvalCasebookReport{}, err
	}
	var report EvalCasebookReport
	if err := json.Unmarshal(data, &report); err != nil {
		return EvalCasebookReport{}, err
	}
	return report, nil
}

func preserveArtifactFromURI(ctx context.Context, store artifact.Store, uri string, key string) error {
	path, err := fileURIPath(uri)
	if err != nil {
		return err
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	_, err = store.Put(ctx, key, content)
	return err
}

func fileURIPath(uri string) (string, error) {
	parsed, err := url.Parse(uri)
	if err != nil {
		return "", err
	}
	if parsed.Scheme != "file" {
		return "", fmt.Errorf("unsupported artifact URI %q", uri)
	}
	return filepath.FromSlash(parsed.Path), nil
}

func localArtifactURI(root string, key string) (string, error) {
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	path := filepath.Join(absoluteRoot, filepath.Clean(key))
	return (&url.URL{Scheme: "file", Path: filepath.ToSlash(path)}).String(), nil
}
