package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"vermory/internal/memorybackend"
	vermoryruntime "vermory/internal/runtime"
)

type LongMemEvalVectorProfile struct {
	SchemaVersion                  string   `json:"schema_version"`
	ID                             string   `json:"id"`
	Provider                       string   `json:"provider"`
	RetrievalProfile               string   `json:"retrieval_profile"`
	BaseURL                        string   `json:"base_url"`
	Model                          string   `json:"model"`
	Dimensions                     int      `json:"dimensions"`
	ProjectionClass                string   `json:"projection_class"`
	WorkerBatchSize                int      `json:"worker_batch_size"`
	EmbeddingBatchSize             int      `json:"embedding_batch_size"`
	HTTPTimeoutSeconds             int      `json:"http_timeout_seconds"`
	MaxAttempts                    int      `json:"max_attempts"`
	RetryDelayMilliseconds         int      `json:"retry_delay_milliseconds"`
	RetryBackoff                   string   `json:"retry_backoff,omitempty"`
	ProjectionMaxRecoveries        int      `json:"projection_max_recoveries,omitempty"`
	ProjectionRecoveryDelaySeconds int      `json:"projection_recovery_delay_seconds,omitempty"`
	Conditions                     []string `json:"conditions"`
	HardGates                      []string `json:"hard_gates"`
	NonClaims                      []string `json:"non_claims"`
}

func (profile LongMemEvalVectorProfile) Validate() error {
	if profile.SchemaVersion != "longmemeval-vector-profile/v1" {
		return fmt.Errorf("unsupported LongMemEval vector profile schema %q", profile.SchemaVersion)
	}
	if strings.TrimSpace(profile.ID) == "" {
		return fmt.Errorf("LongMemEval vector profile ID is required")
	}
	if profile.Provider != "siliconflow-direct" {
		return fmt.Errorf("LongMemEval vector provider must be siliconflow-direct")
	}
	spec, ok := vermoryruntime.SupportedRetrievalProfile(profile.RetrievalProfile)
	if !ok {
		return fmt.Errorf("unsupported LongMemEval retrieval profile %q", profile.RetrievalProfile)
	}
	if profile.BaseURL != spec.BaseURL || profile.Model != spec.Model || profile.Dimensions != spec.Dimensions || profile.ProjectionClass != string(spec.ProjectionClass) {
		return fmt.Errorf("LongMemEval vector profile does not match registered retrieval profile %q", spec.ID)
	}
	if profile.WorkerBatchSize <= 0 || profile.WorkerBatchSize > 256 {
		return fmt.Errorf("LongMemEval vector worker batch size must be between 1 and 256")
	}
	if profile.EmbeddingBatchSize <= 1 || profile.EmbeddingBatchSize > profile.WorkerBatchSize {
		return fmt.Errorf("LongMemEval embedding batch size must be between 2 and the worker batch size")
	}
	if profile.HTTPTimeoutSeconds <= 0 || profile.HTTPTimeoutSeconds > 600 {
		return fmt.Errorf("LongMemEval embedding timeout must be between 1 and 600 seconds")
	}
	if profile.MaxAttempts <= 0 || profile.MaxAttempts > 5 {
		return fmt.Errorf("LongMemEval embedding attempts must be between 1 and 5")
	}
	if profile.RetryDelayMilliseconds < 0 || profile.RetryDelayMilliseconds > 60000 {
		return fmt.Errorf("LongMemEval embedding retry delay must be between 0 and 60000 milliseconds")
	}
	if profile.RetryBackoff != "" && profile.RetryBackoff != "fixed" && profile.RetryBackoff != "linear" {
		return fmt.Errorf("LongMemEval embedding retry backoff must be fixed or linear")
	}
	if profile.ProjectionMaxRecoveries < 0 || profile.ProjectionMaxRecoveries > 100 {
		return fmt.Errorf("LongMemEval projection recoveries must be between 0 and 100")
	}
	if profile.ProjectionRecoveryDelaySeconds < 0 || profile.ProjectionRecoveryDelaySeconds > 3600 {
		return fmt.Errorf("LongMemEval projection recovery delay must be between 0 and 3600 seconds")
	}
	if profile.ProjectionMaxRecoveries == 0 && profile.ProjectionRecoveryDelaySeconds != 0 {
		return fmt.Errorf("LongMemEval projection recovery delay requires a positive recovery budget")
	}
	if profile.ProjectionMaxRecoveries > 0 && profile.ProjectionRecoveryDelaySeconds == 0 {
		return fmt.Errorf("LongMemEval projection recovery budget requires a positive delay")
	}
	if !slices.Equal(profile.Conditions, longMemEvalVectorConditions()) {
		return fmt.Errorf("LongMemEval vector conditions are %v, want %v", profile.Conditions, longMemEvalVectorConditions())
	}
	if len(profile.HardGates) == 0 || len(profile.NonClaims) == 0 {
		return fmt.Errorf("LongMemEval vector profile requires hard gates and non-claims")
	}
	return nil
}

