package authn

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"vermory/internal/runtime"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPostgresTokenLifecycleStoresOnlyDigestAndReplaysSafely(t *testing.T) {
	pool := openAuthTestPool(t)
	ctx := context.Background()
	request := validIssueRequest("identity-a", "alice-client", "issue-lifecycle")

	first, err := IssueToken(ctx, pool, request)
	if err != nil {
		t.Fatal(err)
	}
	if first.Replayed || first.Token.Reveal() == "" || first.Inspection.TenantID != request.TenantID {
		t.Fatalf("unexpected issue receipt: %#v", first)
	}
	secret, err := base64.RawURLEncoding.DecodeString(first.Token.secret)
	if err != nil {
		t.Fatal(err)
	}

	var storedDigest []byte
	var storedPublicID string
	if err := pool.QueryRow(ctx, `
SELECT token_digest, public_id
FROM vermory_auth.api_tokens
WHERE public_id = $1`, first.Token.PublicID()).Scan(&storedDigest, &storedPublicID); err != nil {
		t.Fatal(err)
	}
	digest := first.Token.Digest()
	if storedPublicID != first.Token.PublicID() || !bytes.Equal(storedDigest, digest[:]) {
		t.Fatalf("stored token material mismatch: public_id=%q digest=%x", storedPublicID, storedDigest)
	}
	if bytes.Equal(storedDigest, secret) {
		t.Fatal("stored digest unexpectedly contains raw token secret")
	}

	replay, err := IssueToken(ctx, pool, request)
	if err != nil {
		t.Fatal(err)
	}
	if !replay.Replayed || replay.Token.Reveal() != "" || replay.Inspection.TokenID != first.Inspection.TokenID {
		t.Fatalf("issue replay exposed or changed token: first=%#v replay=%#v", first, replay)
	}

	conflict := request
	conflict.SubjectID = "different-subject"
	if _, err := IssueToken(ctx, pool, conflict); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("expected idempotency conflict, got %v", err)
	}
}

func TestPostgresAuthenticatorEnforcesExpiryAndRevocation(t *testing.T) {
	pool := openAuthTestPool(t)
	ctx := context.Background()
	authenticator := NewPostgresAuthenticator(pool)

	request := validIssueRequest("identity-a", "alice-client", "issue-auth")
	issued, err := IssueToken(ctx, pool, request)
	if err != nil {
		t.Fatal(err)
	}
	principal, err := authenticator.Authenticate(ctx, issued.Token.Reveal())
	if err != nil {
		t.Fatal(err)
	}
	if principal.TenantID != request.TenantID || principal.SubjectID != request.SubjectID || principal.Role != request.Role {
		t.Fatalf("unexpected principal: %#v", principal)
	}

	if _, err := authenticator.Authenticate(ctx, issued.Token.Reveal()+"x"); !errors.Is(err, ErrAuthenticationFailed) {
		t.Fatalf("malformed token should fail safely: %v", err)
	}

	if _, err := pool.Exec(ctx, `
UPDATE vermory_auth.api_tokens
SET created_at = now() - interval '2 hours', expires_at = now() - interval '1 second'
WHERE public_id = $1`, issued.Token.PublicID()); err != nil {
		t.Fatal(err)
	}
	if _, err := authenticator.Authenticate(ctx, issued.Token.Reveal()); !errors.Is(err, ErrAuthenticationFailed) {
		t.Fatalf("expired token should fail authentication: %v", err)
	}

	validAgain := validIssueRequest("identity-a", "alice-client", "issue-revoke")
	revocable, err := IssueToken(ctx, pool, validAgain)
	if err != nil {
		t.Fatal(err)
	}
	revoked, err := RevokeToken(ctx, pool, RevokeTokenRequest{OperationID: "revoke-1", PublicID: revocable.Token.PublicID()})
	if err != nil {
		t.Fatal(err)
	}
	if revoked.Status != TokenStatusRevoked || revoked.RevokedAt == nil {
		t.Fatalf("unexpected revoke inspection: %#v", revoked)
	}
	if _, err := authenticator.Authenticate(ctx, revocable.Token.Reveal()); !errors.Is(err, ErrAuthenticationFailed) {
		t.Fatalf("revoked token should fail authentication: %v", err)
	}
	revokeReplay, err := RevokeToken(ctx, pool, RevokeTokenRequest{OperationID: "revoke-1", PublicID: revocable.Token.PublicID()})
	if err != nil || !revokeReplay.Replayed {
		t.Fatalf("revoke replay mismatch: inspection=%#v err=%v", revokeReplay, err)
	}
	if _, err := RevokeToken(ctx, pool, RevokeTokenRequest{OperationID: "revoke-2", PublicID: revocable.Token.PublicID()}); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("conflicting revoke should fail safely: %v", err)
	}
}

