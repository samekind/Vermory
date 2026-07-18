package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"vermory/internal/app"
	"vermory/internal/brand"
	"vermory/internal/identitycli"
	"vermory/internal/mcpserver"
	"vermory/internal/memorybackend"
	"vermory/internal/operatorcli"
	"vermory/internal/reality"
	"vermory/internal/resolver"
	"vermory/internal/runtime"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"
)

func main() {
	if err := newRootCommand().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func newRootCommand() *cobra.Command {
	var artifactRoot string
	var databaseURL string
	var providerName string
	var providerBaseURL string
	var providerAPIKeyEnv string
	var providerModel string
	var providerModels []string
	var runID string
	var maxTokens int
	var probePrompt string
	var caseDir string
	var caseRoot string
	var caseLine string
	var casebookReportPath string
	var benchmarkMapPath string
	var backendName string
	var backendBaseURL string
	var backendAPIKeyEnv string
	var embeddingBaseURL string
	var embeddingAPIKeyEnv string
	var embeddingModel string
	var embeddingDimensions int
	var backendCasebookPath string
	var loadRecords int
	var loadQueries int
	var loadConcurrency int
	var loadScopeSuffix string
	var mcpDatabaseURL string
	var mcpTenantID string
	var mcpWorkspaceAttachment string
	mcpRetrieval := defaultRetrievalRuntimeOptions()

	rootCmd := &cobra.Command{
		Use:           brand.Slug,
		Short:         brand.Name + " - " + brand.Tagline,
		Version:       brand.Version,
		SilenceErrors: true,
		SilenceUsage:  true,
	}
	rootCmd.AddCommand(&cobra.Command{
		Use:   "version",
		Short: "Print release build metadata",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return json.NewEncoder(cmd.OutOrStdout()).Encode(brand.Info())
		},
	})

	runSelfCaseCmd := &cobra.Command{
		Use:   "run-self-case",
		Short: "Run the legacy ContextMesh Bluebridge self-case vertical slice",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.RunSelfCase(cmd.Context(), app.SelfCaseOptions{
				DatabaseURL:  databaseURL,
				ArtifactRoot: artifactRoot,
			})
		},
	}
	runSelfCaseCmd.Flags().StringVar(&databaseURL, "database-url", "", "PostgreSQL connection URL")
	runSelfCaseCmd.Flags().StringVar(&artifactRoot, "artifact-root", "./artifacts", "artifact output root")
	rootCmd.AddCommand(runSelfCaseCmd)
	rootCmd.AddCommand(operatorcli.NewWorkspaceCommand())
	rootCmd.AddCommand(operatorcli.NewMemoryCommand())
	rootCmd.AddCommand(operatorcli.NewDefaultsCommand())
	rootCmd.AddCommand(operatorcli.NewBridgeCommand())
	rootCmd.AddCommand(identitycli.NewIdentityCommand())
	rootCmd.AddCommand(identitycli.NewDatabaseCommand())
	rootCmd.AddCommand(newWebChatCommand())
	rootCmd.AddCommand(newServeCommand())
	rootCmd.AddCommand(newBenchmarkLongMemEvalCommand())
	rootCmd.AddCommand(newBenchmarkLongMemEvalRetrievalCommand())
	rootCmd.AddCommand(newBenchmarkLongMemEvalQACommand())
	rootCmd.AddCommand(newRetrievalAblationCommand())
	rootCmd.AddCommand(newRetrievalProfileComparisonCommand())
	rootCmd.AddCommand(newRetrievalWorkerCommand())
	rootCmd.AddCommand(newConversationFormationWorkerCommand())
	rootCmd.AddCommand(newRetrievalStatusCommand())
	rootCmd.AddCommand(newRetrievalRebuildCommand())
	rootCmd.AddCommand(newRetrievalSnapshotRebuildCommand())
	rootCmd.AddCommand(newRetrievalPruneEventsCommand())

	mcpStdioCmd := &cobra.Command{
		Use:   "mcp-stdio",
		Short: "Run the local workspace continuity MCP server over stdio",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(mcpDatabaseURL) == "" {
				return fmt.Errorf("--database-url is required")
			}
			if strings.TrimSpace(mcpTenantID) == "" {
				return fmt.Errorf("--tenant-id is required")
			}
			attachment, err := resolver.DecodeWorkspaceAttachment(mcpWorkspaceAttachment)
			if err != nil {
				return err
			}
			store, err := runtime.OpenStore(cmd.Context(), mcpDatabaseURL)
			if err != nil {
				return fmt.Errorf("open MCP runtime store")
			}
			defer store.Close()
			if err := store.Migrate(cmd.Context()); err != nil {
				return fmt.Errorf("migrate MCP runtime store")
			}
			retriever, err := buildRuntimeRetriever(store, mcpRetrieval)
			if err != nil {
				return err
			}
			service := runtime.NewService(store, mcpTenantID)
			if retriever != nil {
				service = runtime.NewServiceWithRetriever(store, mcpTenantID, retriever)
			}
			handler := mcpserver.NewWithAttachment(service, mcpTenantID, attachment)
			return mcpserver.NewServer(handler).Run(cmd.Context(), &mcp.StdioTransport{})
		},
	}
	mcpStdioCmd.Flags().StringVar(&mcpDatabaseURL, "database-url", "", "PostgreSQL connection URL")
	mcpStdioCmd.Flags().StringVar(&mcpTenantID, "tenant-id", "", "server-owned tenant identifier")
	mcpStdioCmd.Flags().StringVar(&mcpWorkspaceAttachment, "workspace-attachment", "", "trusted workstation-generated workspace attachment")
	_ = mcpStdioCmd.MarkFlagRequired("workspace-attachment")
	addSharedRetrievalFlags(mcpStdioCmd, &mcpRetrieval)
	rootCmd.AddCommand(mcpStdioCmd)
	rootCmd.AddCommand(newWorkspaceAttachmentCommand())

	evalSelfCaseCmd := &cobra.Command{
		Use:   "eval-self-case",
		Short: "Run direct provider baselines against the legacy ContextMesh self-case",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			report, err := app.EvalSelfCase(cmd.Context(), app.EvalSelfCaseOptions{
				DatabaseURL:  databaseURL,
				ArtifactRoot: artifactRoot,
				Provider:     providerName,
				BaseURL:      providerBaseURL,
				APIKeyEnv:    providerAPIKeyEnv,
				Model:        providerModel,
				RunID:        runID,
				MaxTokens:    maxTokens,
			})
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "provider_mode=%s provider=%s model=%s report=%s\n", report.ProviderMode, report.ProviderName, report.Model, report.ReportURI)
			return nil
		},
	}
	evalSelfCaseCmd.Flags().StringVar(&databaseURL, "database-url", "", "optional PostgreSQL connection URL for migration preflight")
	evalSelfCaseCmd.Flags().StringVar(&artifactRoot, "artifact-root", "./artifacts", "artifact output root")
	evalSelfCaseCmd.Flags().StringVar(&providerName, "provider", "mock", "provider: mock, grok-cli, openai-compatible, siliconflow, or duojie")
	evalSelfCaseCmd.Flags().StringVar(&providerBaseURL, "base-url", "", "direct provider base URL")
	evalSelfCaseCmd.Flags().StringVar(&providerAPIKeyEnv, "api-key-env", "", "environment variable containing provider API key")
	evalSelfCaseCmd.Flags().StringVar(&providerModel, "model", "", "provider model name")
	evalSelfCaseCmd.Flags().StringVar(&runID, "run-id", "", "stable platform run id")
	evalSelfCaseCmd.Flags().IntVar(&maxTokens, "max-tokens", 1024, "maximum output tokens")
	rootCmd.AddCommand(evalSelfCaseCmd)

	probeProviderCmd := &cobra.Command{
		Use:   "probe-provider",
		Short: "Probe direct provider/model connectivity and capture artifacts",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			report, err := app.ProbeProvider(cmd.Context(), app.ProbeProviderOptions{
				ArtifactRoot: artifactRoot,
				Provider:     providerName,
				BaseURL:      providerBaseURL,
				APIKeyEnv:    providerAPIKeyEnv,
				RunID:        runID,
				Prompt:       probePrompt,
				Models:       providerModels,
				MaxTokens:    maxTokens,
			})
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "provider_mode=%s provider=%s report=%s\n", report.ProviderMode, report.ProviderName, report.ReportURI)
			for _, result := range report.Results {
				fmt.Fprintf(cmd.OutOrStdout(), "model=%s status=%s preview=%s\n", result.Model, result.Status, result.OutputPreview)
				if result.Error != "" {
					fmt.Fprintf(cmd.OutOrStdout(), "model=%s error=%s\n", result.Model, result.Error)
				}
			}
			return nil
		},
	}
	probeProviderCmd.Flags().StringVar(&artifactRoot, "artifact-root", "./artifacts", "artifact output root")
	probeProviderCmd.Flags().StringVar(&providerName, "provider", "mock", "provider: mock, grok-cli, openai-compatible, siliconflow, or duojie")
	probeProviderCmd.Flags().StringVar(&providerBaseURL, "base-url", "", "direct provider base URL")
	probeProviderCmd.Flags().StringVar(&providerAPIKeyEnv, "api-key-env", "", "environment variable containing provider API key")
	probeProviderCmd.Flags().StringSliceVar(&providerModels, "models", nil, "provider model names to probe")
	probeProviderCmd.Flags().StringVar(&probePrompt, "prompt", "Only reply OK.", "probe prompt")
	probeProviderCmd.Flags().StringVar(&runID, "run-id", "", "stable probe run id")
	probeProviderCmd.Flags().IntVar(&maxTokens, "max-tokens", 64, "maximum output tokens")
	rootCmd.AddCommand(probeProviderCmd)

	evalMatrixCmd := &cobra.Command{
		Use:   "eval-matrix",
		Short: "Run a model-by-task evaluation matrix for the legacy ContextMesh self-case",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			report, err := app.EvalMatrix(cmd.Context(), app.EvalMatrixOptions{
				ArtifactRoot: artifactRoot,
				Provider:     providerName,
				BaseURL:      providerBaseURL,
				APIKeyEnv:    providerAPIKeyEnv,
				RunID:        runID,
				Models:       providerModels,
				MaxTokens:    maxTokens,
			})
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "provider_mode=%s provider=%s report=%s\n", report.ProviderMode, report.ProviderName, report.ReportURI)
			for _, model := range report.Models {
				fmt.Fprintf(cmd.OutOrStdout(), "model=%s task_count=%d\n", model.Model, len(model.Tasks))
			}
			return nil
		},
	}
	evalMatrixCmd.Flags().StringVar(&artifactRoot, "artifact-root", "./artifacts", "artifact output root")
	evalMatrixCmd.Flags().StringVar(&providerName, "provider", "mock", "provider: mock, grok-cli, openai-compatible, siliconflow, or duojie")
	evalMatrixCmd.Flags().StringVar(&providerBaseURL, "base-url", "", "direct provider base URL")
	evalMatrixCmd.Flags().StringVar(&providerAPIKeyEnv, "api-key-env", "", "environment variable containing provider API key")
	evalMatrixCmd.Flags().StringSliceVar(&providerModels, "models", nil, "provider model names to evaluate")
	evalMatrixCmd.Flags().StringVar(&runID, "run-id", "", "stable matrix run id")
	evalMatrixCmd.Flags().IntVar(&maxTokens, "max-tokens", 256, "maximum output tokens")
	rootCmd.AddCommand(evalMatrixCmd)

	evalCasebookCmd := &cobra.Command{
		Use:   "eval-casebook",
		Short: "Run one casebook case on a specific continuity line",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			report, err := app.EvalCasebook(cmd.Context(), app.EvalCasebookOptions{
				CaseDir:      caseDir,
				Line:         caseLine,
				ArtifactRoot: artifactRoot,
				Provider:     providerName,
				BaseURL:      providerBaseURL,
				APIKeyEnv:    providerAPIKeyEnv,
				Model:        providerModel,
				RunID:        runID,
				MaxTokens:    maxTokens,
			})
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "line=%s case=%s task=%s report=%s\n", report.Line, report.CaseID, report.TaskID, report.Artifacts.MDURI)
			return nil
		},
	}
	evalCasebookCmd.Flags().StringVar(&artifactRoot, "artifact-root", "./artifacts", "artifact output root")
	evalCasebookCmd.Flags().StringVar(&providerName, "provider", "mock", "provider: mock, grok-cli, openai-compatible, siliconflow, or duojie")
	evalCasebookCmd.Flags().StringVar(&providerBaseURL, "base-url", "", "direct provider base URL")
	evalCasebookCmd.Flags().StringVar(&providerAPIKeyEnv, "api-key-env", "", "environment variable containing provider API key")
	evalCasebookCmd.Flags().StringVar(&providerModel, "model", "", "provider model name")
	evalCasebookCmd.Flags().StringVar(&runID, "run-id", "", "stable casebook run id")
	evalCasebookCmd.Flags().IntVar(&maxTokens, "max-tokens", 1024, "maximum output tokens")
	evalCasebookCmd.Flags().StringVar(&caseDir, "case-dir", "", "casebook case directory")
	evalCasebookCmd.Flags().StringVar(&caseLine, "line", "", "continuity line: workspace, conversation, or bridge")
	rootCmd.AddCommand(evalCasebookCmd)

	evalCasebookSuiteCmd := &cobra.Command{
		Use:   "eval-casebook-suite",
		Short: "Run all casebook cases under a case root",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			report, err := app.EvalCasebookSuite(cmd.Context(), app.EvalCasebookSuiteOptions{
				CaseRoot:     caseRoot,
				ArtifactRoot: artifactRoot,
				Provider:     providerName,
				BaseURL:      providerBaseURL,
				APIKeyEnv:    providerAPIKeyEnv,
				Model:        providerModel,
				RunID:        runID,
				MaxTokens:    maxTokens,
			})
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "cases=%d executed=%d failed=%d report=%s\n", report.Total, report.Executed, report.Failed, report.Artifacts.MDURI)
			return nil
		},
	}
	evalCasebookSuiteCmd.Flags().StringVar(&artifactRoot, "artifact-root", "./artifacts", "artifact output root")
	evalCasebookSuiteCmd.Flags().StringVar(&providerName, "provider", "mock", "provider: mock, grok-cli, openai-compatible, siliconflow, or duojie")
	evalCasebookSuiteCmd.Flags().StringVar(&providerBaseURL, "base-url", "", "direct provider base URL")
	evalCasebookSuiteCmd.Flags().StringVar(&providerAPIKeyEnv, "api-key-env", "", "environment variable containing provider API key")
	evalCasebookSuiteCmd.Flags().StringVar(&providerModel, "model", "", "provider model name")
	evalCasebookSuiteCmd.Flags().StringVar(&runID, "run-id", "", "stable casebook suite run id")
	evalCasebookSuiteCmd.Flags().IntVar(&maxTokens, "max-tokens", 1024, "maximum output tokens")
	evalCasebookSuiteCmd.Flags().StringVar(&caseRoot, "case-root", "casebook/cases", "casebook cases root directory")
	rootCmd.AddCommand(evalCasebookSuiteCmd)

	acceptanceReportCmd := &cobra.Command{
		Use:   "acceptance-report",
		Short: "Write minimal acceptance JSON and Markdown artifacts",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			var casebookRun app.EvalCasebookReport
			var err error
			if casebookReportPath != "" {
				casebookRun, err = app.LoadCasebookReport(casebookReportPath)
			} else {
				casebookRun, err = app.EvalCasebook(cmd.Context(), app.EvalCasebookOptions{
					CaseDir:      caseDir,
					Line:         caseLine,
					ArtifactRoot: artifactRoot,
					Provider:     providerName,
					BaseURL:      providerBaseURL,
					APIKeyEnv:    providerAPIKeyEnv,
					Model:        providerModel,
					RunID:        runID,
					MaxTokens:    maxTokens,
				})
			}
			if err != nil {
				return err
			}
			report, err := app.AcceptanceReport(cmd.Context(), app.AcceptanceReportOptions{
				ArtifactRoot: artifactRoot,
				RunID:        runID,
				CasebookRun:  casebookRun,
			})
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "line=%s source_run=%s report=%s\n", report.Line, report.SourceRunID, report.Artifacts.MDURI)
			return nil
		},
	}
	acceptanceReportCmd.Flags().StringVar(&artifactRoot, "artifact-root", "./artifacts", "artifact output root")
	acceptanceReportCmd.Flags().StringVar(&providerName, "provider", "mock", "provider: mock, grok-cli, openai-compatible, siliconflow, or duojie")
	acceptanceReportCmd.Flags().StringVar(&providerBaseURL, "base-url", "", "direct provider base URL")
	acceptanceReportCmd.Flags().StringVar(&providerAPIKeyEnv, "api-key-env", "", "environment variable containing provider API key")
	acceptanceReportCmd.Flags().StringVar(&providerModel, "model", "", "provider model name")
	acceptanceReportCmd.Flags().StringVar(&runID, "run-id", "", "stable acceptance run id")
	acceptanceReportCmd.Flags().IntVar(&maxTokens, "max-tokens", 1024, "maximum output tokens")
	acceptanceReportCmd.Flags().StringVar(&caseDir, "case-dir", "", "casebook case directory")
	acceptanceReportCmd.Flags().StringVar(&caseLine, "line", "", "continuity line: workspace, conversation, or bridge")
	acceptanceReportCmd.Flags().StringVar(&casebookReportPath, "casebook-report", "", "existing eval-casebook report.json path")
	rootCmd.AddCommand(acceptanceReportCmd)

	benchmarkCoverageCmd := &cobra.Command{
		Use:   "benchmark-coverage",
		Short: "Write public benchmark mapping coverage artifacts",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			report, err := app.BenchmarkCoverage(cmd.Context(), app.BenchmarkCoverageOptions{
				MapPath:      benchmarkMapPath,
				ArtifactRoot: artifactRoot,
				RunID:        runID,
			})
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "benchmarks=%d executable=%d report=%s\n", report.Total, report.ExecutableCount, report.Artifacts.MDURI)
			return nil
		},
	}
	benchmarkCoverageCmd.Flags().StringVar(&artifactRoot, "artifact-root", "./artifacts", "artifact output root")
	benchmarkCoverageCmd.Flags().StringVar(&runID, "run-id", "", "stable benchmark coverage run id")
	benchmarkCoverageCmd.Flags().StringVar(&benchmarkMapPath, "map-path", "casebook/benchmarks/public-benchmark-map.json", "public benchmark map JSON path")
	rootCmd.AddCommand(benchmarkCoverageCmd)

	internalReadyCmd := &cobra.Command{
		Use:   "internal-ready",
		Short: "Run the Internal Ready acceptance chain",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			report, err := app.InternalReady(cmd.Context(), app.InternalReadyOptions{
				CaseRoot:     caseRoot,
				BenchmarkMap: benchmarkMapPath,
				ArtifactRoot: artifactRoot,
				Provider:     providerName,
				BaseURL:      providerBaseURL,
				APIKeyEnv:    providerAPIKeyEnv,
				Model:        providerModel,
				RunID:        runID,
				MaxTokens:    maxTokens,
			})
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "pass=%t cases=%d executed=%d benchmarks=%d executable_benchmarks=%d report=%s\n",
				report.Pass,
				report.Casebook.Total,
				report.Casebook.Executed,
				report.Benchmark.Total,
				report.Benchmark.ExecutableCount,
				report.Artifacts.MDURI,
			)
			return nil
		},
	}
	internalReadyCmd.Flags().StringVar(&artifactRoot, "artifact-root", "./artifacts", "artifact output root")
	internalReadyCmd.Flags().StringVar(&providerName, "provider", "mock", "provider: mock, grok-cli, openai-compatible, siliconflow, or duojie")
	internalReadyCmd.Flags().StringVar(&providerBaseURL, "base-url", "", "direct provider base URL")
	internalReadyCmd.Flags().StringVar(&providerAPIKeyEnv, "api-key-env", "", "environment variable containing provider API key")
	internalReadyCmd.Flags().StringVar(&providerModel, "model", "", "provider model name")
	internalReadyCmd.Flags().StringVar(&runID, "run-id", "", "stable internal-ready run id")
	internalReadyCmd.Flags().IntVar(&maxTokens, "max-tokens", 1024, "maximum output tokens")
	internalReadyCmd.Flags().StringVar(&caseRoot, "case-root", "casebook/cases", "casebook cases root directory")
	internalReadyCmd.Flags().StringVar(&benchmarkMapPath, "benchmark-map", "casebook/benchmarks/public-benchmark-map.json", "public benchmark map JSON path")
	rootCmd.AddCommand(internalReadyCmd)

	backendLifecycleCmd := &cobra.Command{
		Use:   "backend-lifecycle",
		Short: "Run memory backend lifecycle hard gates and write evidence artifacts",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			backend, cleanup, err := memorybackend.OpenBackend(cmd.Context(), memorybackend.OpenConfig{
				Name:             backendName,
				BaseURL:          backendBaseURL,
				APIKey:           os.Getenv(backendAPIKeyEnv),
				DatabaseURL:      databaseURL,
				EmbeddingBaseURL: embeddingBaseURL,
				EmbeddingAPIKey:  os.Getenv(embeddingAPIKeyEnv),
				EmbeddingModel:   embeddingModel,
				Dimensions:       embeddingDimensions,
			})
			if err != nil {
				return err
			}
			defer cleanup()
			report := memorybackend.RunLifecycleSuite(cmd.Context(), backend)
			artifacts, err := memorybackend.WriteLifecycleArtifacts(artifactRoot, runID, report)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "backend=%s pass=%t duration=%s json=%s markdown=%s\n", report.Backend, report.Pass, report.Duration, artifacts.JSONPath, artifacts.MarkdownPath)
			if !report.Pass {
				return fmt.Errorf("backend %s failed lifecycle hard gates", report.Backend)
			}
			return nil
		},
	}
	backendLifecycleCmd.Flags().StringVar(&backendName, "backend", "", "backend: native, mem0, supermemory, or memos")
	backendLifecycleCmd.Flags().StringVar(&backendBaseURL, "base-url", "", "memory backend base URL")
	backendLifecycleCmd.Flags().StringVar(&backendAPIKeyEnv, "api-key-env", "", "environment variable containing the memory backend API key")
	backendLifecycleCmd.Flags().StringVar(&databaseURL, "database-url", "", "PostgreSQL URL for the native backend")
	backendLifecycleCmd.Flags().StringVar(&embeddingBaseURL, "embedding-base-url", "", "OpenAI-compatible embedding API base URL")
	backendLifecycleCmd.Flags().StringVar(&embeddingAPIKeyEnv, "embedding-api-key-env", "", "environment variable containing the embedding API key")
	backendLifecycleCmd.Flags().StringVar(&embeddingModel, "embedding-model", "bge-m3", "embedding model for the native backend")
	backendLifecycleCmd.Flags().IntVar(&embeddingDimensions, "embedding-dimensions", 1024, "embedding dimensions for the native backend")
	backendLifecycleCmd.Flags().StringVar(&artifactRoot, "artifact-root", "./artifacts", "artifact output root")
	backendLifecycleCmd.Flags().StringVar(&runID, "run-id", "", "stable backend bake-off run id")
	rootCmd.AddCommand(backendLifecycleCmd)

	backendQualityCmd := &cobra.Command{
		Use:   "backend-quality",
		Short: "Run B01-B10 memory backend scenarios and write evidence artifacts",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			suite, err := memorybackend.LoadQualitySuite(backendCasebookPath)
			if err != nil {
				return err
			}
			backend, cleanup, err := memorybackend.OpenBackend(cmd.Context(), memorybackend.OpenConfig{
				Name:             backendName,
				BaseURL:          backendBaseURL,
				APIKey:           os.Getenv(backendAPIKeyEnv),
				DatabaseURL:      databaseURL,
				EmbeddingBaseURL: embeddingBaseURL,
				EmbeddingAPIKey:  os.Getenv(embeddingAPIKeyEnv),
				EmbeddingModel:   embeddingModel,
				Dimensions:       embeddingDimensions,
			})
			if err != nil {
				return err
			}
			defer cleanup()
			report := memorybackend.RunQualitySuite(cmd.Context(), backend, suite)
			artifacts, err := memorybackend.WriteQualityArtifacts(artifactRoot, runID, report)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "backend=%s pass=%t assertions=%d/%d leakage=%d p50=%s p95=%s json=%s markdown=%s\n",
				report.Backend, report.Pass, report.Metrics.PassedAssertions, report.Metrics.Assertions,
				report.Metrics.ForbiddenLeakage, report.Metrics.SearchP50, report.Metrics.SearchP95,
				artifacts.JSONPath, artifacts.MarkdownPath,
			)
			if !report.Pass {
				return fmt.Errorf("backend %s failed B01-B10 quality scenarios", report.Backend)
			}
			return nil
		},
	}
	backendQualityCmd.Flags().StringVar(&backendName, "backend", "", "backend: native, mem0, supermemory, or memos")
	backendQualityCmd.Flags().StringVar(&backendBaseURL, "base-url", "", "memory backend base URL")
	backendQualityCmd.Flags().StringVar(&backendAPIKeyEnv, "api-key-env", "", "environment variable containing the memory backend API key")
	backendQualityCmd.Flags().StringVar(&databaseURL, "database-url", "", "PostgreSQL URL for the native backend")
	backendQualityCmd.Flags().StringVar(&embeddingBaseURL, "embedding-base-url", "", "OpenAI-compatible embedding API base URL")
	backendQualityCmd.Flags().StringVar(&embeddingAPIKeyEnv, "embedding-api-key-env", "", "environment variable containing the embedding API key")
	backendQualityCmd.Flags().StringVar(&embeddingModel, "embedding-model", "bge-m3", "embedding model for the native backend")
	backendQualityCmd.Flags().IntVar(&embeddingDimensions, "embedding-dimensions", 1024, "embedding dimensions for the native backend")
	backendQualityCmd.Flags().StringVar(&backendCasebookPath, "casebook", "backend-casebook/scenarios.json", "backend scenario casebook JSON")
	backendQualityCmd.Flags().StringVar(&artifactRoot, "artifact-root", "./artifacts", "artifact output root")
	backendQualityCmd.Flags().StringVar(&runID, "run-id", "", "stable backend bake-off run id")
	rootCmd.AddCommand(backendQualityCmd)

	backendLoadCmd := &cobra.Command{
		Use:   "backend-load",
		Short: "Measure bulk ingest and exact technical identifier recall",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			backend, cleanup, err := memorybackend.OpenBackend(cmd.Context(), memorybackend.OpenConfig{
				Name:             backendName,
				BaseURL:          backendBaseURL,
				APIKey:           os.Getenv(backendAPIKeyEnv),
				DatabaseURL:      databaseURL,
				EmbeddingBaseURL: embeddingBaseURL,
				EmbeddingAPIKey:  os.Getenv(embeddingAPIKeyEnv),
				EmbeddingModel:   embeddingModel,
				Dimensions:       embeddingDimensions,
			})
			if err != nil {
				return err
			}
			defer cleanup()
			scopeSuffix := loadScopeSuffix
			if scopeSuffix == "" {
				scopeSuffix = runID
			}
			report := memorybackend.RunLoadSuite(cmd.Context(), backend, memorybackend.LoadOptions{
				Records: loadRecords, Queries: loadQueries, Concurrency: loadConcurrency, ScopeSuffix: scopeSuffix,
			})
			artifacts, err := memorybackend.WriteLoadArtifacts(artifactRoot, runID, report)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "backend=%s pass=%t writes=%d/%d ingest=%s recall=%.4f p50=%s p95=%s json=%s markdown=%s\n",
				report.Backend, report.Pass, report.WritesSucceeded, report.RecordsRequested,
				report.IngestDuration, report.Recall, report.SearchP50, report.SearchP95,
				artifacts.JSONPath, artifacts.MarkdownPath,
			)
			if !report.Pass {
				return fmt.Errorf("backend %s failed load or identifier-recall checks", report.Backend)
			}
			return nil
		},
	}
	backendLoadCmd.Flags().StringVar(&backendName, "backend", "", "backend: native, mem0, supermemory, or memos")
	backendLoadCmd.Flags().StringVar(&backendBaseURL, "base-url", "", "memory backend base URL")
	backendLoadCmd.Flags().StringVar(&backendAPIKeyEnv, "api-key-env", "", "environment variable containing the memory backend API key")
	backendLoadCmd.Flags().StringVar(&databaseURL, "database-url", "", "PostgreSQL URL for the native backend")
	backendLoadCmd.Flags().StringVar(&embeddingBaseURL, "embedding-base-url", "", "OpenAI-compatible embedding API base URL")
	backendLoadCmd.Flags().StringVar(&embeddingAPIKeyEnv, "embedding-api-key-env", "", "environment variable containing the embedding API key")
	backendLoadCmd.Flags().StringVar(&embeddingModel, "embedding-model", "bge-m3", "embedding model for the native backend")
	backendLoadCmd.Flags().IntVar(&embeddingDimensions, "embedding-dimensions", 1024, "embedding dimensions for the native backend")
	backendLoadCmd.Flags().IntVar(&loadRecords, "records", 200, "number of similar technical records to ingest")
	backendLoadCmd.Flags().IntVar(&loadQueries, "queries", 20, "number of exact identifier queries")
	backendLoadCmd.Flags().IntVar(&loadConcurrency, "concurrency", 4, "concurrent write workers")
	backendLoadCmd.Flags().StringVar(&loadScopeSuffix, "scope-suffix", "", "existing load scope suffix to reset; defaults to run-id")
	backendLoadCmd.Flags().StringVar(&artifactRoot, "artifact-root", "./artifacts", "artifact output root")
	backendLoadCmd.Flags().StringVar(&runID, "run-id", "", "stable backend load run id")
	rootCmd.AddCommand(backendLoadCmd)

	var realityCaseDir string
	realityFreezeCmd := &cobra.Command{
		Use:   "reality-freeze",
		Short: "Freeze one reality case into a deterministic fixture lock",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			report, err := reality.FreezeCase(realityCaseDir)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "case=%s lock_sha256=%s lock=%s\n", report.CaseID, report.LockSHA256, report.LockPath)
			return nil
		},
	}
	realityFreezeCmd.Flags().StringVar(&realityCaseDir, "case-dir", "", "reality case directory")
	_ = realityFreezeCmd.MarkFlagRequired("case-dir")
	rootCmd.AddCommand(realityFreezeCmd)

	var realityCaseRoot string
	var realityArtifactRoot string
	var realityRunID string
	realityValidateCmd := &cobra.Command{
		Use:   "reality-validate",
		Short: "Validate frozen reality cases and write evidence reports",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			report := reality.ValidateRoot(realityCaseRoot)
			report.RunID = realityRunID
			artifacts, err := reality.WriteValidationArtifacts(realityArtifactRoot, report)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "pass=%t cases=%d json=%s markdown=%s\n", report.Pass, report.Cases, artifacts.JSONPath, artifacts.MarkdownPath)
			if !report.Pass {
				return fmt.Errorf("reality validation failed for %s", realityCaseRoot)
			}
			return nil
		},
	}
	realityValidateCmd.Flags().StringVar(&realityCaseRoot, "case-root", "reality/cases", "reality cases root directory")
	realityValidateCmd.Flags().StringVar(&realityArtifactRoot, "artifact-root", "./artifacts", "artifact output root")
	realityValidateCmd.Flags().StringVar(&realityRunID, "run-id", "experiment-0-public", "stable reality validation run id")
	rootCmd.AddCommand(realityValidateCmd)

	var attestationInput string
	var attestationPublicKey string
	attestationVerifyCmd := &cobra.Command{
		Use:   "reality-attestation-verify",
		Short: "Verify an attestation signed by an external sealed evaluator",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			data, err := os.ReadFile(attestationInput)
			if err != nil {
				return err
			}
			publicKey, err := base64.StdEncoding.DecodeString(attestationPublicKey)
			if err != nil {
				return fmt.Errorf("decode public key: %w", err)
			}
			attestation, err := reality.VerifyAttestation(data, publicKey)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "evaluator=%s suite=%s implementation=%s hard_gates_pass=%t\n",
				attestation.EvaluatorID, attestation.SuiteVersion, attestation.ImplementationDigest, attestation.HardGatesPass)
			keys := make([]string, 0, len(attestation.Counts))
			for key := range attestation.Counts {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			for _, key := range keys {
				fmt.Fprintf(cmd.OutOrStdout(), "count.%s=%d\n", key, attestation.Counts[key])
			}
			return nil
		},
	}
	attestationVerifyCmd.Flags().StringVar(&attestationInput, "input", "", "external attestation JSON path")
	attestationVerifyCmd.Flags().StringVar(&attestationPublicKey, "public-key", "", "base64-encoded Ed25519 public key")
	_ = attestationVerifyCmd.MarkFlagRequired("input")
	_ = attestationVerifyCmd.MarkFlagRequired("public-key")
	rootCmd.AddCommand(attestationVerifyCmd)

	var experimentCaseRoot string
	var experimentArtifactRoot string
	var experimentRunID string
	var experimentAttestationInput string
	var experimentAttestationPublicKey string
	experiment0Cmd := &cobra.Command{
		Use:   "experiment-0",
		Short: "Generate the reality-first Experiment 0 evidence readout",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			var sealedAttestation *reality.Attestation
			if experimentAttestationInput != "" || experimentAttestationPublicKey != "" {
				if experimentAttestationInput == "" || experimentAttestationPublicKey == "" {
					return fmt.Errorf("sealed-attestation and sealed-public-key must be provided together")
				}
				data, err := os.ReadFile(experimentAttestationInput)
				if err != nil {
					return err
				}
				publicKey, err := base64.StdEncoding.DecodeString(experimentAttestationPublicKey)
				if err != nil {
					return fmt.Errorf("decode sealed public key: %w", err)
				}
				verified, err := reality.VerifyAttestation(data, publicKey)
				if err != nil {
					return err
				}
				sealedAttestation = &verified
			}
			report := reality.BuildExperiment0(reality.Experiment0Options{
				RunID:             experimentRunID,
				CaseRoot:          experimentCaseRoot,
				SealedAttestation: sealedAttestation,
			})
			artifacts, err := reality.WriteExperiment0Artifacts(experimentArtifactRoot, report)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "pass=%t cases=%d sealed=%s json=%s markdown=%s\n",
				report.Pass, report.PublicValidation.Cases, report.SealedStatus, artifacts.JSONPath, artifacts.MarkdownPath)
			if !report.Pass {
				return fmt.Errorf("experiment 0 failed")
			}
			return nil
		},
	}
	experiment0Cmd.Flags().StringVar(&experimentCaseRoot, "case-root", "reality/cases", "frozen reality cases root")
	experiment0Cmd.Flags().StringVar(&experimentArtifactRoot, "artifact-root", "./artifacts", "artifact output root")
	experiment0Cmd.Flags().StringVar(&experimentRunID, "run-id", "experiment-0-v1", "stable Experiment 0 run id")
	experiment0Cmd.Flags().StringVar(&experimentAttestationInput, "sealed-attestation", "", "optional externally signed sealed attestation JSON")
	experiment0Cmd.Flags().StringVar(&experimentAttestationPublicKey, "sealed-public-key", "", "base64 Ed25519 key for the optional sealed attestation")
	rootCmd.AddCommand(experiment0Cmd)

	return rootCmd
}
