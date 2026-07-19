package app

import (
	"context"
	"fmt"
	"reflect"
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
	if bundle.ProfileSHA256 != profile.SHA256 {
		return utilityeval.Report{}, fmt.Errorf("utility comparison profile digest mismatch: bundle=%q profile=%q", bundle.ProfileSHA256, profile.SHA256)
	}
	if bundle.ScorerVersion != profile.ScorerVersion {
		return utilityeval.Report{}, fmt.Errorf("utility comparison scorer mismatch: bundle=%q profile=%q", bundle.ScorerVersion, profile.ScorerVersion)
	}
	if err := validateBundleCases(profile.RealityCaseIDs, bundle.Inputs); err != nil {
		return utilityeval.Report{}, err
	}
	if err := validateBundleScoringAliases(profile.ScoringAliases, bundle.Inputs); err != nil {
		return utilityeval.Report{}, err
	}
	if strings.TrimSpace(opts.ArtifactRoot) == "" {
		opts.ArtifactRoot = "./artifacts"
	}
	if opts.MaxTokens <= 0 {
		opts.MaxTokens = profile.MaxOutputTokens
	}
	opts.RunID = chooseRunID(opts.RunID, "utility")
	lane, err := utilityProviderLane(profile, opts.Provider, opts.Model)
	if err != nil {
		return utilityeval.Report{}, err
	}
	llm, providerMode, providerName, model, err := buildProvider(EvalSelfCaseOptions{
		Provider:        opts.Provider,
		BaseURL:         opts.BaseURL,
		APIKeyEnv:       opts.APIKeyEnv,
		Model:           opts.Model,
		DisableThinking: lane.DisableThinking,
	})
	if err != nil {
		return utilityeval.Report{}, err
	}
	return utilityeval.Run(ctx, utilityeval.RunOptions{
		RunID:           opts.RunID,
		ProfileID:       profile.ID,
		ProfileSHA256:   profile.SHA256,
		ProviderName:    providerName,
		ProviderMode:    providerMode,
		Model:           model,
		MaxTokens:       opts.MaxTokens,
		MaxContextBytes: profile.MaxContextBytes,
		ScorerVersion:   profile.ScorerVersion,
		Workers:         profile.Workers,
		DisableThinking: lane.DisableThinking,
		Temperature:     lane.Temperature,
		Inputs:          bundle.Inputs,
		Provider:        llm,
		Artifacts:       artifact.NewLocalStore(opts.ArtifactRoot),
	})
}

func utilityProviderLane(profile utilityeval.Profile, providerName, model string) (utilityeval.ProviderLane, error) {
	providerName = strings.TrimSpace(providerName)
	if providerName == "" || providerName == "mock" {
		return utilityeval.ProviderLane{Provider: "mock", Model: strings.TrimSpace(model)}, nil
	}
	model = strings.TrimSpace(model)
	for _, lane := range profile.ProviderLanes {
		if lane.Provider == providerName && lane.Model == model {
			return lane, nil
		}
	}
	return utilityeval.ProviderLane{}, fmt.Errorf("utility comparison provider lane is not declared: provider=%q model=%q", providerName, model)
}

func validateBundleScoringAliases(expected map[string]map[string][]string, inputs []utilityeval.CaseInput) error {
	for _, input := range inputs {
		want := expected[input.ID]
		if len(want) == 0 && len(input.ScoringAliases) == 0 {
			continue
		}
		if !reflect.DeepEqual(input.ScoringAliases, want) {
			return fmt.Errorf("utility comparison scoring aliases changed for case %q", input.ID)
		}
	}
	return nil
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
