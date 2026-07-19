package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"vermory/internal/memorybackend"
	"vermory/internal/runtime"

	"github.com/spf13/cobra"
)

type retrievalRuntimeOptions struct {
	Mode                runtime.RetrievalMode
	ProfileID           string
	EmbeddingBaseURL    string
	EmbeddingAPIKeyEnv  string
	EmbeddingModel      string
	EmbeddingDimensions int
}

func defaultRetrievalRuntimeOptions() retrievalRuntimeOptions {
	return retrievalRuntimeOptions{
		Mode:               runtime.RetrievalLexical,
		ProfileID:          runtime.ProductionRetrievalProfileID,
		EmbeddingAPIKeyEnv: "SILICONFLOW_API_KEY",
	}
}

func (options retrievalRuntimeOptions) profile() runtime.RetrievalProfile {
	profileID := strings.TrimSpace(options.ProfileID)
	baseURL := strings.TrimSpace(options.EmbeddingBaseURL)
	model := strings.TrimSpace(options.EmbeddingModel)
	dimensions := options.EmbeddingDimensions
	var projectionClass runtime.ProjectionClass
	if spec, ok := runtime.SupportedRetrievalProfile(profileID); ok {
		projectionClass = spec.ProjectionClass
		if baseURL == "" {
			baseURL = spec.BaseURL
		}
		if model == "" {
			model = spec.Model
		}
		if dimensions == 0 {
			dimensions = spec.Dimensions
		}
	}
	return runtime.RetrievalProfile{
		ID:              profileID,
		BaseURL:         baseURL,
		Model:           model,
		Dimensions:      dimensions,
		ProjectionClass: projectionClass,
	}
}

func (options retrievalRuntimeOptions) validateSemantic() (string, error) {
	if options.Mode != runtime.RetrievalShadow && options.Mode != runtime.RetrievalVector {
		return "", fmt.Errorf("--retrieval-mode must be shadow or vector for semantic retrieval")
	}
	if err := options.profile().Validate(); err != nil {
		return "", err
	}
	apiKeyEnv := strings.TrimSpace(options.EmbeddingAPIKeyEnv)
	if apiKeyEnv == "" {
		return "", fmt.Errorf("--embedding-api-key-env is required")
	}
	apiKey := strings.TrimSpace(os.Getenv(apiKeyEnv))
	if apiKey == "" {
		return "", fmt.Errorf("embedding API key environment variable is not set")
	}
	return apiKey, nil
}

func buildRuntimeRetriever(store *runtime.Store, options retrievalRuntimeOptions) (runtime.MemoryRetriever, error) {
	if options.Mode == "" || options.Mode == runtime.RetrievalLexical {
		return nil, nil
	}
	apiKey, err := options.validateSemantic()
	if err != nil {
		return nil, err
	}
	if store == nil {
		return nil, fmt.Errorf("retrieval store is required")
	}
	profile := options.profile()
	embedder, err := memorybackend.NewOpenAIEmbedder(
		profile.BaseURL,
		apiKey,
		profile.Model,
		profile.Dimensions,
		&http.Client{Timeout: 60 * time.Second},
	)
	if err != nil {
		return nil, fmt.Errorf("configure embedding provider")
	}
	coordinator, err := runtime.NewRetrievalCoordinator(store, embedder, profile)
	if err != nil {
		return nil, err
	}
	return retrievalModeBinding{retriever: coordinator, mode: options.Mode}, nil
}

type retrievalModeBinding struct {
	retriever runtime.MemoryRetriever
	mode      runtime.RetrievalMode
}

func (binding retrievalModeBinding) Retrieve(ctx context.Context, request runtime.RetrievalRequest) (runtime.RetrievalResult, error) {
	request.Mode = binding.mode
	return binding.retriever.Retrieve(ctx, request)
}

type retrievalModeFlag struct {
	target *runtime.RetrievalMode
}

func (flag retrievalModeFlag) String() string {
	if flag.target == nil {
		return ""
	}
	return string(*flag.target)
}

func (flag retrievalModeFlag) Set(value string) error {
	*flag.target = runtime.RetrievalMode(strings.TrimSpace(value))
	return nil
}

func (retrievalModeFlag) Type() string { return "string" }

