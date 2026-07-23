package provider

import (
	"context"
	"errors"
	"fmt"
)

type GenerateRequest struct {
	Model         string
	System        string
	Prompt        string
	ContextPacket string
	MaxTokens     int
	Temperature   *float64
	JSONSchema    string
}

type TokenUsage struct {
	InputTokens       int `json:"input_tokens"`
	CachedInputTokens int `json:"cached_input_tokens"`
	OutputTokens      int `json:"output_tokens"`
	ReasoningTokens   int `json:"reasoning_tokens"`
	TotalTokens       int `json:"total_tokens"`
}

type GenerateResponse struct {
	Output      string
	RawArtifact []byte
	Model       string
	Usage       *TokenUsage
}

type Provider interface {
	Generate(ctx context.Context, req GenerateRequest) (GenerateResponse, error)
}

type HTTPStatusError struct {
	StatusCode int
	Status     string
	Body       string
}

func (err *HTTPStatusError) Error() string {
	return fmt.Sprintf("provider: chat completions returned %s: %s", err.Status, err.Body)
}

func ShouldRetry(err error) bool {
	var statusErr *HTTPStatusError
	if errors.As(err, &statusErr) {
		return shouldRetryStatus(statusErr.StatusCode)
	}
	return true
}
