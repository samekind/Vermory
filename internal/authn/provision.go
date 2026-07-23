package authn

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var roleIdentifierPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_$]{0,62}$`)

var runtimeReadWriteTables = []string{
	"continuity_spaces",
	"continuity_bindings",
	"conversation_bindings",
	"observations",
	"memory_deliveries",
	"memory_search_documents",
	"conversation_turns",
	"conversation_tool_results",
	"bridge_operations",
	"bridge_events",
	"bridge_memory_effects",
	"conversation_links",
	"source_match_decisions",
	"source_formation_runs",
	"source_formation_items",
	"conversation_formation_schedules",
	"memory_projection_events",
	"memory_projection_cursors",
	"memory_vector_documents",
	"memory_vector_documents_2560",
	"memory_retrieval_runs",
}

var runtimeReadOnlyTables = []string{
	"memory_eligibility_operations",
	"memory_projection_retention",
}

var runtimeGovernedMemoryInsertColumns = []string{
	"tenant_id",
	"continuity_id",
	"origin_observation_id",
	"memory_kind",
	"memory_key",
	"lifecycle_status",
	"content",
	"supersedes_memory_id",
}

var runtimeGovernedMemoryUpdateColumns = []string{
	"lifecycle_status",
	"content",
	"updated_at",
}

var forbiddenRuntimeTables = []string{
	"vermory_auth.api_tokens",
	"public.goose_db_version",
	"public.projects",
	"public.sources",
	"public.source_versions",
	"public.claims",
	"public.capsules",
	"public.capsule_claims",
	"public.packets",
	"public.audit_logs",
	"public.wcef_runs",
	"public.memory_projection_prune_runs",
}

func GrantRuntimeRole(ctx context.Context, pool *pgxpool.Pool, roleName string) error {
	if pool == nil {
		return errors.New("grant runtime role: database pool is required")
	}
	if !roleIdentifierPattern.MatchString(roleName) {
		return invalidRequest("invalid PostgreSQL role identifier")
	}
	var canLogin, superuser, bypassRLS bool
	if err := pool.QueryRow(ctx, `
SELECT rolcanlogin, rolsuper, rolbypassrls
FROM pg_roles
WHERE rolname = $1`, roleName).Scan(&canLogin, &superuser, &bypassRLS); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return invalidRequest("PostgreSQL role does not exist")
		}
		return fmt.Errorf("inspect runtime role: %w", err)
	}
	if !canLogin || superuser || bypassRLS {
		return invalidRequest("PostgreSQL role is not a restricted login role")
	}

	var ownedTables int
	if err := pool.QueryRow(ctx, `
SELECT count(*)
FROM pg_class c
JOIN pg_namespace n ON n.oid = c.relnamespace
JOIN pg_roles r ON r.oid = c.relowner
WHERE r.rolname = $1
  AND n.nspname IN ('public', 'vermory_auth')
  AND c.relkind IN ('r', 'p')`, roleName).Scan(&ownedTables); err != nil {
		return fmt.Errorf("inspect runtime role ownership: %w", err)
	}
	if ownedTables != 0 {
		return invalidRequest("PostgreSQL runtime role owns application tables")
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin runtime role grant: %w", err)
	}
	defer tx.Rollback(ctx)
	roleSQL := pgx.Identifier{roleName}.Sanitize()
	readWriteSQL := make([]string, 0, len(runtimeReadWriteTables))
	for _, table := range runtimeReadWriteTables {
		readWriteSQL = append(readWriteSQL, pgx.Identifier{"public", table}.Sanitize())
	}
	readOnlySQL := make([]string, 0, len(runtimeReadOnlyTables))
	for _, table := range runtimeReadOnlyTables {
		readOnlySQL = append(readOnlySQL, pgx.Identifier{"public", table}.Sanitize())
	}
	forbiddenSQL := make([]string, 0, len(forbiddenRuntimeTables))
	for _, table := range forbiddenRuntimeTables {
		parts := strings.SplitN(table, ".", 2)
		forbiddenSQL = append(forbiddenSQL, pgx.Identifier{parts[0], parts[1]}.Sanitize())
	}
	statements := []string{
		"GRANT USAGE ON SCHEMA public, vermory_auth TO " + roleSQL,
		"GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE " + strings.Join(readWriteSQL, ", ") + " TO " + roleSQL,
		"GRANT SELECT, DELETE ON TABLE public.governed_memories TO " + roleSQL,
		"REVOKE INSERT, UPDATE ON TABLE public.governed_memories FROM " + roleSQL,
		"GRANT INSERT (" + strings.Join(runtimeGovernedMemoryInsertColumns, ", ") + ") ON TABLE public.governed_memories TO " + roleSQL,
		"GRANT UPDATE (" + strings.Join(runtimeGovernedMemoryUpdateColumns, ", ") + ") ON TABLE public.governed_memories TO " + roleSQL,
		"GRANT SELECT ON TABLE " + strings.Join(readOnlySQL, ", ") + " TO " + roleSQL,
		"REVOKE INSERT, UPDATE, DELETE ON TABLE " + strings.Join(readOnlySQL, ", ") + " FROM " + roleSQL,
		"GRANT USAGE, SELECT ON SEQUENCE public.observations_observation_seq_seq TO " + roleSQL,
		"GRANT USAGE, SELECT ON SEQUENCE public.memory_projection_events_event_id_seq TO " + roleSQL,
		"GRANT EXECUTE ON FUNCTION vermory_auth.authenticate_token(TEXT, BYTEA) TO " + roleSQL,
		"GRANT EXECUTE ON FUNCTION vermory_auth.schema_version() TO " + roleSQL,
		"REVOKE ALL PRIVILEGES ON TABLE " + strings.Join(forbiddenSQL, ", ") + " FROM " + roleSQL,
	}
	for _, statement := range statements {
		if _, err := tx.Exec(ctx, statement); err != nil {
			return fmt.Errorf("apply runtime role grant: %w", err)
		}
	}

	for _, table := range runtimeReadOnlyTables {
		var canRead, canMutate bool
		if err := tx.QueryRow(ctx, `