func (profile LongMemEvalVectorProfile) runtimeProfile() vermoryruntime.RetrievalProfile {
	return vermoryruntime.RetrievalProfile{
		ID:              profile.RetrievalProfile,
		BaseURL:         profile.BaseURL,
		Model:           profile.Model,
		Dimensions:      profile.Dimensions,
		ProjectionClass: vermoryruntime.ProjectionClass(profile.ProjectionClass),
	}
}

func loadLongMemEvalVectorProfile(path string) (LongMemEvalVectorProfile, string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return LongMemEvalVectorProfile{}, "", err
	}
	var profile LongMemEvalVectorProfile
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&profile); err != nil {
		return LongMemEvalVectorProfile{}, "", fmt.Errorf("decode LongMemEval vector profile: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return LongMemEvalVectorProfile{}, "", fmt.Errorf("decode LongMemEval vector profile: trailing JSON")
	}
	if err := profile.Validate(); err != nil {
		return LongMemEvalVectorProfile{}, "", err
	}
	digest := sha256.Sum256(data)
	return profile, hex.EncodeToString(digest[:]), nil
}

type longMemEvalEmbeddingStats struct {
	LogicalOperations int64 `json:"logical_operations"`
	ProviderAttempts  int64 `json:"provider_attempts"`
	AttemptedItems    int64 `json:"attempted_items"`
	SuccessfulItems   int64 `json:"successful_items"`
	FailedAttempts    int64 `json:"failed_attempts"`
	RetriedOperations int64 `json:"retried_operations"`
	TerminalFailures  int64 `json:"terminal_failures"`
}

type longMemEvalRetryingEmbedder struct {
	delegate          vermoryruntime.Embedder
	batchDelegate     vermoryruntime.BatchEmbedder
	maxAttempts       int
	retryDelay        time.Duration
	retryBackoff      string
	sleeper           func(context.Context, time.Duration) error
	logicalOperations atomic.Int64
	providerAttempts  atomic.Int64
	attemptedItems    atomic.Int64
	successfulItems   atomic.Int64
	failedAttempts    atomic.Int64
	retriedOperations atomic.Int64
	terminalFailures  atomic.Int64
}

func newLongMemEvalRetryingEmbedder(delegate vermoryruntime.Embedder, profile LongMemEvalVectorProfile) (*longMemEvalRetryingEmbedder, error) {
	if delegate == nil {
		return nil, fmt.Errorf("LongMemEval vector embedder is required")
	}
	batch, ok := delegate.(vermoryruntime.BatchEmbedder)
	if !ok {
		return nil, fmt.Errorf("LongMemEval vector embedder must support batch requests")
	}
	return &longMemEvalRetryingEmbedder{
		delegate: delegate, batchDelegate: batch,
		maxAttempts:  profile.MaxAttempts,
		retryDelay:   time.Duration(profile.RetryDelayMilliseconds) * time.Millisecond,
		retryBackoff: profile.RetryBackoff,
		sleeper:      sleepLongMemEvalEmbeddingRetry,
	}, nil
}

