package app

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"vermory/internal/artifact"
	"vermory/internal/utilityeval"
)

type UtilityComparisonOptions struct {
	ProfilePath  string
	BundlePath   string
	ArtifactRoot string
	Provider     string
	BaseURL      string
	APIKeyEnv    string
	Model        string
	RunID        string
	MaxTokens    int
}

func EvalUtilityComparison(ctx context.Context, opts UtilityComparisonOptions) (utilityeval.Report, error) {
	profile, err := utilityeval.LoadProfile(opts.ProfilePath)
	if err != nil {
		return utilityeval.Report{}, err
	}
	bundle, err := utilityeval.LoadContextBundle(opts.BundlePath)
	if err != nil {
		return utilityeval.Report{}, err
	}
	if bundle.ProfileID != profile.ID {
		return utilityeval.Report{}, fmt.Errorf("utility comparison profile mismatch: bundle=%q profile=%q", bundle.ProfileID, profile.ID)
	}
	if err := validateBundleCases(profile.RealityCaseIDs, bundle.Inputs); err != nil {
		return utilityeval.Report{}, err
	}
	if strings.TrimSpace(opts.ArtifactRoot) == "" {
		opts.ArtifactRoot = "./artifacts"
	}
	if opts.MaxTokens <= 0 {
		opts.MaxTokens = profile.MaxOutputTokens
	}
	opts.RunID = chooseRunID(opts.RunID, "utility")
	llm, providerMode, providerName, model, err := buildProvider(EvalSelfCaseOptions{
		Provider:  opts.Provider,
		BaseURL:   opts.BaseURL,
		APIKeyEnv: opts.APIKeyEnv,
		Model:     opts.Model,
	})
	if err != nil {
		return utilityeval.Report{}, err
	}
	return utilityeval.Run(ctx, utilityeval.RunOptions{
		RunID:           opts.RunID,
		ProviderName:    providerName,
		ProviderMode:    providerMode,
		Model:           model,
		MaxTokens:       opts.MaxTokens,
		MaxContextBytes: profile.MaxContextBytes,
		Inputs:          bundle.Inputs,
		Provider:        llm,
		Artifacts:       artifact.NewLocalStore(opts.ArtifactRoot),
	})
}

func validateBundleCases(expected []string, inputs []utilityeval.CaseInput) error {
	got := make([]string, 0, len(inputs))
	for _, input := range inputs {
		got = append(got, input.ID)
	}
	sort.Strings(got)
	want := append([]string(nil), expected...)
	sort.Strings(want)
	if len(got) != len(want) {
		return fmt.Errorf("utility comparison case count mismatch: got=%d want=%d", len(got), len(want))
	}
	for index := range want {
		if got[index] != want[index] {
			return fmt.Errorf("utility comparison case mismatch: got=%q want=%q", got[index], want[index])
		}
	}
	return nil
}