func TestRuntimeRoleCanLookupButCannotReadAuthOrLegacyTables(t *testing.T) {
	pool := openAuthTestPool(t)
	ctx := context.Background()
	roleName := createRuntimeTestRole(t, pool)
	if err := GrantRuntimeRole(ctx, pool, roleName); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{
		"source_match_decisions",
		"source_formation_runs",
		"source_formation_items",
		"conversation_formation_schedules",
		"conversation_tool_results",
		"memory_vector_documents_2560",
	} {
		var canUseAudit bool
		if err := pool.QueryRow(ctx, `SELECT has_table_privilege($1, 'public.' || $2, 'SELECT,INSERT,UPDATE,DELETE')`, roleName, table).Scan(&canUseAudit); err != nil {
			t.Fatal(err)
		}
		if !canUseAudit {
			t.Fatalf("runtime role cannot use %s", table)
		}
	}
	var canReadRetention, canMutateRetention, canUsePruneRuns bool
	if err := pool.QueryRow(ctx, `
SELECT has_table_privilege($1, 'public.memory_projection_retention', 'SELECT'),
       has_table_privilege($1, 'public.memory_projection_retention', 'INSERT,UPDATE,DELETE'),
       has_table_privilege($1, 'public.memory_projection_prune_runs', 'SELECT,INSERT,UPDATE,DELETE')`,
		roleName,
	).Scan(&canReadRetention, &canMutateRetention, &canUsePruneRuns); err != nil {
		t.Fatal(err)
	}
	if !canReadRetention || canMutateRetention || canUsePruneRuns {
		t.Fatalf("runtime retention privileges read/mutate/prune=%v/%v/%v", canReadRetention, canMutateRetention, canUsePruneRuns)
	}
	var canInspectSchema, canReadMigrationTable, canMutateMigrationTable bool
	if err := pool.QueryRow(ctx, `
SELECT has_function_privilege($1, 'vermory_auth.schema_version()', 'EXECUTE'),
       has_table_privilege($1, 'public.goose_db_version', 'SELECT'),
       has_table_privilege($1, 'public.goose_db_version', 'INSERT,UPDATE,DELETE')`, roleName).Scan(
		&canInspectSchema,
		&canReadMigrationTable,
		&canMutateMigrationTable,
	); err != nil {
		t.Fatal(err)
	}
	if !canInspectSchema || canReadMigrationTable || canMutateMigrationTable {
		t.Fatalf("runtime schema privileges execute/read/mutate=%v/%v/%v", canInspectSchema, canReadMigrationTable, canMutateMigrationTable)
	}

	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = conn.Exec(context.Background(), "RESET ROLE")
		conn.Release()
	})
	if _, err := conn.Exec(ctx, "SET ROLE "+quoteIdentifier(roleName)); err != nil {
		t.Fatal(err)
	}
	var lookupCount int
	if err := conn.QueryRow(ctx, `
SELECT count(*) FROM vermory_auth.authenticate_token($1, $2)`, "does-not-exist", make([]byte, 32)).Scan(&lookupCount); err != nil {
		t.Fatalf("runtime role cannot execute token lookup: %v", err)
	}
	if err := conn.QueryRow(ctx, `SELECT count(*) FROM memory_projection_retention`).Scan(&lookupCount); err != nil {
		t.Fatalf("runtime role cannot read tenant-filtered retention floor: %v", err)
	}
	var schemaVersion int64
	if err := conn.QueryRow(ctx, `SELECT vermory_auth.schema_version()`).Scan(&schemaVersion); err != nil {
		t.Fatalf("runtime role cannot inspect bounded schema version: %v", err)
	}
	if schemaVersion != runtime.MaximumSupportedSchemaVersion {
		t.Fatalf("runtime schema version mismatch: got %d want %d", schemaVersion, runtime.MaximumSupportedSchemaVersion)
	}
	for _, table := range []string{"vermory_auth.api_tokens", "goose_db_version", "projects", "sources", "claims", "capsules", "packets", "wcef_runs", "memory_projection_prune_runs"} {
		var ignored int
		err := conn.QueryRow(ctx, "SELECT count(*) FROM "+table).Scan(&ignored)
		if err == nil {
			t.Fatalf("runtime role unexpectedly read %s", table)
		}
	}
}

func openAuthTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	databaseURL := os.Getenv("VERMORY_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("VERMORY_TEST_DATABASE_URL is not set")
	}
	store, err := runtime.OpenStore(context.Background(), databaseURL)
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
	pool, err := pgxpool.New(context.Background(), databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func validIssueRequest(tenantID, subjectID, operationID string) IssueTokenRequest {
	return IssueTokenRequest{
		OperationID: operationID,
		TenantID:    tenantID,
		SubjectID:   subjectID,
		Role:        RoleClient,
		ExpiresAt:   time.Now().UTC().Add(time.Hour),
	}
}

func createRuntimeTestRole(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	roleName := "vermory_runtime_test_" + time.Now().UTC().Format("150405.000000000")
	roleName = strings.ReplaceAll(roleName, ".", "")
	if _, err := pool.Exec(context.Background(), "CREATE ROLE "+quoteIdentifier(roleName)+" LOGIN NOSUPERUSER NOBYPASSRLS"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DROP OWNED BY "+quoteIdentifier(roleName))
		_, _ = pool.Exec(context.Background(), "DROP ROLE IF EXISTS "+quoteIdentifier(roleName))
	})
	return roleName
}

func quoteIdentifier(identifier string) string {
	return pgx.Identifier{identifier}.Sanitize()
}
