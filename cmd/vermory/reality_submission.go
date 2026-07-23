package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"vermory/internal/reality"

	"github.com/spf13/cobra"
)

type realitySubmissionCreateOptions struct {
	Artifact       string
	ArtifactURI    string
	SourceRevision string
	SubmissionID   string
	SuiteProfile   string
	Interfaces     []string
	Platforms      []string
	CreatedAt      string
	ValidFor       time.Duration
	Nonce          string
	Output         string
}

func newRealitySubmissionCreateCommand() *cobra.Command {
	options := realitySubmissionCreateOptions{ValidFor: 7 * 24 * time.Hour}
	command := &cobra.Command{
		Use:   "reality-submission-create",
		Short: "Bind an exact Vermory artifact into an external evaluation submission",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, args []string) error {
			artifactPath, err := filepath.Abs(options.Artifact)
			if err != nil {
				return fmt.Errorf("resolve --artifact: %w", err)
			}
			outputPath, err := filepath.Abs(options.Output)
			if err != nil {
				return fmt.Errorf("resolve --output: %w", err)
			}
			if artifactPath == outputPath {
				return fmt.Errorf("--output must not overwrite --artifact")
			}
			createdAt := time.Now().UTC().Truncate(time.Second)
			if strings.TrimSpace(options.CreatedAt) != "" {
				parsed, err := time.Parse(time.RFC3339, options.CreatedAt)
				if err != nil {
					return fmt.Errorf("parse --created-at: %w", err)
				}
				createdAt = parsed.UTC()
			}
			if options.ValidFor <= 0 {
				return fmt.Errorf("--valid-for must be positive")
			}
			nonce := strings.TrimSpace(options.Nonce)
			if nonce == "" {
				generated, err := reality.GenerateEvaluationSubmissionNonce()
				if err != nil {
					return err
				}
				nonce = generated
			}
			submission, err := reality.BuildEvaluationSubmission(reality.EvaluationSubmission{
				Version:         1,
				ProtocolVersion: reality.ExternalEvaluationProtocolVersion,
				SubmissionID:    strings.TrimSpace(options.SubmissionID),
				Nonce:           nonce,
				CreatedAt:       createdAt,
				ExpiresAt:       createdAt.Add(options.ValidFor),
				SuiteProfile:    strings.TrimSpace(options.SuiteProfile),
				Implementation: reality.EvaluationImplementation{
					SourceRevision: strings.TrimSpace(options.SourceRevision),
					ArtifactURI:    strings.TrimSpace(options.ArtifactURI),
				},
				Interfaces: append([]string(nil), options.Interfaces...),
				Platforms:  append([]string(nil), options.Platforms...),
				Execution: reality.EvaluationExecutionBoundary{
					Database:       reality.EvaluationDatabaseEphemeralPostgres,
					Provider:       reality.EvaluationProviderOwnedProxy,
					Network:        reality.EvaluationNetworkDenyExceptProvider,
					Telemetry:      reality.EvaluationTelemetryDisabled,
					ResultArtifact: reality.EvaluationResultEvaluatorControlled,
				},
			}, artifactPath)
			if err != nil {
				return err
			}
			if err := reality.WriteEvaluationSubmission(outputPath, submission); err != nil {
				return err
			}
			digest, err := reality.EvaluationSubmissionDigest(submission)
			if err != nil {
				return err
			}
			fmt.Fprintf(command.OutOrStdout(), "submission=%s digest=%s implementation=%s output=%s\n",
				submission.SubmissionID, digest, submission.Implementation.ArtifactSHA256, outputPath)
			return nil
		},
	}
	command.Flags().StringVar(&options.Artifact, "artifact", "", "exact local implementation artifact")
	command.Flags().StringVar(&options.ArtifactURI, "artifact-uri", "", "immutable public HTTPS URI for the artifact")
	command.Flags().StringVar(&options.SourceRevision, "source-revision", "", "exact lowercase source revision")
	command.Flags().StringVar(&options.SubmissionID, "submission-id", "", "unique lowercase submission identifier")
	command.Flags().StringVar(&options.SuiteProfile, "suite-profile", "", "requested external suite profile")
	command.Flags().StringArrayVar(&options.Interfaces, "interface", nil, "versioned product interface; repeat for multiple interfaces")
	command.Flags().StringArrayVar(&options.Platforms, "platform", nil, "supported runtime platform; repeat for multiple platforms")
	command.Flags().StringVar(&options.CreatedAt, "created-at", "", "RFC3339 UTC creation time; defaults to current time")
	command.Flags().DurationVar(&options.ValidFor, "valid-for", options.ValidFor, "submission validity duration")
	command.Flags().StringVar(&options.Nonce, "nonce", "", "optional 32-byte lowercase hexadecimal nonce")
	command.Flags().StringVar(&options.Output, "output", "", "submission JSON output path")
	for _, name := range []string{"artifact", "artifact-uri", "source-revision", "submission-id", "suite-profile", "interface", "platform", "output"} {
		_ = command.MarkFlagRequired(name)
	}
	return command
}

func newRealitySubmissionVerifyCommand() *cobra.Command {
	var input string
	var artifact string
	command := &cobra.Command{
		Use:   "reality-submission-verify",
		Short: "Verify an external evaluation submission and optional artifact bytes",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, args []string) error {
			data, err := os.ReadFile(input)
			if err != nil {
				return err
			}
			submission, err := reality.ParseEvaluationSubmission(data)
			if err != nil {
				return err
			}
			if strings.TrimSpace(artifact) != "" {
				if err := reality.VerifyEvaluationSubmissionArtifact(submission, artifact); err != nil {
					return err
				}
			}
			digest, err := reality.EvaluationSubmissionDigest(submission)
			if err != nil {
				return err
			}
			fmt.Fprintf(command.OutOrStdout(), "submission=%s digest=%s implementation=%s expires_at=%s artifact_verified=%t\n",
				submission.SubmissionID, digest, submission.Implementation.ArtifactSHA256,
				submission.ExpiresAt.Format(time.RFC3339), strings.TrimSpace(artifact) != "")
			return nil
		},
	}
	command.Flags().StringVar(&input, "input", "", "external evaluation submission JSON path")
	command.Flags().StringVar(&artifact, "artifact", "", "optional local artifact path to verify")
	_ = command.MarkFlagRequired("input")
	return command
}
