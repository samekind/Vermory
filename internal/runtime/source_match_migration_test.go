package runtime

import (
	"context"
	"strings"
	"testing"
)

func TestSourceMatchMigrationCreatesGovernedAuditTable(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()

	var tableExists bool
	if err := store.pool.QueryRow(ctx, `
SELECT to_regclass('public.source_match_decisions') IS NOT NULL`).Scan(&tableExists); err != nil {
		t.Fatal(err)
	}
	if !tableExists {
		t.Fatal("source_match_decisions is missing")
	}

	rows, err := store.pool.Query(ctx, `
SELECT column_name
FROM information_schema.columns
WHERE table_schema = 'public' AND table_name = 'source_match_decisions'
ORDER BY ordinal_position`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	columns := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		columns[name] = true
	}
	for _, required := range []string{
		"id", "tenant_id", "continuity_id", "operation_id", "request_fingerprint",
		"source_ref", "source_content", "candidate_set", "candidate_set_fingerprint",
		"provider_name", "requested_model", "resolved_model", "status",
		"provider_output", "provider_artifact_sha256", "reason", "failure_code",
		"selected_memory_key", "target_memory_id", "observation_id",
		"candidate_memory_id", "created_at", "completed_at",
	} {
		if !columns[required] {
			t.Fatalf("source_match_decisions column %q is missing: %#v", required, columns)
		}
	}

	var checks []string
	if err := store.pool.QueryRow(ctx, `
SELECT COALESCE(array_agg(pg_get_constraintdef(oid) ORDER BY conname), ARRAY[]::text[])
FROM pg_constraint
WHERE conrelid = 'public.source_match_decisions'::regclass AND contype = 'c'`).Scan(&checks); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(checks, " ")
	for _, expected := range []string{"pending", "matched", "abstained", "failed", "jsonb_typeof", "64"} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("source match checks do not constrain %q: %s", expected, joined)
		}
	}

	var rlsEnabled bool
	var policyCount int
	if err := store.pool.QueryRow(ctx, `
SELECT c.relrowsecurity,
       (SELECT count(*) FROM pg_policy policy WHERE policy.polrelid = c.oid)
FROM pg_class c
WHERE c.oid = 'public.source_match_decisions'::regclass`).Scan(&rlsEnabled, &policyCount); err != nil {
		t.Fatal(err)
	}
	if !rlsEnabled || policyCount != 1 {
		t.Fatalf("source match audit is not RLS protected: enabled=%v policies=%d", rlsEnabled, policyCount)
	}

	for _, constraint := range []string{
		"source_match_decisions_tenant_continuity_fk",
		"source_match_decisions_tenant_target_memory_fk",
		"source_match_decisions_tenant_observation_fk",
		"source_match_decisions_tenant_candidate_memory_fk",
	} {
		var validated bool
		var definition string
		if err := store.pool.QueryRow(ctx, `
SELECT convalidated, pg_get_constraintdef(oid)
FROM pg_constraint
WHERE conname = $1`, constraint).Scan(&validated, &definition); err != nil {
			t.Fatalf("inspect constraint %s: %v", constraint, err)
		}
		if !validated || !strings.Contains(definition, "tenant_id") {
			t.Fatalf("constraint %s is not tenant-aware: validated=%v definition=%q", constraint, validated, definition)
		}
		if constraint != "source_match_decisions_tenant_continuity_fk" && !strings.Contains(definition, "continuity_id") {
			t.Fatalf("constraint %s is not continuity-aware: %q", constraint, definition)
		}
	}
}

func TestResetForTestClearsSourceMatchAudit(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	continuityID, err := store.ConfirmWorkspaceBinding(ctx, "reset-source-match", "/fixtures/reset-source-match")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.pool.Exec(ctx, `
INSERT INTO source_match_decisions (
  tenant_id, continuity_id, operation_id, request_fingerprint,
  source_ref, source_content, candidate_set, candidate_set_fingerprint,
  provider_name, requested_model, status
) VALUES (
  'reset-source-match', $1::uuid, 'reset-source-match-op', repeat('a', 64),
  'fixture:reset', 'Reset this audit row.', '[]'::jsonb, repeat('b', 64),
  'test-provider', 'test-model', 'pending'
)`, continuityID); err != nil {
		t.Fatal(err)
	}
	if err := store.ResetForTest(ctx); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := store.pool.QueryRow(ctx, `SELECT count(*) FROM source_match_decisions`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("source match audit survived reset: %d", count)
	}
}
