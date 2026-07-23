package provider

import (
	"strings"
	"testing"
)

func TestBuildDirectProviderUsesStableProviderDefaults(t *testing.T) {
	t.Setenv("SILICONFLOW_API_KEY", "siliconflow-secret")
	llm, name, model, err := BuildDirect(DirectOptions{
		Name: "siliconflow", Model: "deepseek-ai/DeepSeek-V4-Flash", DisableThinking: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	client, ok := llm.(*OpenAICompatible)
	if !ok || name != "siliconflow" || model != "deepseek-ai/DeepSeek-V4-Flash" {
		t.Fatalf("unexpected SiliconFlow provider: llm=%T name=%q model=%q", llm, name, model)
	}
	if client.baseURL != "https://api.siliconflow.cn/v1" || client.apiKey != "siliconflow-secret" || !client.disableThinking {
		t.Fatalf("unexpected SiliconFlow configuration: %#v", client)
	}

	t.Setenv("DUOJIE_API_KEY", "duojie-secret")
	llm, name, model, err = BuildDirect(DirectOptions{Name: "duojie", Model: "glm-5.1"})
	if err != nil {
		t.Fatal(err)
	}
	client, ok = llm.(*OpenAICompatible)
	if !ok || name != "duojie" || model != "glm-5.1" || client.baseURL != "https://api.duojie.games/v1" || client.apiKey != "duojie-secret" {
		t.Fatalf("unexpected Duojie provider: llm=%T name=%q model=%q client=%#v", llm, name, model, client)
	}
}

func TestBuildDirectProviderUsesGrokDefaultsAndRejectsIncompleteHTTPProvider(t *testing.T) {
	llm, name, model, err := BuildDirect(DirectOptions{Name: "grok-cli", GrokCommand: "/tmp/fake-grok"})
	if err != nil {
		t.Fatal(err)
	}
	grok, ok := llm.(*GrokCLI)
	if !ok || name != "grok-cli" || model != "grok-4.5" || grok.command != "/tmp/fake-grok" {
		t.Fatalf("unexpected Grok provider: llm=%T name=%q model=%q provider=%#v", llm, name, model, grok)
	}

	if _, _, _, err := BuildDirect(DirectOptions{Name: "openai-compatible", Model: "model", APIKeyEnv: "MISSING_DIRECT_KEY"}); err == nil || !strings.Contains(err.Error(), "--base-url") {
		t.Fatalf("missing base URL was accepted: %v", err)
	}
	t.Setenv("MISSING_DIRECT_KEY", "")
	if _, _, _, err := BuildDirect(DirectOptions{Name: "openai-compatible", Model: "model", BaseURL: "https://example.invalid/v1", APIKeyEnv: "MISSING_DIRECT_KEY"}); err == nil || !strings.Contains(err.Error(), "MISSING_DIRECT_KEY") {
		t.Fatalf("missing API key environment was accepted: %v", err)
	}
	if _, _, _, err := BuildDirect(DirectOptions{Name: "unsupported"}); err == nil {
		t.Fatal("unsupported direct provider was accepted")
	}
}
