package main

import (
	"fmt"
	"os"
	"strings"

	"vermory/internal/retrievalablation"

	"github.com/spf13/cobra"
)

var executeRetrievalProfileComparison = retrievalablation.ExecuteProfileComparison

func newRetrievalProfileComparisonCommand() *cobra.Command {
	options := retrievalablation.ProfileComparisonRunOptions{}
	var outputDir string
	var apiKeyEnv string
	command := &cobra.Command{
		Use:   "retrieval-profile-compare",
		Short: "Compare registered semantic retrieval profiles on one governed corpus",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, args []string) error {
			apiKeyEnv = strings.TrimSpace(apiKeyEnv)
			if apiKeyEnv == "" {
				return fmt.Errorf("--embedding-api-key-env is required")
			}
			options.EmbeddingAPIKey = os.Getenv(apiKeyEnv)
			if strings.TrimSpace(options.EmbeddingAPIKey) == "" {
				return fmt.Errorf("embedding API key environment variable %s is not set", apiKeyEnv)
			}
			report, paths, replayed, err := executeRetrievalProfileComparison(command.Context(), options, outputDir)
			if err != nil {
				return err
			}
			queries := 0
			if len(report.Profiles) > 0 {
				queries = report.Profiles[0].Metrics.QueryCount
			}
			fmt.Fprintf(command.OutOrStdout(), "run=%s queries=%d hard_gates=%s decision=%s qualification=%s replayed=%t report=%s\n",
				report.RunID, queries, cliPassLabel(report.HardGates.Pass), report.Decision.Status,
				report.QualificationStatus, replayed, paths.JSON)
			return nil
		},
	}
	command.Flags().StringVar(&options.DatabaseURL, "database-url", "", "dedicated PostgreSQL connection URL")
	command.Flags().StringVar(&options.CorpusPath, "corpus", "", "versioned retrieval corpus JSON path")
	command.Flags().StringVar(&options.RunID, "run-id", "", "stable profile comparison run id")
	command.Flags().StringVar(&outputDir, "output-dir", "", "directory for report.json and report.md")
	command.Flags().StringVar(&apiKeyEnv, "embedding-api-key-env", "SILICONFLOW_API_KEY", "environment variable containing the embedding API key")
	command.Flags().StringVar(&options.ImplementationRevision, "implementation-revision", "", "exact source revision used for this run")
	return command
}
