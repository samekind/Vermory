package runtime

import (
	"context"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"vermory/internal/authn"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestTenantPoolFiltersOmittedQueriesAndClearsReusedConnections(t *testing.T) {
	admin, databaseURL := openTenantPoolAdmin(t)
	ctx := context.Background()
	seedTenantContinuity(t, admin.pool, "identity-a")
	seedTenantContinuity(t, admin.pool, "identity-b")

	roleName, runtimeURL := createTenantPoolRole(t, admin.pool, databaseURL, "runtime", "")
	if err := authn.GrantRuntimeRole(ctx, admin.pool, roleName); err != nil {
		t.Fatal(err)
	}
	runtimeStore, err := OpenStoreWithOptions(ctx, runtimeURL, StoreOptions{EnforceTenantContext: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(runtimeStore.Close)
	if err := runtimeStore.ValidateRuntimeRole(ctx); err != nil {
		t.Fatal(err)
	}

	if err := runtimeStore.pool.QueryRow(ctx, `SELECT count(*) FROM continuity_spaces`).Scan(new(int)); err == nil {
		t.Fatal("missing tenant context did not fail closed")
	}
	for _, tenantID := range []string{"identity-a", "identity-b", "identity-a", "identity-b"} {
		tenantCtx, err := withTenantContext(ctx, tenantID)
		if err != nil {
			t.Fatal(err)
		}
		var tenants string
		if err := runtimeStore.pool.QueryRow(tenantCtx, `
SELECT string_agg(DISTINCT tenant_id, ',' ORDER BY tenant_id)
FROM continuity_spaces`).Scan(&tenants); err != nil {
			t.Fatal(err)
		}
		if tenants != tenantID {
			t.Fatalf("tenant %s saw rows for %q", tenantID, tenants)
		}
	}

	var wait sync.WaitGroup
	errorsByTenant := make(chan error, 40)
	for _, tenantID := range []string{"identity-a", "identity-b"} {
		tenantID := tenantID
		wait.Add(1)
		go func() {
			defer wait.Done()
			for range 20 {
				tenantCtx, err := withTenantContext(ctx, tenantID)
				if err != nil {
					errorsByTenant <- err
					return
				}
				var visible string
				if err := runtimeStore.pool.QueryRow(tenantCtx, `SELECT max(tenant_id) FROM continuity_spaces`).Scan(&visible); err != nil {
					errorsByTenant <- err
					return
				}
				if visible != tenantID {
					errorsByTenant <- fmt.Errorf("tenant %s observed %q", tenantID, visible)
					return
				}
			}
		}()
	}
	wait.Wait()
	close(errorsByTenant)
	for err := range errorsByTenant {
		t.Fatal(err)
	}

	resolution, err := runtimeStore.ResolveWorkspace(ctx, "identity-a", WorkspaceAnchor{RepoRoot: "/runtime/method-scoping"})
	if err != nil {
		t.Fatal(err)
	}
	if resolution.Status != ResolutionNeedsConfirmation {
		t.Fatalf("store method did not execute under tenant context: %#v", resolution)
	}
}

func TestTenantPoolRejectsCrossTenantForeignKeys(t *testing.T) {
	admin, databaseURL := openTenantPoolAdmin(t)
	ctx := context.Background()
	graphA := seedTenantGraph(t, admin.pool, "identity-a", "a")
	graphB := seedTenantGraph(t, admin.pool, "identity-b", "b")
	graphBSecond := seedTenantGraph(t, admin.pool, "identity-b", "b-second")

	roleName, runtimeURL := createTenantPoolRole(t, admin.pool, databaseURL, "foreign_key", "")
	if err := authn.GrantRuntimeRole(ctx, admin.pool, roleName); err != nil {
		t.Fatal(err)
	}
	runtimeStore, err := OpenStoreWithOptions(ctx, runtimeURL, StoreOptions{EnforceTenantContext: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(runtimeStore.Close)
	tenantB, err := withTenantContext(ctx, "identity-b")
	if err != nil {
		t.Fatal(err)
	}

	attacks := []struct {
		name string
		sql  string
		args []any
	}{
		{
			name: "continuity",
			sql: `INSERT INTO continuity_bindings (continuity_id, tenant_id, repo_root, binding_state)
              VALUES ($1::uuid, 'identity-b', '/attack/a-continuity', 'ambiguous')`,
			args: []any{graphA.continuityID},
		},
		{
			name: "memory",
			sql: `INSERT INTO memory_search_documents (memory_id, tenant_id, continuity_id, content, search_document)
              VALUES ($1::uuid, 'identity-b', $2::uuid, 'attack', to_tsvector('simple', 'attack'))`,
			args: []any{graphA.memoryID, graphB.continuityID},
		},
		{
			name: "delivery",
			sql: `INSERT INTO conversation_turns (
                tenant_id, continuity_id, operation_id, status, user_observation_id,
                delivery_id, request_fingerprint
              ) VALUES (
                'identity-b', $1::uuid, 'attack-delivery', 'in_progress', $2::uuid,
                $3::uuid, repeat('a', 64)
              )`,
			args: []any{graphB.continuityID, graphB.observationID, graphA.deliveryID},
		},
		{
			name: "bridge",
			sql: `INSERT INTO bridge_events (tenant_id, bridge_id, event_type, operation_id)
              VALUES ('identity-b', $1::uuid, 'created', 'attack-bridge')`,
			args: []any{graphA.bridgeID},
		},
		{
			name: "source match continuity",
			sql: `INSERT INTO source_match_decisions (
                tenant_id, continuity_id, operation_id, request_fingerprint,
                source_ref, source_content, candidate_set, candidate_set_fingerprint,
                provider_name, requested_model, status, selected_memory_key,
                target_memory_id, observation_id, completed_at
              ) VALUES (
                'identity-b', $1::uuid, 'attack-source-match', repeat('a', 64),
                'fixture:attack', 'cross continuity attack', '[]'::jsonb, repeat('b', 64),
                'test-provider', 'test-model', 'matched', 'release.signing.mode',
                $2::uuid, $3::uuid, now()
              )`,
			args: []any{graphB.continuityID, graphBSecond.memoryID, graphB.observationID},
		},
		{
			name: "source formation continuity",
			sql: `INSERT INTO source_formation_runs (
                tenant_id, continuity_id, operation_id, request_fingerprint,
                source_ref, source_sha256, source_bytes, active_snapshot,
                active_snapshot_fingerprint, provider_name, requested_model, status
              ) VALUES (
                'identity-b', $1::uuid, 'attack-source-formation', repeat('a', 64),
                'fixture:attack', repeat('b', 64), 1, '[]'::jsonb,
                repeat('c', 64), 'test-provider', 'test-model', 'pending'
              )`,
			args: []any{graphA.continuityID},
		},
		{
			name: "projection event memory",
			sql: `INSERT INTO memory_projection_events (
                tenant_id, continuity_id, memory_id, desired_state, authority_version
              ) VALUES ('identity-b', $1::uuid, $2::uuid, 'active', now())`,
			args: []any{graphB.continuityID, graphA.memoryID},
		},
		{
			name: "vector document memory",
			sql: `INSERT INTO memory_vector_documents (
                profile_id, tenant_id, continuity_id, memory_id, content_sha256, embedding
              ) VALUES (
                'siliconflow-bge-m3-1024-v1', 'identity-b', $1::uuid, $2::uuid,
                repeat('a', 64), array_fill(0::real, ARRAY[1024])::vector
              )`,
			args: []any{graphB.continuityID, graphA.memoryID},
		},
		{
			name: "retrieval audit continuity",
			sql: `INSERT INTO memory_retrieval_runs (
                tenant_id, primary_continuity_id, continuity_ids, operation_id,
                request_fingerprint, requested_mode, effective_mode, profile_id,
                query_sha256, projection_current, degraded
              ) VALUES (
                'identity-b', $1::uuid, ARRAY[$1::uuid], 'attack-retrieval-run',
                repeat('a', 64), 'vector', 'vector', 'siliconflow-bge-m3-1024-v1',
                repeat('b', 64), true, false
              )`,
			args: []any{graphA.continuityID},
		},
		{
			name: "conversation formation schedule continuity",
			sql: `INSERT INTO conversation_formation_schedules (
                tenant_id, continuity_id, requested_through_sequence,
                processed_through_sequence, schedule_state
              ) VALUES ('identity-b', $1::uuid, 0, 0, 'idle')`,
			args: []any{graphA.continuityID},
		},
	}
	for _, attack := range attacks {
		if _, err := runtimeStore.pool.Exec(tenantB, attack.sql, attack.args...); err == nil {
			t.Fatalf("cross-tenant %s reference was accepted", attack.name)
		}
	}
}

func TestRuntimeRoleValidationRejectsUnsafeIdentities(t *testing.T) {
	admin, databaseURL := openTenantPoolAdmin(t)
	ctx := context.Background()
	if err := admin.ValidateRuntimeRole(ctx); err == nil {
		t.Fatal("admin/table-owner identity passed runtime validation")
	}
	if err := admin.ValidateProjectionPruneOperatorRole(ctx); err != nil {
		t.Fatalf("admin/table-owner identity failed prune validation: %v", err)
	}

	runtimeRole, runtimeURL := createTenantPoolRole(t, admin.pool, databaseURL, "valid", "")
	if err := authn.GrantRuntimeRole(ctx, admin.pool, runtimeRole); err != nil {
		t.Fatal(err)
	}
	runtimeStore, err := OpenStoreWithOptions(ctx, runtimeURL, StoreOptions{EnforceTenantContext: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(runtimeStore.Close)
	if err := runtimeStore.ValidateRuntimeRole(ctx); err != nil {
		t.Fatalf("restricted runtime role was rejected: %v", err)
	}
	if err := runtimeStore.ValidateProjectionPruneOperatorRole(ctx); !errors.Is(err, ErrUnsafePruneRole) {
		t.Fatalf("restricted runtime role passed prune validation: %v", err)
	}
	if _, err := admin.pool.Exec(ctx, "REVOKE SELECT ON public.memory_projection_retention FROM "+pgx.Identifier{runtimeRole}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	if err := runtimeStore.ValidateRuntimeRole(ctx); err == nil {
		t.Fatal("runtime identity without retention floor read access passed validation")
	}
	if err := authn.GrantRuntimeRole(ctx, admin.pool, runtimeRole); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.pool.Exec(ctx, "GRANT UPDATE ON public.memory_projection_retention TO "+pgx.Identifier{runtimeRole}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	if err := runtimeStore.ValidateRuntimeRole(ctx); err == nil {
		t.Fatal("runtime identity with retention floor mutation passed validation")
	}
	if err := authn.GrantRuntimeRole(ctx, admin.pool, runtimeRole); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.pool.Exec(ctx, "GRANT SELECT ON public.memory_projection_prune_runs TO "+pgx.Identifier{runtimeRole}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	if err := runtimeStore.ValidateRuntimeRole(ctx); err == nil {
		t.Fatal("runtime identity with prune receipt access passed validation")
	}
	if err := authn.GrantRuntimeRole(ctx, admin.pool, runtimeRole); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.pool.Exec(ctx, "REVOKE ALL ON public.source_match_decisions FROM "+pgx.Identifier{runtimeRole}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	if err := runtimeStore.ValidateRuntimeRole(ctx); err == nil {
		t.Fatal("runtime identity without source_match_decisions access passed validation")
	}
	if err := authn.GrantRuntimeRole(ctx, admin.pool, runtimeRole); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.pool.Exec(ctx, "REVOKE ALL ON public.source_formation_items FROM "+pgx.Identifier{runtimeRole}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	if err := runtimeStore.ValidateRuntimeRole(ctx); err == nil {
		t.Fatal("runtime identity without source_formation_items access passed validation")
	}
	if err := authn.GrantRuntimeRole(ctx, admin.pool, runtimeRole); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.pool.Exec(ctx, "REVOKE ALL ON public.memory_vector_documents FROM "+pgx.Identifier{runtimeRole}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	if err := runtimeStore.ValidateRuntimeRole(ctx); err == nil {
		t.Fatal("runtime identity without memory_vector_documents access passed validation")
	}
	if err := authn.GrantRuntimeRole(ctx, admin.pool, runtimeRole); err != nil {
		t.Fatal(err)
	}

	_, bypassURL := createTenantPoolRole(t, admin.pool, databaseURL, "bypass", "BYPASSRLS")
	bypassStore, err := OpenStore(ctx, bypassURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(bypassStore.Close)
	if err := bypassStore.ValidateRuntimeRole(ctx); err == nil {
		t.Fatal("BYPASSRLS identity passed runtime validation")
	}

	unprovisionedRole, unprovisionedURL := createTenantPoolRole(t, admin.pool, databaseURL, "unprovisioned", "")
	if _, err := admin.pool.Exec(ctx, "GRANT USAGE ON SCHEMA public, vermory_auth TO "+pgx.Identifier{unprovisionedRole}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	unprovisionedStore, err := OpenStore(ctx, unprovisionedURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(unprovisionedStore.Close)
	if err := unprovisionedStore.ValidateRuntimeRole(ctx); err == nil {
		t.Fatal("unprovisioned runtime identity passed startup validation")
	}

	leakyRole, leakyURL := createTenantPoolRole(t, admin.pool, databaseURL, "legacy_access", "")
	if _, err := admin.pool.Exec(ctx, "GRANT SELECT ON public.projects TO "+pgx.Identifier{leakyRole}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	leakyStore, err := OpenStore(ctx, leakyURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(leakyStore.Close)
	if err := leakyStore.ValidateRuntimeRole(ctx); err == nil {
		t.Fatal("identity with legacy table access passed runtime validation")
	}

	ownerRole, ownerURL := createTenantPoolRole(t, admin.pool, databaseURL, "owner", "")
	var originalOwner string
	if err := admin.pool.QueryRow(ctx, `
SELECT pg_get_userbyid(relowner)
FROM pg_class
WHERE oid = 'public.continuity_spaces'::regclass`).Scan(&originalOwner); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.pool.Exec(ctx, "ALTER TABLE public.continuity_spaces OWNER TO "+pgx.Identifier{ownerRole}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = admin.pool.Exec(context.Background(), "ALTER TABLE public.continuity_spaces OWNER TO "+pgx.Identifier{originalOwner}.Sanitize())
	})
	ownerStore, err := OpenStore(ctx, ownerURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(ownerStore.Close)
	if err := ownerStore.ValidateRuntimeRole(ctx); err == nil {
		t.Fatal("served-table owner identity passed runtime validation")
	}
}

func TestTenantPoolAllTenantStoreMethodsAttachContext(t *testing.T) {
	files, err := filepath.Glob("*_store.go")
	if err != nil {
		t.Fatal(err)
	}
	fileSet := token.NewFileSet()
	for _, path := range files {
		parsed, err := parser.ParseFile(fileSet, path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, declaration := range parsed.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Recv == nil || function.Body == nil || !ast.IsExported(function.Name.Name) || !hasNamedParameter(function, "tenantID") {
				continue
			}
			attached := false
			ast.Inspect(function.Body, func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if !ok {
					return true
				}
				identifier, ok := call.Fun.(*ast.Ident)
				if ok && identifier.Name == "withTenantContext" {
					attached = true
					return false
				}
				return true
			})
			if !attached {
				t.Errorf("%s.%s accepts tenantID without attaching runtime tenant context", path, function.Name.Name)
			}
		}
	}
}

type tenantGraph struct {
	continuityID  string
	observationID string
	memoryID      string
	deliveryID    string
	bridgeID      string
}

func openTenantPoolAdmin(t *testing.T) (*Store, string) {
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
	return store, databaseURL
}

func seedTenantContinuity(t *testing.T, pool *pgxpool.Pool, tenantID string) string {
	t.Helper()
	var continuityID string
	if err := pool.QueryRow(context.Background(), `
INSERT INTO continuity_spaces (tenant_id, continuity_line, state)
VALUES ($1, 'workspace', 'active')
RETURNING id::text`, tenantID).Scan(&continuityID); err != nil {
		t.Fatal(err)
	}
	return continuityID
}

func seedTenantGraph(t *testing.T, pool *pgxpool.Pool, tenantID, suffix string) tenantGraph {
	t.Helper()
	ctx := context.Background()
	graph := tenantGraph{continuityID: seedTenantContinuity(t, pool, tenantID)}
	if err := pool.QueryRow(ctx, `
INSERT INTO observations (tenant_id, continuity_id, operation_id, observation_kind, content)
VALUES ($1, $2::uuid, $3, 'source_update', $4)
RETURNING id::text`, tenantID, graph.continuityID, "seed-observation-"+suffix, "seed "+suffix).Scan(&graph.observationID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
INSERT INTO governed_memories (
  tenant_id, continuity_id, origin_observation_id, memory_kind, lifecycle_status, content
)
VALUES ($1, $2::uuid, $3::uuid, 'fact', 'active', $4)
RETURNING id::text`, tenantID, graph.continuityID, graph.observationID, "memory "+suffix).Scan(&graph.memoryID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
INSERT INTO memory_deliveries (tenant_id, continuity_id, operation_id, task, context_body)
VALUES ($1, $2::uuid, $3, 'seed task', 'seed context')
RETURNING id::text`, tenantID, graph.continuityID, "seed-delivery-"+suffix).Scan(&graph.deliveryID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
INSERT INTO bridge_operations (
  tenant_id, operation_id, action, status, source_continuity_id,
  source_anchor, target_profile, title, request_fingerprint
)
VALUES ($1, $2, 'export', 'active', $3::uuid, $4, 'team_handoff', 'seed', $5)
RETURNING id::text`, tenantID, "seed-bridge-"+suffix, graph.continuityID, "/seed/"+suffix, "fingerprint-"+suffix).Scan(&graph.bridgeID); err != nil {
		t.Fatal(err)
	}
	return graph
}

func createTenantPoolRole(t *testing.T, pool *pgxpool.Pool, databaseURL, suffix, attributes string) (string, string) {
	t.Helper()
	roleName := "vermory_pool_" + suffix + "_" + strings.ReplaceAll(time.Now().UTC().Format("150405.000000000"), ".", "")
	roleSQL := pgx.Identifier{roleName}.Sanitize()
	statement := "CREATE ROLE " + roleSQL + " LOGIN PASSWORD 'vermory-test-only' NOSUPERUSER " + attributes
	if _, err := pool.Exec(context.Background(), statement); err != nil {
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
	parsed.User = url.UserPassword(roleName, "vermory-test-only")
	query := parsed.Query()
	query.Set("pool_max_conns", "2")
	parsed.RawQuery = query.Encode()
	return roleName, parsed.String()
}

func hasNamedParameter(function *ast.FuncDecl, name string) bool {
	for _, field := range function.Type.Params.List {
		for _, identifier := range field.Names {
			if identifier.Name == name {
				return true
			}
		}
	}
	return false
}
