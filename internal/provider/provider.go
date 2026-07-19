package provider

import "context"

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
