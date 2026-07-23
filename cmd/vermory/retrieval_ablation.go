package main

import (
	"fmt"
	"os"
	"strings"

	"vermory/internal/retrievalablation"

	"github.com/spf13/cobra"
)

var executeRetrievalAblation = retrievalablation.Execute

func newRetrievalAblationCommand() *cobra.Command {
	options := retrievalablation.Options{}
	var outputDir string
	var apiKeyEnv string
	command := &cobra.Command{
		Use:   "retrieval-ablation",
		Short: "Measure lexical, vector, and hybrid governed retrieval",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, args []string) error {
			apiKeyEnv = strings.TrimSpace(apiKeyEnv)
			if apiKeyEnv == "" {
				return fmt.Errorf("--embedding-api-key-env is required")
			}
			options.EmbeddingAPIKey = os.Getenv(apiKeyEnv)
			if options.EmbeddingAPIKey == "" {
				return fmt.Errorf("embedding API key environment variable %s is not set", apiKeyEnv)
			}
			report, paths, replayed, err := executeRetrievalAblation(command.Context(), options, outputDir)
			if err != nil {
				return err
			}
			fmt.Fprintf(command.OutOrStdout(), "run=%s queries=%d hard_gates=%s qualification=%s replayed=%t report=%s\n",
				report.RunID, reportQueryCount(report), cliPassLabel(report.HardGates.Pass),
				report.QualificationStatus, replayed, paths.JSON)
			return nil
		},
	}
	command.Flags().StringVar(&options.DatabaseURL, "database-url", "", "dedicated PostgreSQL connection URL")
	command.Flags().StringVar(&options.CorpusPath, "corpus", "", "versioned retrieval corpus JSON path")
	command.Flags().StringVar(&options.RunID, "run-id", "", "stable retrieval ablation run id")
	command.Flags().StringVar(&outputDir, "output-dir", "", "directory for report.json and report.md")
	command.Flags().StringVar(&options.EmbeddingBaseURL, "embedding-base-url", "", "direct OpenAI-compatible embedding API base URL")
	command.Flags().StringVar(&apiKeyEnv, "embedding-api-key-env", "", "environment variable containing the embedding API key")
	command.Flags().StringVar(&options.EmbeddingModel, "embedding-model", "BAAI/bge-m3", "embedding model name")
	command.Flags().IntVar(&options.EmbeddingDimensions, "embedding-dimensions", 1024, "embedding vector dimensions")
	command.Flags().StringVar(&options.ImplementationRevision, "implementation-revision", "", "exact source revision used for this run")
	return command
}

func reportQueryCount(report retrievalablation.Report) int {
	for _, condition := range report.Conditions {
		if condition.Name == retrievalablation.ConditionLexical {
			return condition.Metrics.QueryCount
		}
	}
	return 0
}

func cliPassLabel(pass bool) string {
	if pass {
		return "pass"
	}
	return "fail"
}
