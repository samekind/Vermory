package webchat

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"vermory/internal/authn"
	"vermory/internal/runtime"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestI01AuthenticatedTenantIsolationAndLifecycle(t *testing.T) {
	ctx := context.Background()
	_, adminPool, databaseURL := openAuthenticatedAcceptanceAdmin(t)
	roleName, runtimeURL := createAuthenticatedRuntimeRole(t, adminPool, databaseURL)
	if err := authn.GrantRuntimeRole(ctx, adminPool, roleName); err != nil {
		t.Fatal(err)
	}
	runtimeStore, err := runtime.OpenStoreWithOptions(ctx, runtimeURL, runtime.StoreOptions{EnforceTenantContext: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(runtimeStore.Close)
	if err := runtimeStore.ValidateRuntimeRole(ctx); err != nil {
		t.Fatal(err)
	}
	authPool, err := pgxpool.New(ctx, runtimeURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(authPool.Close)
	handler := NewAuthenticatedHandler(runtimeStore, nil, "", authn.NewPostgresAuthenticator(authPool))

	clientA := issueAcceptanceToken(t, adminPool, "identity-a", "alice-client", authn.RoleClient, "i01-client-a")
	operatorA := issueAcceptanceToken(t, adminPool, "identity-a", "alice-operator", authn.RoleOperator, "i01-operator-a")
	operatorB := issueAcceptanceToken(t, adminPool, "identity-b", "bob-operator", authn.RoleOperator, "i01-operator-b")
	expired := issueAcceptanceToken(t, adminPool, "identity-a", "alice-expired", authn.RoleClient, "i01-expired")
	if _, err := adminPool.Exec(ctx, `
UPDATE vermory_auth.api_tokens
SET created_at = now() - interval '2 hours', expires_at = now() - interval '1 second'
WHERE public_id = $1`, expired.publicID); err != nil {
		t.Fatal(err)
	}

	preparedA := authenticatedRequest(t, handler, clientA.raw, http.MethodPost, "/v1/integrations/openclaw/turns/prepare", `{
  "operation_id":"i01-a-initial",
  "session_key":"agent:main:shared-anchor",
  "message":"Record the current governed fact."
}`)
	if preparedA.Code != http.StatusOK {
		t.Fatalf("tenant A prepare failed: %d %s", preparedA.Code, preparedA.Body.String())
	}
	var firstA runtime.PreparedConversationTurn
	decodeResponse(t, preparedA, &firstA)
	completedA := authenticatedRequest(t, handler, clientA.raw, http.MethodPost, "/v1/integrations/openclaw/turns/complete", `{
  "operation_id":"i01-a-initial",
  "session_key":"agent:main:shared-anchor",
  "answer":"TENANT-A-ONLY",
  "model":"acceptance/external"
}`)
	if completedA.Code != http.StatusOK {
		t.Fatalf("tenant A completion failed: %d %s", completedA.Code, completedA.Body.String())
	}
	var completed runtime.ChatTurnReceipt
	decodeResponse(t, completedA, &completed)

	clientDenied := authenticatedRequest(t, handler, clientA.raw, http.MethodPost, "/v1/memories/confirm", `{
  "operation_id":"i01-client-confirm-denied",
  "channel":"openclaw",
  "thread_id":"agent:main:shared-anchor",
  "observation_id":"`+completed.AssistantObservationID+`"
}`)
	if clientDenied.Code != http.StatusForbidden {
		t.Fatalf("client governance returned %d: %s", clientDenied.Code, clientDenied.Body.String())
	}

	confirmedResponse := authenticatedRequest(t, handler, operatorA.raw, http.MethodPost, "/v1/memories/confirm", `{
  "operation_id":"i01-operator-confirm-a",
  "channel":"openclaw",
  "thread_id":"agent:main:shared-anchor",
  "observation_id":"`+completed.AssistantObservationID+`"
}`)
	if confirmedResponse.Code != http.StatusOK {
		t.Fatalf("operator confirmation failed: %d %s", confirmedResponse.Code, confirmedResponse.Body.String())
	}
	var confirmed runtime.MemoryReceipt
	decodeResponse(t, confirmedResponse, &confirmed)

	preparedA2 := prepareAcceptanceTurn(t, handler, clientA.raw, "i01-a-recall")
	if !strings.Contains(preparedA2.Context, "TENANT-A-ONLY") {
		t.Fatalf("tenant A did not receive governed memory: %q", preparedA2.Context)
	}
	preparedB := prepareAcceptanceTurn(t, handler, operatorB.raw, "i01-b-isolated")
	if strings.Contains(preparedB.Context, "TENANT-A-ONLY") {
		t.Fatalf("tenant B received tenant A memory: %q", preparedB.Context)
	}
	if preparedB.ContinuityID == firstA.ContinuityID {
		t.Fatal("same anchor resolved to one continuity across tenants")
	}

	crossTenant := authenticatedRequest(t, handler, operatorB.raw, http.MethodPost, "/v1/memories/correct", `{
  "operation_id":"i01-b-cross-tenant-correct",
  "channel":"openclaw",
  "thread_id":"agent:main:shared-anchor",
  "memory_id":"`+confirmed.MemoryID+`",
  "content":"attacker replacement"
}`)
	if crossTenant.Code != http.StatusBadRequest && crossTenant.Code != http.StatusNotFound {
		t.Fatalf("cross-tenant correction returned %d: %s", crossTenant.Code, crossTenant.Body.String())
	}
	if strings.Contains(crossTenant.Body.String(), confirmed.MemoryID) || strings.Contains(crossTenant.Body.String(), "identity-a") {
		t.Fatalf("cross-tenant response leaked resource authority: %s", crossTenant.Body.String())
	}

	authority := authenticatedRequest(t, handler, operatorB.raw, http.MethodPost, "/v1/integrations/openclaw/turns/prepare", `{
  "operation_id":"i01-request-authority",
  "session_key":"agent:main:shared-anchor",
  "message":"attack",
  "tenant_id":"identity-a"
}`)
	if authority.Code != http.StatusBadRequest {
		t.Fatalf("request-owned tenant was accepted: %d %s", authority.Code, authority.Body.String())
	}

	assertAuthenticatedPoolReuse(t, handler, clientA.raw, operatorB.raw)
	assertDirectRLSAndForeignKeyBoundary(t, runtimeURL, firstA.ContinuityID)

	expiredResponse := authenticatedRequest(t, handler, expired.raw, http.MethodPost, "/v1/integrations/openclaw/turns/prepare", `{
  "operation_id":"i01-expired-request",
  "session_key":"agent:main:shared-anchor",
  "message":"must fail"
}`)
	if expiredResponse.Code != http.StatusUnauthorized {
		t.Fatalf("expired token returned %d: %s", expiredResponse.Code, expiredResponse.Body.String())
	}
	if _, err := authn.RevokeToken(ctx, adminPool, authn.RevokeTokenRequest{OperationID: "i01-revoke-client-a", PublicID: clientA.publicID}); err != nil {
		t.Fatal(err)
	}
	revokedResponse := authenticatedRequest(t, handler, clientA.raw, http.MethodPost, "/v1/integrations/openclaw/turns/prepare", `{
  "operation_id":"i01-revoked-request",
  "session_key":"agent:main:shared-anchor",
  "message":"must fail"
}`)
	if revokedResponse.Code != http.StatusUnauthorized {
		t.Fatalf("revoked token returned %d: %s", revokedResponse.Code, revokedResponse.Body.String())
	}

	var activeA, activeB int
	if err := adminPool.QueryRow(ctx, `SELECT count(*) FROM governed_memories WHERE tenant_id = 'identity-a' AND lifecycle_status = 'active'`).Scan(&activeA); err != nil {
		t.Fatal(err)
	}
	if err := adminPool.QueryRow(ctx, `SELECT count(*) FROM governed_memories WHERE tenant_id = 'identity-b' AND lifecycle_status = 'active'`).Scan(&activeB); err != nil {
		t.Fatal(err)
	}
	if activeA != 1 || activeB != 0 {
		t.Fatalf("unexpected governed memory counts: A=%d B=%d", activeA, activeB)
	}
}

type acceptanceToken struct {
	raw      string
	publicID string
}

func openAuthenticatedAcceptanceAdmin(t *testing.T) (*runtime.Store, *pgxpool.Pool, string) {
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
	return store, pool, databaseURL
}

func createAuthenticatedRuntimeRole(t *testing.T, pool *pgxpool.Pool, databaseURL string) (string, string) {
	t.Helper()
	roleName := "vermory_i01_" + strings.ReplaceAll(time.Now().UTC().Format("150405.000000000"), ".", "")
	roleSQL := pgx.Identifier{roleName}.Sanitize()
	if _, err := pool.Exec(context.Background(), "CREATE ROLE "+roleSQL+" LOGIN PASSWORD 'vermory-i01-test-only' NOSUPERUSER NOBYPASSRLS"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DROP OWNED BY "+roleSQL)
		_, _ = pool.Exec(context.Background(), "DROP ROLE IF EXISTS "+roleSQL)
	})
	parsed, err := url.Parse(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	parsed.User = url.UserPassword(roleName, "vermory-i01-test-only")
	query := parsed.Query()
	query.Set("pool_max_conns", "2")
	parsed.RawQuery = query.Encode()
	return roleName, parsed.String()
}

func issueAcceptanceToken(t *testing.T, pool *pgxpool.Pool, tenantID, subjectID string, role authn.Role, operationID string) acceptanceToken {
	t.Helper()
	receipt, err := authn.IssueToken(context.Background(), pool, authn.IssueTokenRequest{
		OperationID: operationID,
		TenantID:    tenantID,
		SubjectID:   subjectID,
		Role:        role,
		ExpiresAt:   time.Now().UTC().Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	return acceptanceToken{raw: receipt.Token.Reveal(), publicID: receipt.Token.PublicID()}
}

func authenticatedRequest(t *testing.T, handler http.Handler, token, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+token)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func prepareAcceptanceTurn(t *testing.T, handler http.Handler, token, operationID string) runtime.PreparedConversationTurn {
	t.Helper()
	response := authenticatedRequest(t, handler, token, http.MethodPost, "/v1/integrations/openclaw/turns/prepare", `{
  "operation_id":"`+operationID+`",
  "session_key":"agent:main:shared-anchor",
  "message":"Recall TENANT-A-ONLY."
}`)
	if response.Code != http.StatusOK {
		t.Fatalf("prepare %s returned %d: %s", operationID, response.Code, response.Body.String())
	}
	var prepared runtime.PreparedConversationTurn
	if err := json.Unmarshal(response.Body.Bytes(), &prepared); err != nil {
		t.Fatal(err)
	}
	return prepared
}

func assertAuthenticatedPoolReuse(t *testing.T, handler http.Handler, tokenA, tokenB string) {
	t.Helper()
	var wait sync.WaitGroup
	errorsByTenant := make(chan error, 24)
	for index := range 12 {
		for _, tenant := range []struct {
			name  string
			token string
			want  bool
		}{{name: "a", token: tokenA, want: true}, {name: "b", token: tokenB, want: false}} {
			index, tenant := index, tenant
			wait.Add(1)
			go func() {
				defer wait.Done()
				operationID := fmt.Sprintf("i01-pool-%s-%02d", tenant.name, index)
				response := authenticatedRequest(t, handler, tenant.token, http.MethodPost, "/v1/integrations/openclaw/turns/prepare", `{
  "operation_id":"`+operationID+`",
  "session_key":"agent:main:shared-anchor",
  "message":"Recall TENANT-A-ONLY."
}`)
				if response.Code != http.StatusOK {
					errorsByTenant <- fmt.Errorf("%s returned %d", operationID, response.Code)
					return
				}
				var prepared runtime.PreparedConversationTurn
				if err := json.Unmarshal(response.Body.Bytes(), &prepared); err != nil {
					errorsByTenant <- err
					return
				}
				hasFact := strings.Contains(prepared.Context, "TENANT-A-ONLY")
				if hasFact != tenant.want {
					errorsByTenant <- fmt.Errorf("%s fact visibility=%v want=%v", operationID, hasFact, tenant.want)
				}
			}()
		}
	}
	wait.Wait()
	close(errorsByTenant)
	for err := range errorsByTenant {
		t.Fatal(err)
	}
}

func assertDirectRLSAndForeignKeyBoundary(t *testing.T, runtimeURL, tenantAContinuityID string) {
	t.Helper()
	ctx := context.Background()
	parsed, err := url.Parse(runtimeURL)
	if err != nil {
		t.Fatal(err)
	}
	query := parsed.Query()
	query.Del("pool_max_conns")
	parsed.RawQuery = query.Encode()
	conn, err := pgx.Connect(ctx, parsed.String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	if _, err := conn.Exec(ctx, `SELECT set_config('vermory.tenant_id', '', false)`); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := conn.QueryRow(ctx, `SELECT count(*) FROM continuity_spaces`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("missing tenant context exposed %d rows", count)
	}
	for _, tenantID := range []string{"identity-a", "identity-b"} {
		if _, err := conn.Exec(ctx, `SELECT set_config('vermory.tenant_id', $1, false)`, tenantID); err != nil {
			t.Fatal(err)
		}
		var visible string
		if err := conn.QueryRow(ctx, `SELECT string_agg(DISTINCT tenant_id, ',') FROM continuity_spaces`).Scan(&visible); err != nil {
			t.Fatal(err)
		}
		if visible != tenantID {
			t.Fatalf("filter omission under %s saw %q", tenantID, visible)
		}
	}
	if _, err := conn.Exec(ctx, `SELECT set_config('vermory.tenant_id', 'identity-b', false)`); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(ctx, `
INSERT INTO continuity_bindings (continuity_id, tenant_id, repo_root, binding_state)
VALUES ($1::uuid, 'identity-b', '/i01/cross-tenant-attack', 'ambiguous')`, tenantAContinuityID); err == nil {
		t.Fatal("cross-tenant foreign key attack was accepted")
	}
}
