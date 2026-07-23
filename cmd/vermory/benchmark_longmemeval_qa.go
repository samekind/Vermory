package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"vermory/internal/app"
	"vermory/internal/benchmark"
	"vermory/internal/provider"

	"github.com/spf13/cobra"
)

type benchmarkLongMemEvalQACommandOptions struct {
	SourceDatasetPath      string
	RetrievalResultsPath   string
	QualificationPath      string
	ExecutionPath          string
	ArtifactRoot           string
	RunID                  string
	ImplementationRevision string
	Phase                  string
	ReaderCommand          string
	ReaderBaseURL          string
	ReaderAPIKeyEnv        string
	JudgeCommand           string
	JudgeBaseURL           string
	JudgeAPIKeyEnv         string
	Resume                 bool
}

func newBenchmarkLongMemEvalQACommand() *cobra.Command {
	return newBenchmarkLongMemEvalQACommandWithProviders(nil, nil)
}

func newBenchmarkLongMemEvalQACommandWithProviders(readerOverride, judgeOverride provider.Provider) *cobra.Command {
	options := benchmarkLongMemEvalQACommandOptions{}
	command := &cobra.Command{
		Use:           "benchmark-longmemeval-qa",
		Short:         "Run full LongMemEval-S reader QA and custom judging",
		Args:          cobra.NoArgs,
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(command *cobra.Command, args []string) error {
			phase := strings.ToLower(strings.TrimSpace(options.Phase))
			switch phase {
			case "reader", "judge", "all", "finalize":
			default:
				return fmt.Errorf("unsupported LongMemEval QA phase %q", options.Phase)
			}
			qualification, err := benchmark.LoadQualification(options.QualificationPath)
			if err != nil {
				return err
			}
			execution, err := benchmark.LoadExecution(options.ExecutionPath)
			if err != nil {
				return err
			}
			if err := benchmark.ValidateLongMemEvalQAExecution(qualification, execution); err != nil {
				return err
			}
			appOptions := app.LongMemEvalQAOptions{
				Qualification:          qualification,
				Execution:              execution,
				SourceDatasetPath:      options.SourceDatasetPath,
				RetrievalResultsPath:   options.RetrievalResultsPath,
				ArtifactRoot:           options.ArtifactRoot,
				RunID:                  options.RunID,
				ImplementationRevision: options.ImplementationRevision,
				Resume:                 options.Resume,
			}
			if readerOverride != nil || judgeOverride != nil {
				appOptions.RetrySleeper = func(context.Context, time.Duration) error { return nil }
			}

			var readerSummary app.LongMemEvalQAReaderSummary
			var judgeSummary app.LongMemEvalQAJudgeSummary
			var report app.LongMemEvalQAReport
			if phase == "reader" || phase == "all" {
				reader, err := buildBenchmarkLongMemEvalQAProvider(*execution.Reader, options.ReaderCommand, options.ReaderBaseURL, options.ReaderAPIKeyEnv, readerOverride)
				if err != nil {
					return err
				}
				appOptions.ReaderProvider = reader
				readerSummary, err = app.RunLongMemEvalQAReader(command.Context(), appOptions)
				if err != nil {
					return err
				}
			}
			if phase == "judge" || phase == "all" {
				judge, err := buildBenchmarkLongMemEvalQAProvider(*execution.Judge, options.JudgeCommand, options.JudgeBaseURL, options.JudgeAPIKeyEnv, judgeOverride)
				if err != nil {
					return err
				}
				appOptions.JudgeProvider = judge
				judgeSummary, err = app.RunLongMemEvalQAJudge(command.Context(), appOptions)
				if err != nil {
					return err
				}
			}
			if phase == "finalize" || phase == "all" {
				report, err = app.FinalizeLongMemEvalQA(appOptions)
				if err != nil {
					return err
				}
			}
			switch phase {
			case "reader":
				fmt.Fprintf(command.OutOrStdout(), "phase=reader total=%d completed=%d reader_failed=%d resumed=%d provider_calls=%d\n",
					readerSummary.Total, readerSummary.Completed, readerSummary.Failed, readerSummary.Resumed, readerSummary.ProviderCalls)
			case "judge":
				fmt.Fprintf(command.OutOrStdout(), "phase=judge total=%d judged=%d judge_failed=%d judge_invalid=%d judge_not_run=%d resumed=%d provider_calls=%d\n",
					judgeSummary.Total, judgeSummary.Judged, judgeSummary.Failed, judgeSummary.Invalid, judgeSummary.NotRun, judgeSummary.Resumed, judgeSummary.ProviderCalls)
			case "finalize":
				fmt.Fprintf(command.OutOrStdout(), "phase=finalize records=%d tasks=%d failures=%d report=%s\n",
					report.RecordCount, report.TaskCount, len(report.Failures), report.Artifacts["report"])
			case "all":
				fmt.Fprintf(command.OutOrStdout(), "phase=all reader_total=%d reader_completed=%d reader_failed=%d reader_resumed=%d judge_total=%d judged=%d judge_failed=%d judge_invalid=%d judge_not_run=%d judge_resumed=%d failures=%d report=%s\n",
					readerSummary.Total, readerSummary.Completed, readerSummary.Failed, readerSummary.Resumed,
					judgeSummary.Total, judgeSummary.Judged, judgeSummary.Failed, judgeSummary.Invalid, judgeSummary.NotRun, judgeSummary.Resumed,
					len(report.Failures), report.Artifacts["report"])
			}
			return nil
		},
	}
	command.Flags().StringVar(&options.SourceDatasetPath, "source-dataset", "", "verified official longmemeval_s_cleaned.json path")
	command.Flags().StringVar(&options.RetrievalResultsPath, "retrieval-results", "", "verified LongMemEval retrieval-results.jsonl path")
	command.Flags().StringVar(&options.QualificationPath, "qualification", "casebook/benchmarks/qualifications/longmemeval-s-cleaned-qa.json", "official LongMemEval-S QA qualification manifest")
	command.Flags().StringVar(&options.ExecutionPath, "execution", "casebook/benchmarks/executions/longmemeval-s-full-reader-qa.json", "full reader QA execution manifest")
	command.Flags().StringVar(&options.ArtifactRoot, "artifact-root", "./artifacts", "artifact output root")
	command.Flags().StringVar(&options.RunID, "run-id", "", "stable full reader QA run ID")
	command.Flags().StringVar(&options.ImplementationRevision, "implementation-revision", "", "exact source revision used for this run")
	command.Flags().StringVar(&options.Phase, "phase", "all", "execution phase: reader, judge, all, or finalize")
	command.Flags().StringVar(&options.ReaderCommand, "reader-command", "", "reader CLI command when the manifest provider is grok-cli")
	command.Flags().StringVar(&options.ReaderBaseURL, "reader-base-url", "", "reader OpenAI-compatible base URL")
	command.Flags().StringVar(&options.ReaderAPIKeyEnv, "reader-api-key-env", "", "environment variable containing the reader API key")
	command.Flags().StringVar(&options.JudgeCommand, "judge-command", "", "judge CLI command when the manifest provider is grok-cli")
	command.Flags().StringVar(&options.JudgeBaseURL, "judge-base-url", "", "judge OpenAI-compatible base URL")
	command.Flags().StringVar(&options.JudgeAPIKeyEnv, "judge-api-key-env", "", "environment variable containing the judge API key")
	command.Flags().BoolVar(&options.Resume, "resume", false, "resume only from matching terminal reader and judge checkpoints")
	return command
}

func buildBenchmarkLongMemEvalQAProvider(config benchmark.ExecutionModelConfig, command, baseURL, apiKeyEnv string, override provider.Provider) (provider.Provider, error) {
	if override != nil {
		return override, nil
	}
	switch config.Provider {
	case "mock":
		return provider.Mock{}, nil
	case "grok-cli":
		return provider.NewGrokCLI(provider.GrokCLIConfig{Command: command}), nil
	case "openai-compatible", "siliconflow", "duojie":
		baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
		apiKeyEnv = strings.TrimSpace(apiKeyEnv)
		switch config.Provider {
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
		if baseURL == "" {
			return nil, fmt.Errorf("%s provider requires a base URL", config.Provider)
		}
		apiKey := strings.TrimSpace(os.Getenv(apiKeyEnv))
		if apiKey == "" {
			return nil, fmt.Errorf("%s provider requires non-empty env %s", config.Provider, apiKeyEnv)
		}
		return provider.NewOpenAICompatible(provider.Config{BaseURL: baseURL, APIKey: apiKey}), nil
	default:
		return nil, fmt.Errorf("unsupported LongMemEval QA provider %q", config.Provider)
	}
}
