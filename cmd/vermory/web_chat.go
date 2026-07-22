package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"vermory/internal/provider"
	"vermory/internal/runtime"
	"vermory/internal/webchat"

	"github.com/spf13/cobra"
)

type webChatOptions struct {
	DatabaseURL string
	TenantID    string
	Listen      string
	Provider    webChatProviderOptions
	Retrieval   retrievalRuntimeOptions
}

func (o webChatOptions) Validate() error {
	if strings.TrimSpace(o.DatabaseURL) == "" {
		return fmt.Errorf("--database-url is required")
	}
	if strings.TrimSpace(o.TenantID) == "" {
		return fmt.Errorf("--tenant-id is required")
	}
	host, port, err := net.SplitHostPort(strings.TrimSpace(o.Listen))
	if err != nil || strings.TrimSpace(port) == "" {
		return fmt.Errorf("--listen must be a host:port address")
	}
	if host != "localhost" {
		ip := net.ParseIP(host)
		if ip == nil || !ip.IsLoopback() {
			return fmt.Errorf("--listen must use a loopback address")
		}
	}
	return nil
}

type webChatProviderOptions struct {
	Name        string
	Model       string
	BaseURL     string
	APIKeyEnv   string
	GrokCommand string
}

func newWebChatCommand() *cobra.Command {
	options := webChatOptions{Retrieval: defaultRetrievalRuntimeOptions()}
	command := &cobra.Command{
		Use:   "web-chat",
		Short: "Run the local conversation continuity Web Chat application",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, args []string) error {
			if err := options.Validate(); err != nil {
				return err
			}
			llm, model, err := buildWebChatProvider(options.Provider)
			if err != nil {
				return err
			}
			store, err := runtime.OpenStore(command.Context(), options.DatabaseURL)
			if err != nil {
				return fmt.Errorf("open Web Chat runtime store")
			}
			defer store.Close()
			if err := store.Migrate(command.Context()); err != nil {
				return fmt.Errorf("migrate Web Chat runtime store")
			}
			retriever, err := buildRuntimeRetriever(store, options.Retrieval)
			if err != nil {
				return err
			}
			service := runtime.NewConversationService(
				store,
				options.TenantID,
				llm,
				model,
				runtime.ConversationServiceConfig{Retriever: retriever},
			)
			defaults := runtime.NewGlobalDefaultsService(store, options.TenantID)
			bridges := runtime.NewBridgeService(store, options.TenantID)
			server := &http.Server{
				Addr:              options.Listen,
				Handler:           webchat.NewHandlerWithGovernance(service, defaults, bridges),
				ReadHeaderTimeout: 5 * time.Second,
				ReadTimeout:       30 * time.Second,
				WriteTimeout:      5 * time.Minute,
				IdleTimeout:       60 * time.Second,
			}
			go shutdownWebChatServer(command.Context(), server)
			err = server.ListenAndServe()
			if errors.Is(err, http.ErrServerClosed) {
				return nil
			}
			return err
		},
	}
	command.Flags().StringVar(&options.DatabaseURL, "database-url", "", "PostgreSQL connection URL")
	command.Flags().StringVar(&options.TenantID, "tenant-id", "", "server-owned tenant identifier")
	command.Flags().StringVar(&options.Listen, "listen", "127.0.0.1:8787", "loopback listen address")
	command.Flags().StringVar(&options.Provider.Name, "provider", "mock", "provider: external, mock, grok-cli, openai-compatible, siliconflow, or duojie")
	command.Flags().StringVar(&options.Provider.Model, "model", "", "server-owned provider model")
	command.Flags().StringVar(&options.Provider.BaseURL, "base-url", "", "direct provider base URL")
	command.Flags().StringVar(&options.Provider.APIKeyEnv, "api-key-env", "", "environment variable containing provider API key")
	command.Flags().StringVar(&options.Provider.GrokCommand, "grok-command", "", "authenticated Grok CLI command")
	addSharedRetrievalFlags(command, &options.Retrieval)
	return command
}

func buildWebChatProvider(options webChatProviderOptions) (provider.Provider, string, error) {
	name := strings.TrimSpace(options.Name)
	if name == "" {
		name = "mock"
	}
	model := strings.TrimSpace(options.Model)
	switch name {
	case "external":
		return nil, "", nil
	case "mock":
		if model == "" {
			model = "mock-model"
		}
		return provider.Mock{}, model, nil
	case "grok-cli":
		if model == "" {
			model = "grok-4.5"
		}
		return provider.NewGrokCLI(provider.GrokCLIConfig{Command: options.GrokCommand}), model, nil
	case "openai-compatible", "siliconflow", "duojie":
		baseURL := strings.TrimRight(strings.TrimSpace(options.BaseURL), "/")
		apiKeyEnv := strings.TrimSpace(options.APIKeyEnv)
		switch name {
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
		if model == "" {
			return nil, "", fmt.Errorf("%s provider requires --model", name)
		}
		if baseURL == "" {
			return nil, "", fmt.Errorf("%s provider requires --base-url", name)
		}
		apiKey := strings.TrimSpace(os.Getenv(apiKeyEnv))
		if apiKey == "" {
			return nil, "", fmt.Errorf("%s provider requires non-empty env %s", name, apiKeyEnv)
		}
		return provider.NewOpenAICompatible(provider.Config{BaseURL: baseURL, APIKey: apiKey}), model, nil
	default:
		return nil, "", fmt.Errorf("unsupported provider %q", name)
	}
}

func shutdownWebChatServer(ctx context.Context, server *http.Server) {
	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = server.Shutdown(shutdownCtx)
}