func addSharedRetrievalFlags(command *cobra.Command, options *retrievalRuntimeOptions) {
	command.Flags().Var(retrievalModeFlag{target: &options.Mode}, "retrieval-mode", "retrieval mode: lexical, shadow, or vector")
	command.Flags().StringVar(&options.ProfileID, "retrieval-profile", options.ProfileID, "retrieval profile identifier")
	command.Flags().StringVar(&options.EmbeddingBaseURL, "embedding-base-url", options.EmbeddingBaseURL, "direct embedding API base URL")
	command.Flags().StringVar(&options.EmbeddingAPIKeyEnv, "embedding-api-key-env", options.EmbeddingAPIKeyEnv, "environment variable containing the embedding API key")
	command.Flags().StringVar(&options.EmbeddingModel, "embedding-model", options.EmbeddingModel, "embedding model name")
	command.Flags().IntVar(&options.EmbeddingDimensions, "embedding-dimensions", options.EmbeddingDimensions, "embedding vector dimensions")
}

type retrievalWorkerCommandOptions struct {
	DatabaseURL        string
	TenantID           string
	ProfileID          string
	Embedding          retrievalRuntimeOptions
	Once               bool
	PollInterval       time.Duration
	BatchSize          int
	EmbeddingBatchSize int
}

func newRetrievalWorkerCommand() *cobra.Command {
	options := retrievalWorkerCommandOptions{
		ProfileID:    runtime.ProductionRetrievalProfileID,
		Embedding:    defaultRetrievalRuntimeOptions(),
		PollInterval: time.Second,
		BatchSize:    32,
	}
	options.Embedding.Mode = runtime.RetrievalVector
	command := &cobra.Command{
		Use:   "retrieval-worker",
		Short: "Process one tenant's durable retrieval projection events",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, args []string) error {
			if strings.TrimSpace(options.DatabaseURL) == "" {
				return fmt.Errorf("--database-url is required")
			}
			if strings.TrimSpace(options.TenantID) == "" {
				return fmt.Errorf("--tenant-id is required")
			}
			options.Embedding.ProfileID = strings.TrimSpace(options.ProfileID)
			apiKey, err := options.Embedding.validateSemantic()
			if err != nil {
				return err
			}
			store, err := runtime.OpenStoreWithOptions(command.Context(), options.DatabaseURL, runtime.StoreOptions{EnforceTenantContext: true})
			if err != nil {
				return fmt.Errorf("open retrieval worker store")
			}
			defer store.Close()
			if err := store.ValidateRuntimeRole(command.Context()); err != nil {
				return err
			}
			profile := options.Embedding.profile()
			embedder, err := memorybackend.NewOpenAIEmbedder(profile.BaseURL, apiKey, profile.Model, profile.Dimensions, &http.Client{Timeout: 60 * time.Second})
			if err != nil {
				return fmt.Errorf("configure embedding provider")
			}
			worker, err := runtime.NewProjectionWorker(store, embedder, runtime.ProjectionWorkerOptions{
				TenantID:           options.TenantID,
				Profile:            profile,
				BatchSize:          options.BatchSize,
				EmbeddingBatchSize: options.EmbeddingBatchSize,
				PollInterval:       options.PollInterval,
			})
			if err != nil {
				return err
			}
			if !options.Once {
				err := worker.Run(command.Context())
				if errors.Is(err, context.Canceled) {
					return nil
				}
				return err
			}
			result, runErr := worker.RunOnce(command.Context())
			if err := json.NewEncoder(command.OutOrStdout()).Encode(result); err != nil {
				return err
			}
			return runErr
		},
	}
	command.Flags().StringVar(&options.DatabaseURL, "database-url", "", "restricted runtime PostgreSQL connection URL")
	command.Flags().StringVar(&options.TenantID, "tenant-id", "", "fixed tenant identifier")
	command.Flags().StringVar(&options.ProfileID, "profile-id", options.ProfileID, "retrieval profile identifier")
	command.Flags().StringVar(&options.Embedding.EmbeddingBaseURL, "embedding-base-url", options.Embedding.EmbeddingBaseURL, "direct embedding API base URL")
	command.Flags().StringVar(&options.Embedding.EmbeddingAPIKeyEnv, "embedding-api-key-env", options.Embedding.EmbeddingAPIKeyEnv, "environment variable containing the embedding API key")
	command.Flags().StringVar(&options.Embedding.EmbeddingModel, "embedding-model", options.Embedding.EmbeddingModel, "embedding model name")
	command.Flags().IntVar(&options.Embedding.EmbeddingDimensions, "embedding-dimensions", options.Embedding.EmbeddingDimensions, "embedding vector dimensions")
	command.Flags().BoolVar(&options.Once, "once", false, "process at most one batch and exit")
	command.Flags().DurationVar(&options.PollInterval, "poll-interval", options.PollInterval, "continuous worker poll interval")
	command.Flags().IntVar(&options.BatchSize, "batch-size", options.BatchSize, "maximum events processed per pass")
	command.Flags().IntVar(&options.EmbeddingBatchSize, "embedding-batch-size", 1, "maximum texts sent per embedding request")
	return command
}

