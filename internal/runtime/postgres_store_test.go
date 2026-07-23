package runtime

import (
	"context"
	"fmt"
	"os"
	"testing"
)

func TestStoreMigrateAcceptsPoolConfiguration(t *testing.T) {
	cluster := startDisposablePostgres18(t)
	defer cluster.stop(t, "fast")

	store, err := OpenStore(context.Background(), fmt.Sprintf("%s&pool_max_conns=2", cluster.databaseURL))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	if err := store.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestStoreResolveWorkspaceRequiresConfirmationForUnknownAnchor(t *testing.T) {
	store := openTestStore(t)
	result, err := store.ResolveWorkspace(context.Background(), "local", WorkspaceAnchor{RepoRoot: "/repo/new-workspace"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != ResolutionNeedsConfirmation {
		t.Fatalf("expected needs confirmation, got %#v", result)
	}
	if result.ContinuityID != "" {
		t.Fatalf("unknown workspace must not create a continuity, got %#v", result)
	}
}

func TestStoreWorkspaceAttachmentUsesExactFilesystemNamespaceAndRoot(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	const repoRoot = "/work/Vermory"
	alpha, err := store.ConfirmWorkspaceAnchorBinding(ctx, "local", WorkspaceAnchor{
		RepoRoot: repoRoot, FilesystemNamespace: "workstation-alpha",
	})
	if err != nil {
		t.Fatal(err)
	}
	beta, err := store.ConfirmWorkspaceAnchorBinding(ctx, "local", WorkspaceAnchor{
		RepoRoot: repoRoot, FilesystemNamespace: "workstation-beta",
	})
	if err != nil {
		t.Fatal(err)
	}
	if alpha == beta {
		t.Fatalf("same path in different filesystem namespaces shared continuity: %s", alpha)
	}
	for _, test := range []struct {
		name      string
		anchor    WorkspaceAnchor
		wantID    string
		wantState ResolutionStatus
	}{
		{name: "alpha", anchor: WorkspaceAnchor{RepoRoot: repoRoot, FilesystemNamespace: "workstation-alpha"}, wantID: alpha, wantState: ResolutionResolved},
		{name: "beta", anchor: WorkspaceAnchor{RepoRoot: repoRoot, FilesystemNamespace: "workstation-beta"}, wantID: beta, wantState: ResolutionResolved},
		{name: "unknown namespace", anchor: WorkspaceAnchor{RepoRoot: repoRoot, FilesystemNamespace: "workstation-gamma"}, wantState: ResolutionNeedsConfirmation},
		{name: "unknown path", anchor: WorkspaceAnchor{RepoRoot: "/archive/Vermory", FilesystemNamespace: "workstation-alpha"}, wantState: ResolutionNeedsConfirmation},
		{name: "binding mismatch", anchor: WorkspaceAnchor{RepoRoot: repoRoot, FilesystemNamespace: "workstation-beta", ExplicitBindingID: alpha}, wantState: ResolutionNeedsConfirmation},
	} {
		t.Run(test.name, func(t *testing.T) {
			resolution, err := store.ResolveWorkspace(ctx, "local", test.anchor)
			if err != nil {
				t.Fatal(err)
			}
			if resolution.Status != test.wantState || (test.wantID != "" && resolution.ContinuityID != test.wantID) {
				t.Fatalf("unexpected exact attachment resolution: %#v", resolution)
			}
		})
	}
}

func TestStoreCommitObservationIsIdempotent(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	continuityID, err := store.ConfirmWorkspaceBinding(ctx, "local", "/repo/web-checkout")
	if err != nil {
		t.Fatal(err)
	}
	req := CommitObservationRequest{
		OperationID: "writeback-1",
		Kind:        ObservationKindUserCorrection,
		Content:     "Use checkout_eta_v2.",
	}
	first, err := store.CommitObservation(ctx, "local", continuityID, req)
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.CommitObservation(ctx, "local", continuityID, req)
	if err != nil {
		t.Fatal(err)
	}
	if first.ObservationID != second.ObservationID || !second.Replayed {
		t.Fatalf("expected idempotent receipt, first=%#v second=%#v", first, second)
	}
}

func openTestStore(t *testing.T) *Store {
	t.Helper()
	databaseURL := os.Getenv("VERMORY_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("VERMORY_TEST_DATABASE_URL is not set")
	}
	store, err := OpenStore(context.Background(), databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(store.Close)
	if err := store.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := store.ResetForTest(context.Background()); err != nil {
		t.Fatal(err)
	}
	return store
}
