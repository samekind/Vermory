package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"vermory/internal/artifact"
	"vermory/internal/brand"
	"vermory/internal/provider"
)

type ProbeProviderOptions struct {
	ArtifactRoot string
	Provider     string
	BaseURL      string
	APIKeyEnv    string
	RunID        string
	Prompt       string
	Models       []string
	MaxTokens    int
}

type ProbeResult struct {
	Model         string `json:"model"`
	Status        string `json:"status"`
	OutputPreview string `json:"output_preview,omitempty"`
	OutputURI     string `json:"output_uri,omitempty"`
	RawURI        string `json:"raw_uri,omitempty"`
	Error         string `json:"error,omitempty"`
}

type ProbeReport struct {
	RunID        string        `json:"run_id"`
	ProviderMode string        `json:"provider_mode"`
	ProviderName string        `json:"provider_name"`
	BaseURL      string        `json:"base_url,omitempty"`
	Prompt       string        `json:"prompt"`
	Results      []ProbeResult `json:"results"`
	ReportURI    string        `json:"report_uri,omitempty"`
}

func ProbeProvider(ctx context.Context, opts ProbeProviderOptions) (ProbeReport, error) {
	models := compactModels(opts.Models)
	if len(models) == 0 {
		return ProbeReport{}, errors.New("probe-provider requires at least one model")
	}
	if strings.TrimSpace(opts.ArtifactRoot) == "" {
		opts.ArtifactRoot = "./artifacts"
	}
	if strings.TrimSpace(opts.Prompt) == "" {
		opts.Prompt = "Only reply OK."
	}
	if opts.MaxTokens <= 0 {
		opts.MaxTokens = 64
	}

	llm, providerMode, providerName, _, err := buildProvider(EvalSelfCaseOptions{
		ArtifactRoot: opts.ArtifactRoot,
		Provider:     opts.Provider,
		BaseURL:      opts.BaseURL,
		APIKeyEnv:    opts.APIKeyEnv,
		Model:        models[0],
		MaxTokens:    opts.MaxTokens,
	})
	if err != nil {
		return ProbeReport{}, err
	}

	report := ProbeReport{
		RunID:        chooseRunID(opts.RunID, "probe"),
		ProviderMode: providerMode,
		ProviderName: providerName,
		BaseURL:      resolvedBaseURL(providerName, opts.BaseURL),
		Prompt:       opts.Prompt,
		Results:      make([]ProbeResult, 0, len(models)),
	}
	store := artifact.NewLocalStore(opts.ArtifactRoot)

	for _, model := range models {
		result := ProbeResult{Model: model}
		resp, err := llm.Generate(ctx, provider.GenerateRequest{
			Model:     model,
			System:    "Reply as directly as possible. Avoid extra commentary.",
			Prompt:    opts.Prompt,
			MaxTokens: opts.MaxTokens,
		})
		if err != nil {
			result.Status = "error"
			result.Error = err.Error()
			report.Results = append(report.Results, result)
			continue
		}
		result.Status = "ok"
		result.OutputPreview = previewText(resp.Output)
		outputArtifact, err := store.Put(ctx, probeArtifactKey(report.RunID, model, "output.md"), []byte(resp.Output))
		if err != nil {
			return ProbeReport{}, err
		}
		result.OutputURI = outputArtifact.URI
		if len(resp.RawArtifact) > 0 {
			rawArtifact, err := store.Put(ctx, probeArtifactKey(report.RunID, model, "raw.json"), resp.RawArtifact)
			if err != nil {
				return ProbeReport{}, err
			}
			result.RawURI = rawArtifact.URI
		}
		report.Results = append(report.Results, result)
	}

	reportBytes, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return ProbeReport{}, err
	}
	reportArtifact, err := store.Put(ctx, probeArtifactKey(report.RunID, "report.json"), reportBytes)
	if err != nil {
		return ProbeReport{}, err
	}
	markdownArtifact, err := store.Put(ctx, probeArtifactKey(report.RunID, "report.md"), []byte(markdownProbeReport(report)))
	if err != nil {
		return ProbeReport{}, err
	}
	report.ReportURI = markdownArtifact.URI
	_ = reportArtifact

	return report, nil
}

func compactModels(models []string) []string {
	seen := make(map[string]struct{}, len(models))
	out := make([]string, 0, len(models))
	for _, model := range models {
		model = strings.TrimSpace(model)
		if model == "" {
			continue
		}
		if _, ok := seen[model]; ok {
			continue
		}
		seen[model] = struct{}{}
		out = append(out, model)
	}
	return out
}

func previewText(text string) string {
	text = strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
	if len([]rune(text)) <= 160 {
		return text
	}
	runes := []rune(text)
	return string(runes[:160]) + "...(truncated)"
}

func probeArtifactKey(parts ...string) string {
	return strings.Join(append([]string{"provider-probes"}, parts...), "/")
}

func markdownProbeReport(report ProbeReport) string {
	var b strings.Builder
	b.WriteString("# " + brand.Name + " Provider Probe Report\n\n")
	b.WriteString(fmt.Sprintf("- Run ID: `%s`\n", report.RunID))
	b.WriteString(fmt.Sprintf("- Provider mode: `%s`\n", report.ProviderMode))
	b.WriteString(fmt.Sprintf("- Provider: `%s`\n", report.ProviderName))
	if report.BaseURL != "" {
		b.WriteString(fmt.Sprintf("- Base URL: `%s`\n", report.BaseURL))
	}
	b.WriteString(fmt.Sprintf("- Prompt: `%s`\n\n", report.Prompt))
	b.WriteString("| Model | Status | Preview |\n")
	b.WriteString("| --- | --- | --- |\n")
	for _, result := range report.Results {
		preview := result.OutputPreview
		if result.Error != "" {
			preview = result.Error
		}
		b.WriteString(fmt.Sprintf("| `%s` | `%s` | %s |\n", result.Model, result.Status, escapePipes(preview)))
	}
	return b.String()
}

func escapePipes(text string) string {
	return strings.ReplaceAll(text, "|", "\\|")
}

func chooseRunID(runID string, prefix string) string {
	runID = strings.TrimSpace(runID)
	if runID != "" {
		return runID
	}
	return prefix + "-" + timestampToken()
}

func resolvedBaseURL(providerName string, baseURL string) string {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL != "" {
		return baseURL
	}
	switch providerName {
	case "siliconflow":
		return "https://api.siliconflow.cn/v1"
	case "duojie":
		return "https://api.duojie.games/v1"
	default:
		return ""
	}
}
