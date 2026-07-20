package provider

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOpenAICompatibleProviderSendsDirectChatCompletionRequest(t *testing.T) {
	var captured struct {
		Model    string `json:"model"`
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
		MaxTokens      int      `json:"max_tokens"`
		Temperature    *float64 `json:"temperature"`
		EnableThinking *bool    `json:"enable_thinking"`
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("expected direct chat completions path, got %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("expected bearer auth, got %q", got)
		}
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl-test","model":"direct-model","choices":[{"message":{"role":"assistant","content":"direct ok"}}],"usage":{"prompt_tokens":120,"completion_tokens":15,"total_tokens":135,"prompt_tokens_details":{"cached_tokens":80},"completion_tokens_details":{"reasoning_tokens":7}}}`))
	}))
	defer server.Close()

	client := NewOpenAICompatible(Config{
		BaseURL: server.URL + "/v1",
		APIKey:  "test-key",
		Client:  server.Client(),
	})

	resp, err := client.Generate(context.Background(), GenerateRequest{
		Model:         "direct-model",
		System:        "system instructions",
		Prompt:        "finish the task",
		ContextPacket: "confirmed context packet",
		MaxTokens:     77,
		Temperature:   float64Pointer(0),
		JSONSchema:    `{"type":"object","required":["result"]}`,
	})
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}

	if resp.Output != "direct ok" {
		t.Fatalf("unexpected output: %q", resp.Output)
	}
	if captured.Model != "direct-model" {
		t.Fatalf("unexpected model: %q", captured.Model)
	}
	if captured.MaxTokens != 77 {
		t.Fatalf("unexpected max tokens: %d", captured.MaxTokens)
	}
	if captured.EnableThinking != nil {
		t.Fatalf("default request unexpectedly set enable_thinking: %v", *captured.EnableThinking)
	}
	if captured.Temperature == nil || *captured.Temperature != 0 {
		t.Fatalf("expected temperature=0, got %v", captured.Temperature)
	}
	if len(captured.Messages) != 2 {
		t.Fatalf("expected system and user messages, got %d", len(captured.Messages))
	}
	if captured.Messages[0].Role != "system" || captured.Messages[0].Content != "system instructions" {
		t.Fatalf("unexpected system message: %#v", captured.Messages[0])
	}
	userContent := captured.Messages[1].Content
	if !strings.Contains(userContent, "confirmed context packet") || !strings.Contains(userContent, "finish the task") {
		t.Fatalf("user message did not combine packet and prompt: %q", userContent)
	}
	if !strings.Contains(userContent, "Required JSON schema:") || !strings.Contains(userContent, `"required":["result"]`) {
		t.Fatalf("user message omitted the required JSON schema: %q", userContent)
	}
	if !json.Valid(resp.RawArtifact) {
		t.Fatalf("raw artifact should be response JSON")
	}
	if resp.Usage == nil || *resp.Usage != (TokenUsage{
		InputTokens:       120,
		CachedInputTokens: 80,
		OutputTokens:      15,
		ReasoningTokens:   7,
		TotalTokens:       135,
	}) {
		t.Fatalf("unexpected OpenAI-compatible usage: %#v", resp.Usage)
	}
}

func float64Pointer(value float64) *float64 {
	return &value
}

func TestOpenAICompatibleProviderCanDisableThinking(t *testing.T) {
	var enableThinking *bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			EnableThinking *bool `json:"enable_thinking"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		enableThinking = body.EnableThinking
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"model":"direct-model","choices":[{"message":{"content":"direct ok"}}]}`))
	}))
	defer server.Close()

	client := NewOpenAICompatible(Config{
		BaseURL:         server.URL + "/v1",
		APIKey:          "test-key",
		Client:          server.Client(),
		DisableThinking: true,
	})
	if _, err := client.Generate(context.Background(), GenerateRequest{
		Model:     "direct-model",
		Prompt:    "extract facts",
		MaxTokens: 1024,
	}); err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}
	if enableThinking == nil || *enableThinking {
		t.Fatalf("expected enable_thinking=false, got %v", enableThinking)
	}
}

