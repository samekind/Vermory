package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	maxBusyRetries = 3
	retryDelay     = 2 * time.Second
)

type Config struct {
	BaseURL         string
	APIKey          string
	Client          *http.Client
	DisableThinking bool
}

type OpenAICompatible struct {
	baseURL         string
	apiKey          string
	client          *http.Client
	disableThinking bool
}

func NewOpenAICompatible(config Config) *OpenAICompatible {
	client := config.Client
	if client == nil {
		client = &http.Client{Timeout: 90 * time.Second}
	}
	return &OpenAICompatible{
		baseURL:         strings.TrimRight(strings.TrimSpace(config.BaseURL), "/"),
		apiKey:          strings.TrimSpace(config.APIKey),
		client:          client,
		disableThinking: config.DisableThinking,
	}
}

func (p *OpenAICompatible) Generate(ctx context.Context, req GenerateRequest) (GenerateResponse, error) {
	if p.baseURL == "" {
		return GenerateResponse{}, errors.New("provider: base URL is required")
	}
	if p.apiKey == "" {
		return GenerateResponse{}, errors.New("provider: API key is required")
	}
	if strings.TrimSpace(req.Model) == "" {
		return GenerateResponse{}, errors.New("provider: model is required")
	}

	request := openAICompatibleRequest{
		Model:       req.Model,
		Messages:    buildMessages(req),
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
	}
	if p.disableThinking {
		enabled := false
		request.EnableThinking = &enabled
	}
	body, err := json.Marshal(request)
	if err != nil {
		return GenerateResponse{}, err
	}

	raw, err := p.doChatCompletion(ctx, body)
	if err != nil {
		return GenerateResponse{}, err
	}

	var decoded openAICompatibleResponse
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return GenerateResponse{}, err
	}
	output := decoded.FirstContent()
	if strings.TrimSpace(output) == "" {
		return GenerateResponse{}, errors.New("provider: response did not contain assistant content")
	}
	model := decoded.Model
	if model == "" {
		model = req.Model
	}

	return GenerateResponse{
		Output:      sanitizeModelOutput(output),
		RawArtifact: raw,
		Model:       model,
		Usage:       decoded.Usage.normalized(),
	}, nil
}

func (p *OpenAICompatible) doChatCompletion(ctx context.Context, body []byte) ([]byte, error) {
	url := chatCompletionsURL(p.baseURL)
	var lastErr error
	for attempt := 0; attempt < maxBusyRetries; attempt++ {
		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("Accept", "application/json")

		resp, err := p.client.Do(httpReq)
		if err != nil {
			return nil, err
		}
		raw, readErr := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
		resp.Body.Close()
		if readErr != nil {
			return nil, readErr
		}
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return raw, nil
		}

		lastErr = fmt.Errorf("provider: chat completions returned %s: %s", resp.Status, trimForError(raw))
		if !shouldRetryStatus(resp.StatusCode) || attempt == maxBusyRetries-1 {
			return nil, lastErr
		}
		if err := sleepWithContext(ctx, retryDelay); err != nil {
			return nil, err
		}
	}
	return nil, lastErr
}

type openAICompatibleRequest struct {
	Model          string                `json:"model"`
	Messages       []openAICompatibleMsg `json:"messages"`
	MaxTokens      int                   `json:"max_tokens,omitempty"`
	Temperature    *float64              `json:"temperature,omitempty"`
	EnableThinking *bool                 `json:"enable_thinking,omitempty"`
}

type openAICompatibleMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func buildMessages(req GenerateRequest) []openAICompatibleMsg {
	messages := make([]openAICompatibleMsg, 0, 2)
	if strings.TrimSpace(req.System) != "" {
		messages = append(messages, openAICompatibleMsg{Role: "system", Content: req.System})
	}
	messages = append(messages, openAICompatibleMsg{Role: "user", Content: buildUserPrompt(req)})
	return messages
}