SELECT has_table_privilege($1, 'public.' || $2, 'SELECT'),
       has_table_privilege($1, 'public.' || $2, 'INSERT')
       OR has_table_privilege($1, 'public.' || $2, 'UPDATE')
       OR has_table_privilege($1, 'public.' || $2, 'DELETE')`, roleName, table).Scan(&canRead, &canMutate); err != nil {
			return fmt.Errorf("verify runtime read-only boundary: %w", err)
		}
		if !canRead || canMutate {
			return invalidRequest("PostgreSQL role violates runtime read-only table boundary")
		}
	}
	var governedReadDelete, governedTableInsertUpdate, governedAllowedInsert, governedAllowedUpdate, governedValidityMutation bool
	if err := tx.QueryRow(ctx, `
SELECT
  has_table_privilege($1, 'public.governed_memories', 'SELECT')
    AND has_table_privilege($1, 'public.governed_memories', 'DELETE'),
  has_table_privilege($1, 'public.governed_memories', 'INSERT')
    OR has_table_privilege($1, 'public.governed_memories', 'UPDATE'),
  COALESCE((
    SELECT bool_and(has_column_privilege($1, 'public.governed_memories', column_name, 'INSERT'))
    FROM unnest($2::text[]) AS insert_columns(column_name)
  ), false),
  COALESCE((
    SELECT bool_and(has_column_privilege($1, 'public.governed_memories', column_name, 'UPDATE'))
    FROM unnest($3::text[]) AS update_columns(column_name)
  ), false),
  has_column_privilege($1, 'public.governed_memories', 'valid_from', 'INSERT,UPDATE')
    OR has_column_privilege($1, 'public.governed_memories', 'valid_until', 'INSERT,UPDATE')
`, roleName, runtimeGovernedMemoryInsertColumns, runtimeGovernedMemoryUpdateColumns).Scan(
		&governedReadDelete,
		&governedTableInsertUpdate,
		&governedAllowedInsert,
		&governedAllowedUpdate,
		&governedValidityMutation,
	); err != nil {
		return fmt.Errorf("verify governed memory runtime boundary: %w", err)
	}
	if !governedReadDelete || governedTableInsertUpdate || !governedAllowedInsert || !governedAllowedUpdate || governedValidityMutation {
		return invalidRequest("PostgreSQL role violates governed memory column boundary")
	}
	for _, table := range forbiddenRuntimeTables {
		var canUse bool
		if err := tx.QueryRow(ctx, `
SELECT has_table_privilege($1, $2, 'SELECT')
    OR has_table_privilege($1, $2, 'INSERT')
    OR has_table_privilege($1, $2, 'UPDATE')
    OR has_table_privilege($1, $2, 'DELETE')`, roleName, table).Scan(&canUse); err != nil {
			return fmt.Errorf("verify runtime role boundary: %w", err)
		}
		if canUse {
			return invalidRequest("PostgreSQL role inherits forbidden table access")
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit runtime role grant: %w", err)
	}
	return nil
}