func TestOpenAICompatibleProviderFallsBackToReasoningContentWhenContentIsEmpty(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl-test","model":"glm-5","choices":[{"message":{"role":"assistant","content":"","reasoning_content":"OK from reasoning"}}]}`))
	}))
	defer server.Close()

	client := NewOpenAICompatible(Config{
		BaseURL: server.URL + "/v1",
		APIKey:  "test-key",
		Client:  server.Client(),
	})

	resp, err := client.Generate(context.Background(), GenerateRequest{
		Model:     "glm-5",
		Prompt:    "reply",
		MaxTokens: 16,
	})
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}
	if resp.Output != "OK from reasoning" {
		t.Fatalf("expected reasoning fallback, got %q", resp.Output)
	}
}

func TestOpenAICompatibleProviderSanitizesLeakedThinkTags(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl-test","model":"glm-5.1","choices":[{"message":{"role":"assistant","content":"OK</think>前后文分析：\n无</think>OK"}}]}`))
	}))
	defer server.Close()

	client := NewOpenAICompatible(Config{
		BaseURL: server.URL + "/v1",
		APIKey:  "test-key",
		Client:  server.Client(),
	})

	resp, err := client.Generate(context.Background(), GenerateRequest{
		Model:     "glm-5.1",
		Prompt:    "reply",
		MaxTokens: 16,
	})
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}
	if resp.Output != "OK" {
		t.Fatalf("expected sanitized output OK, got %q", resp.Output)
	}
}

func TestOpenAICompatibleProviderRejectsMissingRuntimeInputs(t *testing.T) {
	_, err := NewOpenAICompatible(Config{
		BaseURL: "https://example.invalid/v1",
	}).Generate(context.Background(), GenerateRequest{Model: "m"})
	if err == nil {
		t.Fatal("expected missing API key error")
	}

	_, err = NewOpenAICompatible(Config{
		APIKey: "test-key",
	}).Generate(context.Background(), GenerateRequest{Model: "m"})
	if err == nil {
		t.Fatal("expected missing base URL error")
	}
}

func TestOpenAICompatibleProviderRetriesBusyResponses(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.Header().Set("Content-Type", "application/json")
		if attempts == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"message":"System is too busy now. Please try again later."}`))
			return
		}
		_, _ = w.Write([]byte(`{"id":"chatcmpl-test","model":"direct-model","choices":[{"message":{"role":"assistant","content":"direct ok after retry"}}]}`))
	}))
	defer server.Close()

	client := NewOpenAICompatible(Config{
		BaseURL: server.URL + "/v1",
		APIKey:  "test-key",
		Client:  server.Client(),
	})

	resp, err := client.Generate(context.Background(), GenerateRequest{
		Model:     "direct-model",
		Prompt:    "finish the task",
		MaxTokens: 32,
	})
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}
	if attempts != 2 {
		t.Fatalf("expected 2 attempts, got %d", attempts)
	}
	if resp.Output != "direct ok after retry" {
		t.Fatalf("unexpected output: %q", resp.Output)
	}
}

func TestOpenAICompatibleProviderClassifiesNonRetryableHTTPStatus(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"code":30001,"message":"account balance is insufficient"}`))
	}))
	defer server.Close()

	client := NewOpenAICompatible(Config{
		BaseURL: server.URL + "/v1",
		APIKey:  "test-key",
		Client:  server.Client(),
	})

	_, err := client.Generate(context.Background(), GenerateRequest{
		Model:     "direct-model",
		Prompt:    "finish the task",
		MaxTokens: 32,
	})
	if err == nil {
		t.Fatal("expected provider error")
	}
	if attempts != 1 {
		t.Fatalf("non-retryable HTTP status made %d provider attempts", attempts)
	}
	var statusErr *HTTPStatusError
	if !errors.As(err, &statusErr) {
		t.Fatalf("expected HTTPStatusError, got %T: %v", err, err)
	}
	if statusErr.StatusCode != http.StatusForbidden || ShouldRetry(err) {
		t.Fatalf("unexpected HTTP error classification: %#v", statusErr)
	}
}
