package provider

import (
	"fmt"
	"os"
	"strings"
)

type DirectOptions struct {
	Name            string
	Model           string
	BaseURL         string
	APIKeyEnv       string
	GrokCommand     string
	DisableThinking bool
}

func BuildDirect(options DirectOptions) (Provider, string, string, error) {
	name := strings.TrimSpace(options.Name)
	if name == "" {
		name = "grok-cli"
	}
	model := strings.TrimSpace(options.Model)
	switch name {
	case "grok-cli":
		if model == "" {
			model = "grok-4.5"
		}
		return NewGrokCLI(GrokCLIConfig{Command: options.GrokCommand}), name, model, nil
	case "openai-compatible", "siliconflow", "duojie":
		baseURL := strings.TrimRight(strings.TrimSpace(options.BaseURL), "/")
		apiKeyEnv := strings.TrimSpace(options.APIKeyEnv)
		switch name {
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
				apiKeyEnv = "VERMORY_PROVIDER_API_KEY"
			}
		}
		if model == "" {
			return nil, "", "", fmt.Errorf("%s provider requires --model", name)
		}
		if baseURL == "" {
			return nil, "", "", fmt.Errorf("%s provider requires --base-url", name)
		}
		apiKey := strings.TrimSpace(os.Getenv(apiKeyEnv))
		if apiKey == "" {
			return nil, "", "", fmt.Errorf("%s provider requires non-empty env %s", name, apiKeyEnv)
		}
		return NewOpenAICompatible(Config{
			BaseURL:         baseURL,
			APIKey:          apiKey,
			DisableThinking: options.DisableThinking,
		}), name, model, nil
	default:
		return nil, "", "", fmt.Errorf("unsupported direct provider %q", name)
	}
}
