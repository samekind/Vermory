package runtime

import (
	"context"
	"strings"
	"testing"
)

func TestVerifiedToolOutcomeMigrationCreatesTenantScopedMetadata(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()

	var columns []string
	if err := store.pool.QueryRow(ctx, `
SELECT COALESCE(array_agg(column_name ORDER BY ordinal_position), ARRAY[]::text[])
FROM information_schema.columns
WHERE table_schema = 'public' AND table_name = 'conversation_tool_results'`).Scan(&columns); err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		"id", "tenant_id", "continuity_id", "turn_id", "observation_id", "run_id",
		"tool_name", "tool_call_id", "content_sha256", "created_at",
	} {
		if !containsString(columns, expected) {
			t.Fatalf("conversation_tool_results column %q is missing: %#v", expected, columns)
		}
	}

	var rlsEnabled bool
	var policyCount int
	if err := store.pool.QueryRow(ctx, `
SELECT relrowsecurity, (SELECT count(*) FROM pg_policy WHERE polrelid = c.oid)
FROM pg_class c WHERE c.oid = 'public.conversation_tool_results'::regclass`).Scan(&rlsEnabled, &policyCount); err != nil {
		t.Fatal(err)
	}
	if !rlsEnabled || policyCount != 1 {
		t.Fatalf("conversation_tool_results RLS mismatch: enabled=%v policies=%d", rlsEnabled, policyCount)
	}

	for _, constraint := range []string{
		"conversation_tool_results_tenant_continuity_fk",
		"conversation_tool_results_tenant_turn_fk",
		"conversation_tool_results_tenant_observation_fk",
	} {
		var validated bool
		var definition string
		if err := store.pool.QueryRow(ctx, `
SELECT convalidated, pg_get_constraintdef(oid) FROM pg_constraint WHERE conname = $1`, constraint).Scan(&validated, &definition); err != nil {
			t.Fatalf("inspect %s: %v", constraint, err)
		}
		if !validated || !strings.Contains(definition, "tenant_id") || !strings.Contains(definition, "continuity_id") {
			t.Fatalf("constraint %s is not tenant-and-continuity scoped: %q", constraint, definition)
		}
	}

	var observationCheck string
	if err := store.pool.QueryRow(ctx, `
SELECT pg_get_constraintdef(oid)
FROM pg_constraint
WHERE conrelid = 'public.observations'::regclass
  AND conname = 'observations_observation_kind_check'`).Scan(&observationCheck); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(observationCheck, "tool_result") {
		t.Fatalf("observation kind check does not include tool_result: %s", observationCheck)
	}
}
