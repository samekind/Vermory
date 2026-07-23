package main

import (
	"fmt"

	"vermory/internal/app"

	"github.com/spf13/cobra"
)

func newBenchmarkLongMemEvalCommand() *cobra.Command {
	options := app.LongMemEvalOptions{}
	command := &cobra.Command{
		Use:   "benchmark-longmemeval",
		Short: "Run a qualified LongMemEval original-dataset sample",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, args []string) error {
			report, err := app.RunLongMemEvalSample(command.Context(), options)
			if err != nil {
				return err
			}
			fmt.Fprintf(command.OutOrStdout(), "scope=%s claim_scope=%s records=%d provider=%s model=%s report=%s\n",
				report.ExecutionScope,
				report.ClaimScope,
				len(report.SelectedRecordIDs),
				report.ProviderName,
				report.Model,
				report.Artifacts["report"],
			)
			return nil
		},
	}
	command.Flags().StringVar(&options.DatabaseURL, "database-url", "", "dedicated PostgreSQL connection URL")
	command.Flags().StringVar(&options.SourceDatasetPath, "source-dataset", "", "verified official longmemeval_oracle.json path")
	command.Flags().StringVar(&options.QualificationPath, "qualification", "casebook/benchmarks/qualifications/longmemeval-cleaned-oracle.json", "official source qualification manifest")
	command.Flags().StringVar(&options.ExecutionPath, "execution", "casebook/benchmarks/executions/longmemeval-oracle-sample.json", "frozen sample execution manifest")
	command.Flags().StringVar(&options.ArtifactRoot, "artifact-root", "./artifacts", "artifact output root")
	command.Flags().StringVar(&options.Provider, "provider", "grok-cli", "provider: mock, grok-cli, openai-compatible, siliconflow, or duojie")
	command.Flags().StringVar(&options.BaseURL, "base-url", "", "direct provider base URL")
	command.Flags().StringVar(&options.APIKeyEnv, "api-key-env", "", "environment variable containing provider API key")
	command.Flags().StringVar(&options.Model, "model", "", "provider model name")
	command.Flags().StringVar(&options.RunID, "run-id", "", "stable original-dataset sample run id")
	command.Flags().StringVar(&options.ImplementationRevision, "implementation-revision", "", "exact source revision used for this run")
	return command
}
