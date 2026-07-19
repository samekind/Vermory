package main

import (
	"fmt"
	"os"
	"strings"

	"vermory/internal/app"

	"github.com/spf13/cobra"
)

func newBenchmarkLongMemEvalRetrievalCommand() *cobra.Command {
	options := app.LongMemEvalRetrievalOptions{}
	embeddingAPIKeyEnv := "SILICONFLOW_API_KEY"
	command := &cobra.Command{
		Use:   "benchmark-longmemeval-retrieval",
		Short: "Run full LongMemEval-S retrieval qualification",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, args []string) error {
			if strings.TrimSpace(options.VectorProfilePath) != "" {
				if strings.TrimSpace(embeddingAPIKeyEnv) == "" {
					return fmt.Errorf("--embedding-api-key-env is required for vector retrieval")
				}
				options.EmbeddingAPIKey = os.Getenv(embeddingAPIKeyEnv)
			}
			report, err := app.RunLongMemEvalRetrieval(command.Context(), options)
			if err != nil {
				return err
			}
			fmt.Fprintf(command.OutOrStdout(), "target=%s scope=%s claim_scope=%s records=%d scored=%d report=%s\n",
				report.EvaluationTarget,
				report.ExecutionScope,
				report.ClaimScope,
				report.RecordCount,
				report.ScoredRecordCount,
				report.Artifacts["report"],
			)
			return nil
		},
	}
	command.Flags().StringVar(&options.DatabaseURL, "database-url", "", "dedicated PostgreSQL connection URL")
	command.Flags().StringVar(&options.SourceDatasetPath, "source-dataset", "", "verified official longmemeval_s_cleaned.json path")
	command.Flags().StringVar(&options.QualificationPath, "qualification", "casebook/benchmarks/qualifications/longmemeval-s-cleaned.json", "official LongMemEval-S qualification manifest")
	command.Flags().StringVar(&options.ExecutionPath, "execution", "casebook/benchmarks/executions/longmemeval-s-full-retrieval.json", "full retrieval execution manifest")
	command.Flags().StringVar(&options.ArtifactRoot, "artifact-root", "./artifacts", "artifact output root")
	command.Flags().StringVar(&options.RunID, "run-id", "", "stable full retrieval run ID")
	command.Flags().StringVar(&options.ImplementationRevision, "implementation-revision", "", "exact source revision used for this run")
	command.Flags().BoolVar(&options.Resume, "resume", false, "resume only from matching atomic record checkpoints")
	command.Flags().StringVar(&options.VectorProfilePath, "vector-profile", "", "frozen production vector retrieval profile")
	command.Flags().StringVar(&embeddingAPIKeyEnv, "embedding-api-key-env", embeddingAPIKeyEnv, "environment variable containing the direct embedding API key")
	return command
}
