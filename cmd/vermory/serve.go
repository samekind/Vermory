package main

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"vermory/internal/authn"
	"vermory/internal/brand"
	"vermory/internal/runtime"
	"vermory/internal/webchat"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/spf13/cobra"
)

type serveOptions struct {
	DatabaseURL string
	Listen      string
	TLSCert     string
	TLSKey      string
	Provider    webChatProviderOptions
	Retrieval   retrievalRuntimeOptions
}

func (options serveOptions) Validate() error {
	if strings.TrimSpace(options.DatabaseURL) == "" {
		return fmt.Errorf("--database-url is required")
	}
	host, port, err := net.SplitHostPort(strings.TrimSpace(options.Listen))
	if err != nil || strings.TrimSpace(port) == "" {
		return fmt.Errorf("--listen must be a host:port address")
	}
	certSet := strings.TrimSpace(options.TLSCert) != ""
	keySet := strings.TrimSpace(options.TLSKey) != ""
	if certSet != keySet {
		return fmt.Errorf("--tls-cert and --tls-key must be provided together")
	}
	if !isLoopbackHost(host) && !certSet {
		return fmt.Errorf("non-loopback --listen requires --tls-cert and --tls-key")
	}
	return nil
}

func newServeCommand() *cobra.Command {
	options := serveOptionsFromEnvironment()
	command := &cobra.Command{
		Use:   "serve",
		Short: "Run the authenticated multi-tenant Vermory API",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, args []string) error {
			if err := options.Validate(); err != nil {
				return err
			}
			store, err := runtime.OpenStoreWithOptions(command.Context(), options.DatabaseURL, runtime.StoreOptions{EnforceTenantContext: true})
			if err != nil {
				return fmt.Errorf("open authenticated runtime store")
			}
			defer store.Close()
			compatibility, err := store.RuntimeSchemaCompatibility(command.Context(), brand.Revision)
			if err != nil {
				return err
			}
			if err := compatibility.ErrorIfIncompatible(); err != nil {
				return err
			}
			if err := store.ValidateRuntimeRole(command.Context()); err != nil {
				return err
			}
			llm, model, err := buildWebChatProvider(options.Provider)
			if err != nil {
				return err
			}
			retriever, err := buildRuntimeRetriever(store, options.Retrieval)
			if err != nil {
				return err
			}
			authPool, err := pgxpool.New(command.Context(), options.DatabaseURL)
			if err != nil {
				return fmt.Errorf("open authentication database pool")
			}
			defer authPool.Close()
			if err := authPool.Ping(command.Context()); err != nil {
				return fmt.Errorf("connect authentication database pool")
			}
			server := &http.Server{
				Addr:              options.Listen,
				Handler:           webchat.NewAuthenticatedHandlerWithRetriever(store, llm, model, authn.NewPostgresAuthenticator(authPool), retriever),
				ReadHeaderTimeout: 5 * time.Second,
				ReadTimeout:       30 * time.Second,
				WriteTimeout:      5 * time.Minute,
				IdleTimeout:       60 * time.Second,
			}
			go shutdownWebChatServer(command.Context(), server)
			if strings.TrimSpace(options.TLSCert) != "" {
				err = server.ListenAndServeTLS(options.TLSCert, options.TLSKey)
			} else {
				err = server.ListenAndServe()
			}
			if errors.Is(err, http.ErrServerClosed) {
				return nil
			}
			return err
		},
	}
	command.Flags().StringVar(&options.DatabaseURL, "database-url", options.DatabaseURL, "restricted runtime PostgreSQL connection URL")
	command.Flags().StringVar(&options.Listen, "listen", options.Listen, "authenticated API listen address")
	command.Flags().StringVar(&options.TLSCert, "tls-cert", options.TLSCert, "TLS certificate path")
	command.Flags().StringVar(&options.TLSKey, "tls-key", options.TLSKey, "TLS private key path")
	command.Flags().StringVar(&options.Provider.Name, "provider", options.Provider.Name, "provider: external, mock, grok-cli, openai-compatible, siliconflow, or duojie")
	command.Flags().StringVar(&options.Provider.Model, "model", options.Provider.Model, "server-owned provider model")
	command.Flags().StringVar(&options.Provider.BaseURL, "base-url", options.Provider.BaseURL, "direct provider base URL")
	command.Flags().StringVar(&options.Provider.APIKeyEnv, "api-key-env", options.Provider.APIKeyEnv, "environment variable containing provider API key")
	command.Flags().StringVar(&options.Provider.GrokCommand, "grok-command", options.Provider.GrokCommand, "authenticated Grok CLI command")
	addSharedRetrievalFlags(command, &options.Retrieval)
	return command
}

func serveOptionsFromEnvironment() serveOptions {
	return serveOptions{
		DatabaseURL: strings.TrimSpace(os.Getenv("VERMORY_DATABASE_URL")),
		Listen:      environmentDefault("VERMORY_LISTEN", "127.0.0.1:8788"),
		TLSCert:     strings.TrimSpace(os.Getenv("VERMORY_TLS_CERT")),
		TLSKey:      strings.TrimSpace(os.Getenv("VERMORY_TLS_KEY")),
		Provider: webChatProviderOptions{
			Name:        environmentDefault("VERMORY_PROVIDER", "mock"),
			Model:       strings.TrimSpace(os.Getenv("VERMORY_MODEL")),
			BaseURL:     strings.TrimSpace(os.Getenv("VERMORY_PROVIDER_BASE_URL")),
			APIKeyEnv:   strings.TrimSpace(os.Getenv("VERMORY_PROVIDER_API_KEY_ENV")),
			GrokCommand: strings.TrimSpace(os.Getenv("VERMORY_GROK_COMMAND")),
		},
		Retrieval: defaultRetrievalRuntimeOptions(),
	}
}

func environmentDefault(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
