package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"vermory/internal/artifact"
	"vermory/internal/packet"
	"vermory/internal/provider"
	"vermory/internal/runner"
)

type EvalSelfCaseOptions struct {
	DatabaseURL     string
	ArtifactRoot    string
	Provider        string
	BaseURL         string
	APIKeyEnv       string
	Model           string
	RunID           string
	MaxTokens       int
	DisableThinking bool
}

func EvalSelfCase(ctx context.Context, opts EvalSelfCaseOptions) (runner.EvaluationReport, error) {
	if strings.TrimSpace(opts.ArtifactRoot) == "" {
		opts.ArtifactRoot = "./artifacts"
	}

	root, err := projectRoot()
	if err != nil {
		return runner.EvaluationReport{}, err
	}
	if strings.TrimSpace(opts.DatabaseURL) != "" {
		if err := migrate(ctx, opts.DatabaseURL, filepath.Join(root, "internal", "store", "postgres", "migrations")); err != nil {
			return runner.EvaluationReport{}, err
		}
	}
	caseDir := filepath.Join(root, "casebook", "cases", selfCaseID)
	fixtureClaims, err := readFixtureClaims(filepath.Join(caseDir, "claims.json"))
	if err != nil {
		return runner.EvaluationReport{}, err
	}
	tasks, err := readTasks(filepath.Join(caseDir, "tasks.json"))
	if err != nil {
		return runner.EvaluationReport{}, err
	}
	if len(tasks) == 0 {
		return runner.EvaluationReport{}, errors.New("self-case has no WCEF tasks")
	}

	packetClaims := preparePacketClaims(toConfirmedDomainClaims(fixtureClaims))
	contextPacket := packet.Build(packet.ProfileCodingAgent, "ContextMesh", "AI 工具直连评测", packetClaims)

	llm, providerMode, providerName, model, err := buildProvider(opts)
	if err != nil {
		return runner.EvaluationReport{}, err
	}

	return runner.RunEvaluation(ctx, llm, runner.EvaluationOptions{
		RunID:         opts.RunID,
		ProviderMode:  providerMode,
		ProviderName:  providerName,
		Model:         model,
		ArtifactStore: artifact.NewLocalStore(opts.ArtifactRoot),
		Task:          tasks[0],
		StaleContext:  defaultStaleContext(),
		PlainSummary:  plainSummary(packetClaims),
		ContextPacket: contextPacket.Body,
		MaxTokens:     opts.MaxTokens,
		SystemPrompt:  "你是一个真实模型评测对象。请只根据用户任务和提供的上下文作答，不要编造未给出的项目事实。",
	})
}

func buildProvider(opts EvalSelfCaseOptions) (provider.Provider, string, string, string, error) {
	providerName := strings.TrimSpace(opts.Provider)
	if providerName == "" {
		providerName = "mock"
	}

	switch providerName {
	case "mock":
		model := strings.TrimSpace(opts.Model)
		if model == "" {
			model = "mock-model"
		}
		return provider.Mock{}, "mock", "mock", model, nil
	case "grok-cli":
		model := strings.TrimSpace(opts.Model)
		if model == "" {
			model = "grok-4.5"
		}
		return provider.NewGrokCLI(provider.GrokCLIConfig{}), "real", providerName, model, nil
	case "openai-compatible", "siliconflow", "duojie":
		baseURL, apiKeyEnv := providerRuntimeInputs(providerName, opts)
		model := strings.TrimSpace(opts.Model)
		if model == "" {
			return nil, "", "", "", fmt.Errorf("%s provider requires --model", providerName)
		}
		apiKey := os.Getenv(apiKeyEnv)
		if strings.TrimSpace(apiKey) == "" {
			return nil, "", "", "", fmt.Errorf("%s provider requires non-empty env %s", providerName, apiKeyEnv)
		}
		return provider.NewOpenAICompatible(provider.Config{
			BaseURL:         baseURL,
			APIKey:          apiKey,
			DisableThinking: opts.DisableThinking,
		}), "real", providerName, model, nil
	default:
		return nil, "", "", "", fmt.Errorf("unsupported provider %q", providerName)
	}
}

func providerRuntimeInputs(providerName string, opts EvalSelfCaseOptions) (string, string) {
	baseURL := strings.TrimRight(strings.TrimSpace(opts.BaseURL), "/")
	apiKeyEnv := strings.TrimSpace(opts.APIKeyEnv)
	switch providerName {
	case "siliconflow":
		if baseURL == "" {
			baseURL = "https://api.siliconflow.cn/v1"
		}
		if apiKeyEnv == "" {
			apiKeyEnv = "SILICONFLOW_API_KEY"
		}
	case "duojie":
		if baseURL == "" {
			baseURL = "https://api.duojie.games/v1"
		}
		if apiKeyEnv == "" {
			apiKeyEnv = "DUOJIE_API_KEY"
		}
	default:
		if apiKeyEnv == "" {
			apiKeyEnv = "CONTEXTMESH_PROVIDER_API_KEY"
		}
	}
	return baseURL, apiKeyEnv
}
