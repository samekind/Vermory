package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"vermory/internal/authn"
	"vermory/internal/runtime"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestConversationFormationWorkerCommandRegistersBoundedRuntimeFlags(t *testing.T) {
	command := newConversationFormationWorkerCommand()
	if command.Name() != "conversation-formation-worker" {
		t.Fatalf("unexpected command name %q", command.Name())
	}
	for _, name := range []string{
		"database-url", "tenant-id", "provider", "model", "base-url",
		"api-key-env", "grok-command", "disable-thinking", "once",
		"poll-interval", "lease-duration", "retry-delay", "max-attempts",
	} {
		if command.Flags().Lookup(name) == nil {
			t.Fatalf("conversation-formation-worker is missing --%s", name)
		}
	}
}

func TestConversationFormationWorkerCommandRunsOnceWithRestrictedRole(t *testing.T) {
	databaseURL := os.Getenv("VERMORY_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("VERMORY_TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	admin, err := runtime.OpenStore(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	if err := admin.Migrate(ctx); err != nil {
		admin.Close()
		t.Fatal(err)
	}
	if err := admin.ResetForTest(ctx); err != nil {
		admin.Close()
		t.Fatal(err)
	}
	tenantID := "worker-command"
	anchor := runtime.ConversationAnchor{Channel: "openclaw", ThreadID: "worker-command"}
	service := runtime.NewConversationService(admin, tenantID, nil, "", runtime.ConversationServiceConfig{})
	prepared, err := service.PrepareExternalTurn(ctx, runtime.ExternalConversationTurnRequest{
		OperationID: "worker-command-turn",
		Anchor:      anchor,
		Message:     "The submission bundle is thesis-defense-v7.zip.",
	})
	if err != nil {
		admin.Close()
		t.Fatal(err)
	}
	if _, err := service.CompleteExternalTurn(ctx, runtime.CompleteExternalConversationTurnRequest{
		OperationID: prepared.OperationID,
		Anchor:      anchor,
		Answer:      "Acknowledged.",
		Model:       "fixture-client",
	}); err != nil {
		admin.Close()
		t.Fatal(err)
	}
	admin.Close()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	runtimeURL := createConversationFormationWorkerRole(t, pool, databaseURL)

	grokPath := filepath.Join(t.TempDir(), "grok-fixture")
	response := `{"text":"{\"candidates\":[],\"reason\":\"No durable candidate selected.\"}","modelUsage":{"grok-4.5":{}}}`
	if err := os.WriteFile(grokPath, []byte("#!/bin/sh\nprintf '%s\\n' '"+response+"'\n"), 0o700); err != nil {
		t.Fatal(err)
	}

	command := newConversationFormationWorkerCommand()
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetErr(&bytes.Buffer{})
	command.SetArgs([]string{
		"--database-url", runtimeURL,
		"--tenant-id", tenantID,
		"--provider", "grok-cli",
		"--model", "grok-4.5",
		"--grok-command", grokPath,
		"--once",
		"--lease-duration", "1m",
		"--retry-delay", "1s",
	})
	if err := command.ExecuteContext(ctx); err != nil {
		t.Fatal(err)
	}
	var result runtime.ConversationFormationWorkerResult
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatalf("worker output is not JSON: %v\n%s", err, output.String())
	}
	if !result.Found || result.FormationStatus != runtime.SourceFormationAbstained || result.ProcessedThroughSequence == 0 {
		t.Fatalf("unexpected worker result: %#v", result)
	}
	for _, forbidden := range []string{runtimeURL, "vermory-worker-test-only", grokPath} {
		if strings.Contains(output.String(), forbidden) {
			t.Fatalf("worker receipt leaked %q: %s", forbidden, output.String())
		}
	}
}

func createConversationFormationWorkerRole(t *testing.T, pool *pgxpool.Pool, databaseURL string) string {
	t.Helper()
	roleName := "vermory_worker_" + strings.ReplaceAll(time.Now().UTC().Format("150405.000000000"), ".", "")
	roleSQL := pgx.Identifier{roleName}.Sanitize()
	statement := "CREATE ROLE " + roleSQL + " LOGIN PASSWORD 'vermory-worker-test-only' NOSUPERUSER NOBYPASSRLS"
	if _, err := pool.Exec(context.Background(), statement); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DROP OWNED BY "+roleSQL)
		_, _ = pool.Exec(context.Background(), "DROP ROLE IF EXISTS "+roleSQL)
	})
	if err := authn.GrantRuntimeRole(context.Background(), pool, roleName); err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	parsed.User = url.UserPassword(roleName, "vermory-worker-test-only")
	query := parsed.Query()
	query.Set("pool_max_conns", "2")
	parsed.RawQuery = query.Encode()
	if parsed.String() == "" {
		t.Fatal(fmt.Errorf("build restricted runtime URL"))
	}
	return parsed.String()
}