type retrievalSnapshotRebuildCommandOptions struct {
	DatabaseURL        string
	TenantID           string
	ProfileID          string
	Embedding          retrievalRuntimeOptions
	SnapshotPageSize   int
	EmbeddingBatchSize int
}

func newRetrievalSnapshotRebuildCommand() *cobra.Command {
	options := retrievalSnapshotRebuildCommandOptions{
		ProfileID:        runtime.ProductionRetrievalProfileID,
		Embedding:        defaultRetrievalRuntimeOptions(),
		SnapshotPageSize: 128,
	}
	options.Embedding.Mode = runtime.RetrievalVector
	command := &cobra.Command{
		Use:   "retrieval-snapshot-rebuild",
		Short: "Rebuild one tenant's vector projection from current authority",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, args []string) error {
			if strings.TrimSpace(options.DatabaseURL) == "" {
				return fmt.Errorf("--database-url is required")
			}
			if strings.TrimSpace(options.TenantID) == "" {
				return fmt.Errorf("--tenant-id is required")
			}
			options.Embedding.ProfileID = strings.TrimSpace(options.ProfileID)
			apiKey, err := options.Embedding.validateSemantic()
			if err != nil {
				return err
			}
			store, err := runtime.OpenStoreWithOptions(
				command.Context(), options.DatabaseURL, runtime.StoreOptions{EnforceTenantContext: true},
			)
			if err != nil {
				return fmt.Errorf("open retrieval snapshot rebuild store")
			}
			defer store.Close()
			if err := store.ValidateRuntimeRole(command.Context()); err != nil {
				return err
			}
			profile := options.Embedding.profile()
			embedder, err := memorybackend.NewOpenAIEmbedder(
				profile.BaseURL, apiKey, profile.Model, profile.Dimensions, &http.Client{Timeout: 60 * time.Second},
			)
			if err != nil {
				return fmt.Errorf("configure embedding provider")
			}
			worker, err := runtime.NewProjectionWorker(store, embedder, runtime.ProjectionWorkerOptions{
				TenantID:           options.TenantID,
				Profile:            profile,
				SnapshotPageSize:   options.SnapshotPageSize,
				EmbeddingBatchSize: options.EmbeddingBatchSize,
			})
			if err != nil {
				return err
			}
			result, rebuildErr := worker.RebuildCurrent(command.Context())
			if err := json.NewEncoder(command.OutOrStdout()).Encode(result); err != nil {
				return err
			}
			return rebuildErr
		},
	}
	command.Flags().StringVar(&options.DatabaseURL, "database-url", "", "restricted runtime PostgreSQL connection URL")
	command.Flags().StringVar(&options.TenantID, "tenant-id", "", "fixed tenant identifier")
	command.Flags().StringVar(&options.ProfileID, "profile-id", options.ProfileID, "retrieval profile identifier")
	command.Flags().StringVar(&options.Embedding.EmbeddingBaseURL, "embedding-base-url", options.Embedding.EmbeddingBaseURL, "direct embedding API base URL")
	command.Flags().StringVar(&options.Embedding.EmbeddingAPIKeyEnv, "embedding-api-key-env", options.Embedding.EmbeddingAPIKeyEnv, "environment variable containing the embedding API key")
	command.Flags().StringVar(&options.Embedding.EmbeddingModel, "embedding-model", options.Embedding.EmbeddingModel, "embedding model name")
	command.Flags().IntVar(&options.Embedding.EmbeddingDimensions, "embedding-dimensions", options.Embedding.EmbeddingDimensions, "embedding vector dimensions")
	command.Flags().IntVar(&options.SnapshotPageSize, "snapshot-page-size", options.SnapshotPageSize, "current-authority rows loaded per page")
	command.Flags().IntVar(&options.EmbeddingBatchSize, "embedding-batch-size", 1, "maximum texts sent per embedding request")
	return command
}

func newRetrievalStatusCommand() *cobra.Command {
	var databaseURL, tenantID, profileID string
	command := &cobra.Command{
		Use:   "retrieval-status",
		Short: "Report one tenant's retrieval projection status",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, args []string) error {
			if strings.TrimSpace(databaseURL) == "" {
				return fmt.Errorf("--database-url is required")
			}
			if strings.TrimSpace(tenantID) == "" {
				return fmt.Errorf("--tenant-id is required")
			}
			if !runtime.IsSupportedRetrievalProfileID(profileID) {
				return fmt.Errorf("unsupported retrieval profile")
			}
			store, err := runtime.OpenStoreWithOptions(command.Context(), databaseURL, runtime.StoreOptions{EnforceTenantContext: true})
			if err != nil {
				return fmt.Errorf("open retrieval status store")
			}
			defer store.Close()
			status, err := store.RetrievalProjectionStatus(command.Context(), tenantID, profileID)
			if err != nil {
				return err
			}
			return json.NewEncoder(command.OutOrStdout()).Encode(status)
		},
	}
	command.Flags().StringVar(&databaseURL, "database-url", "", "PostgreSQL connection URL")
	command.Flags().StringVar(&tenantID, "tenant-id", "", "tenant identifier")
	command.Flags().StringVar(&profileID, "profile-id", runtime.ProductionRetrievalProfileID, "retrieval profile identifier")
	return command
}

