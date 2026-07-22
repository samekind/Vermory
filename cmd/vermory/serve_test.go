package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"vermory/internal/runtime"
)

func TestServeOptionsRequireRuntimeDatabaseAndTLSForNonLoopback(t *testing.T) {
	validLoopback := serveOptions{DatabaseURL: "postgresql:///vermory", Listen: "127.0.0.1:8788"}
	if err := validLoopback.Validate(); err != nil {
		t.Fatalf("loopback without TLS should be valid: %v", err)
	}
	validTLS := serveOptions{
		DatabaseURL: "postgresql:///vermory",
		Listen:      "0.0.0.0:8788",
		TLSCert:     "/etc/vermory/tls.crt",
		TLSKey:      "/etc/vermory/tls.key",
	}
	if err := validTLS.Validate(); err != nil {
		t.Fatalf("non-loopback with TLS pair should be valid: %v", err)
	}

	tests := map[string]serveOptions{
		"database":  {Listen: "127.0.0.1:8788"},
		"address":   {DatabaseURL: "postgresql:///vermory", Listen: "not-an-address"},
		"cleartext": {DatabaseURL: "postgresql:///vermory", Listen: "0.0.0.0:8788"},
		"cert-only": {DatabaseURL: "postgresql:///vermory", Listen: "127.0.0.1:8788", TLSCert: "/tmp/cert"},
		"key-only":  {DatabaseURL: "postgresql:///vermory", Listen: "127.0.0.1:8788", TLSKey: "/tmp/key"},
	}
	for name, options := range tests {
		t.Run(name, func(t *testing.T) {
			if err := options.Validate(); err == nil {
				t.Fatalf("unsafe serve options were accepted: %#v", options)
			}
		})
	}
}

func TestServeCommandExposesNoTenantOrImplicitMigrationControls(t *testing.T) {
	command := newServeCommand()
	for _, forbidden := range []string{"tenant-id", "migrate", "admin-database-url"} {
		if command.Flags().Lookup(forbidden) != nil {
			t.Fatalf("serve must not expose --%s", forbidden)
		}
	}
	for _, required := range []string{"database-url", "listen", "tls-cert", "tls-key", "provider", "model"} {
		if command.Flags().Lookup(required) == nil {
			t.Fatalf("serve is missing --%s", required)
		}
	}
}

func TestServeOptionsUseProtectedEnvironmentDefaults(t *testing.T) {
	t.Setenv("VERMORY_DATABASE_URL", " service=vermory-runtime ")
	t.Setenv("VERMORY_LISTEN", "127.0.0.1:9797")
	t.Setenv("VERMORY_TLS_CERT", "/etc/vermory/tls.crt")
	t.Setenv("VERMORY_TLS_KEY", "/etc/vermory/tls.key")
	t.Setenv("VERMORY_PROVIDER", "external")
	t.Setenv("VERMORY_MODEL", "server-model")
	t.Setenv("VERMORY_PROVIDER_BASE_URL", "https://provider.example/v1")
	t.Setenv("VERMORY_PROVIDER_API_KEY_ENV", "VERMORY_PROVIDER_SECRET")
	t.Setenv("VERMORY_GROK_COMMAND", "/usr/local/bin/grok-wrapper")

	options := serveOptionsFromEnvironment()
	if options.DatabaseURL != "service=vermory-runtime" ||
		options.Listen != "127.0.0.1:9797" ||
		options.TLSCert != "/etc/vermory/tls.crt" ||
		options.TLSKey != "/etc/vermory/tls.key" ||
		options.Provider.Name != "external" ||
		options.Provider.Model != "server-model" ||
		options.Provider.BaseURL != "https://provider.example/v1" ||
		options.Provider.APIKeyEnv != "VERMORY_PROVIDER_SECRET" ||
		options.Provider.GrokCommand != "/usr/local/bin/grok-wrapper" {
		t.Fatalf("serve environment defaults drifted: %#v", options)
	}

	command := newServeCommand()
	wantDefaults := map[string]string{
		"database-url": "service=vermory-runtime",
		"listen":       "127.0.0.1:9797",
		"tls-cert":     "/etc/vermory/tls.crt",
		"tls-key":      "/etc/vermory/tls.key",
		"provider":     "external",
		"model":        "server-model",
		"base-url":     "https://provider.example/v1",
		"api-key-env":  "VERMORY_PROVIDER_SECRET",
		"grok-command": "/usr/local/bin/grok-wrapper",
	}
	for name, want := range wantDefaults {
		got, err := command.Flags().GetString(name)
		if err != nil || got != want {
			t.Fatalf("--%s did not preserve environment default: got=%q want=%q err=%v", name, got, want, err)
		}
	}
	if err := command.Flags().Set("listen", "127.0.0.1:9898"); err != nil {
		t.Fatal(err)
	}
	if got, err := command.Flags().GetString("listen"); err != nil || got != "127.0.0.1:9898" {
		t.Fatalf("explicit flag did not override environment default: got=%q err=%v", got, err)
	}
}

func TestServeEnvironmentKeepsSafeDefaultsWhenUnsetOrBlank(t *testing.T) {
	t.Setenv("VERMORY_DATABASE_URL", "")
	t.Setenv("VERMORY_LISTEN", "  ")
	t.Setenv("VERMORY_PROVIDER", "")

	options := serveOptionsFromEnvironment()
	if options.DatabaseURL != "" || options.Listen != "127.0.0.1:8788" || options.Provider.Name != "mock" {
		t.Fatalf("blank environment changed safe defaults: %#v", options)
	}
}

func TestServeRejectsUnsafeDatabaseRoleBeforeListening(t *testing.T) {
	databaseURL := os.Getenv("VERMORY_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("VERMORY_TEST_DATABASE_URL is not set")
	}
	store, err := runtime.OpenStore(context.Background(), databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Migrate(context.Background()); err != nil {
		store.Close()
		t.Fatal(err)
	}
	store.Close()

	command := newServeCommand()
	command.SetOut(&bytes.Buffer{})
	command.SetErr(&bytes.Buffer{})
	command.SetArgs([]string{"--database-url", databaseURL, "--listen", "127.0.0.1:0", "--provider", "mock"})
	err = command.Execute()
	if !errors.Is(err, runtime.ErrUnsafeRuntimeRole) {
		t.Fatalf("serve did not reject admin/table-owner role: %v", err)
	}
	if err != nil && strings.Contains(err.Error(), databaseURL) {
		t.Fatalf("serve error exposed database URL: %v", err)
	}
}