func (embedder *longMemEvalRetryingEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	embedder.logicalOperations.Add(1)
	var lastErr error
	for attempt := 1; attempt <= embedder.maxAttempts; attempt++ {
		embedder.providerAttempts.Add(1)
		embedder.attemptedItems.Add(1)
		vector, err := embedder.delegate.Embed(ctx, text)
		if err == nil {
			embedder.successfulItems.Add(1)
			if attempt > 1 {
				embedder.retriedOperations.Add(1)
			}
			return vector, nil
		}
		lastErr = err
		embedder.failedAttempts.Add(1)
		if attempt < embedder.maxAttempts {
			if err := embedder.sleeper(ctx, embedder.retryDelayForAttempt(attempt)); err != nil {
				return nil, err
			}
		}
	}
	embedder.terminalFailures.Add(1)
	return nil, lastErr
}

func (embedder *longMemEvalRetryingEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	embedder.logicalOperations.Add(1)
	var lastErr error
	for attempt := 1; attempt <= embedder.maxAttempts; attempt++ {
		embedder.providerAttempts.Add(1)
		embedder.attemptedItems.Add(int64(len(texts)))
		vectors, err := embedder.batchDelegate.EmbedBatch(ctx, texts)
		if err == nil {
			embedder.successfulItems.Add(int64(len(texts)))
			if attempt > 1 {
				embedder.retriedOperations.Add(1)
			}
			return vectors, nil
		}
		lastErr = err
		embedder.failedAttempts.Add(1)
		if attempt < embedder.maxAttempts {
			if err := embedder.sleeper(ctx, embedder.retryDelayForAttempt(attempt)); err != nil {
				return nil, err
			}
		}
	}
	embedder.terminalFailures.Add(1)
	return nil, lastErr
}

func (embedder *longMemEvalRetryingEmbedder) retryDelayForAttempt(attempt int) time.Duration {
	if embedder.retryBackoff == "linear" {
		return time.Duration(attempt) * embedder.retryDelay
	}
	return embedder.retryDelay
}

func (embedder *longMemEvalRetryingEmbedder) stats() longMemEvalEmbeddingStats {
	return longMemEvalEmbeddingStats{
		LogicalOperations: embedder.logicalOperations.Load(),
		ProviderAttempts:  embedder.providerAttempts.Load(),
		AttemptedItems:    embedder.attemptedItems.Load(),
		SuccessfulItems:   embedder.successfulItems.Load(),
		FailedAttempts:    embedder.failedAttempts.Load(),
		RetriedOperations: embedder.retriedOperations.Load(),
		TerminalFailures:  embedder.terminalFailures.Load(),
	}
}

func sleepLongMemEvalEmbeddingRetry(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func configureLongMemEvalVectorEmbedder(options LongMemEvalRetrievalOptions, profile LongMemEvalVectorProfile) (*longMemEvalRetryingEmbedder, error) {
	delegate := options.VectorEmbedder
	if delegate == nil {
		if strings.TrimSpace(options.EmbeddingAPIKey) == "" {
			return nil, fmt.Errorf("embedding API key is required for LongMemEval vector retrieval")
		}
		configured, err := memorybackend.NewOpenAIEmbedder(
			profile.BaseURL,
			options.EmbeddingAPIKey,
			profile.Model,
			profile.Dimensions,
			&http.Client{Timeout: time.Duration(profile.HTTPTimeoutSeconds) * time.Second},
		)
		if err != nil {
			return nil, fmt.Errorf("configure LongMemEval embedding provider")
		}
		delegate = configured
	}
	return newLongMemEvalRetryingEmbedder(delegate, profile)
}