type retrievalPruneEventsCommandOptions struct {
	DatabaseURL      string
	TenantID         string
	OperationID      string
	Before           string
	RetainTailEvents int
}

func newRetrievalPruneEventsCommand() *cobra.Command {
	options := retrievalPruneEventsCommandOptions{}
	command := &cobra.Command{
		Use:   "retrieval-prune-events",
		Short: "Prune one tenant's acknowledged retrieval projection events",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, args []string) error {
			if strings.TrimSpace(options.DatabaseURL) == "" {
				return fmt.Errorf("--database-url is required")
			}
			if strings.TrimSpace(options.TenantID) == "" {
				return fmt.Errorf("--tenant-id is required")
			}
			if strings.TrimSpace(options.OperationID) == "" {
				return fmt.Errorf("--operation-id is required")
			}
			before, err := time.Parse(time.RFC3339, strings.TrimSpace(options.Before))
			if err != nil {
				return fmt.Errorf("--before must be RFC3339")
			}
			if options.RetainTailEvents < 0 {
				return fmt.Errorf("--retain-tail-events must be non-negative")
			}
			store, err := runtime.OpenStore(command.Context(), options.DatabaseURL)
			if err != nil {
				return fmt.Errorf("open projection prune store")
			}
			defer store.Close()
			version, err := store.SchemaVersion(command.Context())
			if err != nil {
				return fmt.Errorf("read projection prune schema version")
			}
			if version < 17 {
				return fmt.Errorf("projection pruning requires schema 17 or later")
			}
			if err := store.ValidateProjectionPruneOperatorRole(command.Context()); err != nil {
				return err
			}
			receipt, err := store.PruneProjectionEvents(command.Context(), options.TenantID, runtime.ProjectionPruneRequest{
				OperationID: strings.TrimSpace(options.OperationID), Cutoff: before,
				RetainTailEvents: options.RetainTailEvents,
			})
			if err != nil {
				return err
			}
			return json.NewEncoder(command.OutOrStdout()).Encode(receipt)
		},
	}
	command.Flags().StringVar(&options.DatabaseURL, "database-url", "", "operator PostgreSQL connection URL")
	command.Flags().StringVar(&options.TenantID, "tenant-id", "", "tenant identifier")
	command.Flags().StringVar(&options.OperationID, "operation-id", "", "stable prune operation identifier")
	command.Flags().StringVar(&options.Before, "before", "", "prune events older than this RFC3339 timestamp")
	command.Flags().IntVar(&options.RetainTailEvents, "retain-tail-events", 0, "newest tenant events to retain")
	return command
}

func newRetrievalRebuildCommand() *cobra.Command {
	var databaseURL, tenantID, profileID string
	command := &cobra.Command{
		Use:   "retrieval-rebuild",
		Short: "Reset one tenant's disposable vector projection",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, args []string) error {
			if strings.TrimSpace(databaseURL) == "" {
				return fmt.Errorf("--database-url is required")
			}
			if strings.TrimSpace(tenantID) == "" {
				return fmt.Errorf("--tenant-id is required")
			}
			if !runtime.IsSupportedRetrievalProfileID(profileID) {
				return fmt.Errorf("unsupported retrieval profile")
			}
			store, err := runtime.OpenStore(command.Context(), databaseURL)
			if err != nil {
				return fmt.Errorf("open retrieval rebuild store")
			}
			defer store.Close()
			if err := store.ResetVectorProjection(command.Context(), tenantID, profileID); err != nil {
				return err
			}
			status, err := store.RetrievalProjectionStatus(command.Context(), tenantID, profileID)
			if err != nil {
				return err
			}
			return json.NewEncoder(command.OutOrStdout()).Encode(status)
		},
	}
	command.Flags().StringVar(&databaseURL, "database-url", "", "admin PostgreSQL connection URL")
	command.Flags().StringVar(&tenantID, "tenant-id", "", "tenant identifier")
	command.Flags().StringVar(&profileID, "profile-id", runtime.ProductionRetrievalProfileID, "retrieval profile identifier")
	return command
}
