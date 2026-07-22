package main

import (
	"context"
	"errors"
	"os"
	"testing"

	"vermory/internal/runtime"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestStandaloneRuntimeStoreUsesRestrictedRoleWithoutMigrating(t *testing.T) {
	databaseURL := os.Getenv("VERMORY_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("VERMORY_TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	admin, err := runtime.OpenStore(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(admin.Close)
	if err := admin.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if err := admin.ResetForTest(ctx); err != nil {
		t.Fatal(err)
	}

	const tenantID = "standalone-runtime"
	const repoRoot = "/fixtures/standalone-runtime"
	want, err := runtime.NewGovernanceService(admin, tenantID).ConfirmWorkspace(ctx, repoRoot)
	if err != nil {
		t.Fatal(err)
	}

	adminPool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(adminPool.Close)
	runtimeURL := createConversationFormationWorkerRole(t, adminPool, databaseURL)
	runtimePool, err := pgxpool.New(ctx, runtimeURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(runtimePool.Close)
	var canReadMigrations bool
	if err := runtimePool.QueryRow(ctx, `SELECT has_table_privilege(current_user, 'public.goose_db_version', 'SELECT')`).Scan(&canReadMigrations); err != nil {
		t.Fatal(err)
	}
	if canReadMigrations {
		t.Fatal("restricted runtime role can read the migration table")
	}

	store, err := openStandaloneRuntimeStore(ctx, runtimeURL, "test")
	if err != nil {
		t.Fatalf("restricted standalone runtime failed preflight: %v", err)
	}
	t.Cleanup(store.Close)
	got, err := store.ResolveWorkspace(ctx, tenantID, runtime.WorkspaceAnchor{RepoRoot: repoRoot})
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != runtime.ResolutionResolved || got.ContinuityID != want.ContinuityID {
		t.Fatalf("tenant context did not expose the confirmed workspace: got=%#v want=%#v", got, want)
	}
	other, err := store.ResolveWorkspace(ctx, "standalone-runtime-other", runtime.WorkspaceAnchor{RepoRoot: repoRoot})
	if err != nil {
		t.Fatal(err)
	}
	if other.Status != runtime.ResolutionNeedsConfirmation || other.ContinuityID != "" {
		t.Fatalf("standalone runtime leaked a workspace across tenants: %#v", other)
	}
}

func TestStandaloneRuntimeStoreRejectsAdministratorRole(t *testing.T) {
	databaseURL := os.Getenv("VERMORY_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("VERMORY_TEST_DATABASE_URL is not set")
	}
	admin, err := runtime.OpenStore(context.Background(), databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	if err := admin.Migrate(context.Background()); err != nil {
		admin.Close()
		t.Fatal(err)
	}
	admin.Close()

	store, err := openStandaloneRuntimeStore(context.Background(), databaseURL, "test")
	if store != nil {
		store.Close()
		t.Fatal("administrator role opened a standalone runtime store")
	}
	if !errors.Is(err, runtime.ErrUnsafeRuntimeRole) {
		t.Fatalf("administrator role was not rejected as unsafe: %v", err)
	}
}
