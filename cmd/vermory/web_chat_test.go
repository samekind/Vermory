package main

import (
	"testing"
)

func TestWebChatCommandDefaultsToLoopback(t *testing.T) {
	command := newWebChatCommand()
	listen := command.Flags().Lookup("listen")
	if listen == nil || listen.DefValue != "127.0.0.1:8787" {
		t.Fatalf("unexpected listen default: %#v", listen)
	}
}

func TestWebChatOptionsRequireDatabaseTenantAndLoopback(t *testing.T) {
	for name, options := range map[string]webChatOptions{
		"database": {TenantID: "local", Listen: "127.0.0.1:8787"},
		"tenant":   {DatabaseURL: "postgresql:///vermory", Listen: "127.0.0.1:8787"},
		"loopback": {DatabaseURL: "postgresql:///vermory", TenantID: "local", Listen: "0.0.0.0:8787"},
	} {
		t.Run(name, func(t *testing.T) {
			if err := options.Validate(); err == nil {
				t.Fatalf("invalid options were accepted: %#v", options)
			}
		})
	}
}

func TestBuildWebChatProviderSupportsConfiguredRuntimeProviders(t *testing.T) {
	t.Setenv("TEST_OPENAI_KEY", "test-key")
	t.Setenv("SILICONFLOW_API_KEY", "test-key")
	t.Setenv("DUOJIE_API_KEY", "test-key")

	tests := []webChatProviderOptions{
		{Name: "mock"},
		{Name: "grok-cli"},
		{Name: "openai-compatible", Model: "custom-model", BaseURL: "https://example.test/v1", APIKeyEnv: "TEST_OPENAI_KEY"},
		{Name: "siliconflow", Model: "deepseek-ai/DeepSeek-V4-Flash"},
		{Name: "duojie", Model: "glm-5"},
	}
	for _, options := range tests {
		t.Run(options.Name, func(t *testing.T) {
			llm, model, err := buildWebChatProvider(options)
			if err != nil {
				t.Fatal(err)
			}
			if llm == nil || model == "" {
				t.Fatalf("provider was not configured: provider=%T model=%q", llm, model)
			}
		})
	}
}

func TestBuildWebChatProviderRejectsMissingOpenAICompatibleInputs(t *testing.T) {
	if _, _, err := buildWebChatProvider(webChatProviderOptions{Name: "openai-compatible"}); err == nil {
		t.Fatal("openai-compatible provider without model/base URL/key was accepted")
	}
}

func TestBuildWebChatProviderSupportsExternalHarnessMode(t *testing.T) {
	llm, model, err := buildWebChatProvider(webChatProviderOptions{Name: "external"})
	if err != nil {
		t.Fatal(err)
	}
	if llm != nil || model != "" {
		t.Fatalf("external mode configured an in-process provider: provider=%T model=%q", llm, model)
	}
}