func buildUserPrompt(req GenerateRequest) string {
	contextPacket := strings.TrimSpace(req.ContextPacket)
	jsonSchema := strings.TrimSpace(req.JSONSchema)
	if contextPacket == "" && jsonSchema == "" {
		return req.Prompt
	}
	sections := make([]string, 0, 3)
	if contextPacket != "" {
		sections = append(sections, "Context packet:\n"+req.ContextPacket)
	}
	sections = append(sections, "Task:\n"+req.Prompt)
	if jsonSchema != "" {
		sections = append(sections, "Required JSON schema:\n"+jsonSchema)
	}
	return strings.Join(sections, "\n\n")
}

func chatCompletionsURL(baseURL string) string {
	if strings.HasSuffix(baseURL, "/chat/completions") {
		return baseURL
	}
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return baseURL + "/chat/completions"
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + "/chat/completions"
	return parsed.String()
}

type openAICompatibleResponse struct {
	Model   string `json:"model"`
	Choices []struct {
		Message struct {
			Content          json.RawMessage `json:"content"`
			ReasoningContent string          `json:"reasoning_content"`
		} `json:"message"`
	} `json:"choices"`
	Usage *openAICompatibleUsage `json:"usage"`
}

type openAICompatibleUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
	PromptDetails    struct {
		CachedTokens int `json:"cached_tokens"`
	} `json:"prompt_tokens_details"`
	CompletionDetails struct {
		ReasoningTokens int `json:"reasoning_tokens"`
	} `json:"completion_tokens_details"`
}

func (usage *openAICompatibleUsage) normalized() *TokenUsage {
	if usage == nil {
		return nil
	}
	return &TokenUsage{
		InputTokens:       usage.PromptTokens,
		CachedInputTokens: usage.PromptDetails.CachedTokens,
		OutputTokens:      usage.CompletionTokens,
		ReasoningTokens:   usage.CompletionDetails.ReasoningTokens,
		TotalTokens:       usage.TotalTokens,
	}
}

func (r openAICompatibleResponse) FirstContent() string {
	if len(r.Choices) == 0 {
		return ""
	}
	message := r.Choices[0].Message
	content := message.Content
	var text string
	if err := json.Unmarshal(content, &text); err == nil {
		if strings.TrimSpace(text) != "" {
			return text
		}
	}
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(content, &parts); err == nil {
		var b strings.Builder
		for _, part := range parts {
			if part.Text == "" {
				continue
			}
			if b.Len() > 0 {
				b.WriteString("\n")
			}
			b.WriteString(part.Text)
		}
		if strings.TrimSpace(b.String()) != "" {
			return b.String()
		}
	}
	if strings.TrimSpace(message.ReasoningContent) != "" {
		return message.ReasoningContent
	}
	return string(content)
}

func sanitizeModelOutput(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = stripBalancedThinkBlocks(text)
	text = stripLeakedThinkClosures(text)
	text = strings.ReplaceAll(text, "<think>", "")
	text = strings.ReplaceAll(text, "</think>", "")
	return strings.TrimSpace(text)
}

func stripBalancedThinkBlocks(text string) string {
	for {
		start := strings.Index(text, "<think>")
		if start == -1 {
			return text
		}
		end := strings.Index(text[start+len("<think>"):], "</think>")
		if end == -1 {
			return text
		}
		end += start + len("<think>")
		text = text[:start] + text[end+len("</think>"):]
	}
}

func stripLeakedThinkClosures(text string) string {
	if !strings.Contains(text, "</think>") {
		return text
	}
	parts := strings.Split(text, "</think>")
	for _, part := range parts {
		part = strings.TrimSpace(strings.ReplaceAll(part, "<think>", ""))
		if part != "" {
			return part
		}
	}
	return ""
}

func trimForError(raw []byte) string {
	text := strings.TrimSpace(string(raw))
	if len(text) <= 1000 {
		return text
	}
	return text[:1000] + "...(truncated)"
}

func shouldRetryStatus(statusCode int) bool {
	switch statusCode {
	case http.StatusTooManyRequests, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
}

func sleepWithContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
