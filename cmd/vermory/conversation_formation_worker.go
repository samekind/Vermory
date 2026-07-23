package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"vermory/internal/provider"
	"vermory/internal/runtime"

	"github.com/spf13/cobra"
)

type conversationFormationWorkerCommandOptions struct {
	DatabaseURL     string
	TenantID        string
	Provider        string
	Model           string
	BaseURL         string
	APIKeyEnv       string
	GrokCommand     string
	DisableThinking bool
	Once            bool
	PollInterval    time.Duration
	LeaseDuration   time.Duration
	RetryDelay      time.Duration
	MaxAttempts     int
}

func newConversationFormationWorkerCommand() *cobra.Command {
	options := conversationFormationWorkerCommandOptions{
		Provider:      "grok-cli",
		PollInterval:  time.Second,
		LeaseDuration: 2 * time.Minute,
		RetryDelay:    30 * time.Second,
		MaxAttempts:   5,
	}
	command := &cobra.Command{
		Use:   "conversation-formation-worker",
		Short: "Form reviewable memory candidates from completed conversation turns",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, args []string) error {
			if strings.TrimSpace(options.DatabaseURL) == "" {
				return fmt.Errorf("--database-url is required")
			}
			if strings.TrimSpace(options.TenantID) == "" {
				return fmt.Errorf("--tenant-id is required")
			}
			llm, providerName, model, err := provider.BuildDirect(provider.DirectOptions{
				Name:            options.Provider,
				Model:           options.Model,
				BaseURL:         options.BaseURL,
				APIKeyEnv:       options.APIKeyEnv,
				GrokCommand:     options.GrokCommand,
				DisableThinking: options.DisableThinking,
			})
			if err != nil {
				return err
			}
			store, err := runtime.OpenStoreWithOptions(command.Context(), options.DatabaseURL, runtime.StoreOptions{EnforceTenantContext: true})
			if err != nil {
				return fmt.Errorf("open conversation formation worker store")
			}
			defer store.Close()
			if err := store.ValidateRuntimeRole(command.Context()); err != nil {
				return err
			}
			formation := runtime.NewSourceFormationService(store, options.TenantID, llm, providerName, model)
			worker, err := runtime.NewConversationFormationWorker(store, formation, runtime.ConversationFormationWorkerOptions{
				TenantID:      options.TenantID,
				PollInterval:  options.PollInterval,
				LeaseDuration: options.LeaseDuration,
				RetryDelay:    options.RetryDelay,
				MaxAttempts:   options.MaxAttempts,
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
	command.Flags().StringVar(&options.Provider, "provider", options.Provider, "provider: grok-cli, openai-compatible, siliconflow, or duojie")
	command.Flags().StringVar(&options.Model, "model", "", "provider model name")
	command.Flags().StringVar(&options.BaseURL, "base-url", "", "direct provider base URL")
	command.Flags().StringVar(&options.APIKeyEnv, "api-key-env", "", "environment variable containing provider API key")
	command.Flags().StringVar(&options.GrokCommand, "grok-command", "", "authenticated Grok CLI command")
	command.Flags().BoolVar(&options.DisableThinking, "disable-thinking", false, "request non-thinking mode from compatible providers")
	command.Flags().BoolVar(&options.Once, "once", false, "process at most one formation window and exit")
	command.Flags().DurationVar(&options.PollInterval, "poll-interval", options.PollInterval, "continuous worker poll interval")
	command.Flags().DurationVar(&options.LeaseDuration, "lease-duration", options.LeaseDuration, "worker claim lease duration")
	command.Flags().DurationVar(&options.RetryDelay, "retry-delay", options.RetryDelay, "delay before retrying a failed formation attempt")
	command.Flags().IntVar(&options.MaxAttempts, "max-attempts", options.MaxAttempts, "maximum provider attempts retained for one pending schedule")
	return command
}
