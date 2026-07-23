package runtime

import (
	"context"
	"strings"
	"testing"
)

func TestSourceFormationMigrationCreatesAuthoritativeTables(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()

	for table, required := range map[string][]string{
		"source_formation_runs": {
			"id", "tenant_id", "continuity_id", "operation_id", "request_fingerprint",
			"source_ref", "source_sha256", "source_bytes", "input_kind", "input_manifest",
			"input_manifest_fingerprint", "active_snapshot",
			"active_snapshot_fingerprint", "provider_name", "requested_model",
			"resolved_model", "status", "provider_output", "provider_artifact_sha256",
			"reason", "failure_code", "created_at", "completed_at",
		},
		"source_formation_items": {
			"id", "tenant_id", "continuity_id", "run_id", "ordinal", "decision",
			"memory_key", "quote", "quote_occurrence", "byte_start", "byte_end",
			"content", "reason", "target_memory_id", "evidence_observation_id", "observation_id",
			"candidate_memory_id", "created_at",
		},
	} {
		var tableExists bool
		if err := store.pool.QueryRow(ctx, `SELECT to_regclass('public.' || $1) IS NOT NULL`, table).Scan(&tableExists); err != nil {
			t.Fatal(err)
		}
		if !tableExists {
			t.Fatalf("%s is missing", table)
		}
		rows, err := store.pool.Query(ctx, `
SELECT column_name
FROM information_schema.columns
WHERE table_schema = 'public' AND table_name = $1
ORDER BY ordinal_position`, table)
		if err != nil {
			t.Fatal(err)
		}
		columns := map[string]bool{}
		for rows.Next() {
			var name string
			if err := rows.Scan(&name); err != nil {
				rows.Close()
				t.Fatal(err)
			}
			columns[name] = true
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		for _, name := range required {
			if !columns[name] {
				t.Fatalf("%s column %q is missing: %#v", table, name, columns)
			}
		}

		var enabled bool
		var policyCount int
		if err := store.pool.QueryRow(ctx, `
SELECT c.relrowsecurity,
       (SELECT count(*) FROM pg_policy policy WHERE policy.polrelid = c.oid)
FROM pg_class c
WHERE c.oid = ('public.' || $1)::regclass`, table).Scan(&enabled, &policyCount); err != nil {
			t.Fatal(err)
		}
		if !enabled || policyCount != 1 {
			t.Fatalf("%s is not RLS protected: enabled=%v policies=%d", table, enabled, policyCount)
		}
	}

	var runChecks, itemChecks []string
	if err := store.pool.QueryRow(ctx, `
SELECT COALESCE(array_agg(pg_get_constraintdef(oid) ORDER BY conname), ARRAY[]::text[])
FROM pg_constraint
WHERE conrelid = 'public.source_formation_runs'::regclass AND contype = 'c'`).Scan(&runChecks); err != nil {
		t.Fatal(err)
	}
	if err := store.pool.QueryRow(ctx, `
SELECT COALESCE(array_agg(pg_get_constraintdef(oid) ORDER BY conname), ARRAY[]::text[])
FROM pg_constraint
WHERE conrelid = 'public.source_formation_items'::regclass AND contype = 'c'`).Scan(&itemChecks); err != nil {
		t.Fatal(err)
	}
	runJoined := strings.Join(runChecks, " ")
	for _, expected := range []string{"pending", "completed", "abstained", "failed", "document", "conversation", "input_manifest", "jsonb_typeof", "65536", "64"} {
		if !strings.Contains(runJoined, expected) {
			t.Fatalf("formation run checks do not constrain %q: %s", expected, runJoined)
		}
	}
	itemJoined := strings.Join(itemChecks, " ")
	for _, expected := range []string{"new", "update", "unchanged", "2048", "512", "byte_end", "quote_occurrence"} {
		if !strings.Contains(itemJoined, expected) {
			t.Fatalf("formation item checks do not constrain %q: %s", expected, itemJoined)
		}
	}

	for _, constraint := range []string{
		"source_formation_runs_tenant_continuity_fk",
		"source_formation_items_tenant_run_fk",
		"source_formation_items_tenant_target_memory_fk",
		"source_formation_items_tenant_evidence_observation_fk",
		"source_formation_items_tenant_observation_fk",
		"source_formation_items_tenant_candidate_memory_fk",
	} {
		var validated bool
		var definition string
		if err := store.pool.QueryRow(ctx, `
SELECT convalidated, pg_get_constraintdef(oid)
FROM pg_constraint
WHERE conname = $1`, constraint).Scan(&validated, &definition); err != nil {
			t.Fatalf("inspect constraint %s: %v", constraint, err)
		}
		if !validated || !strings.Contains(definition, "tenant_id") || !strings.Contains(definition, "continuity_id") {
			t.Fatalf("constraint %s is not tenant-and-continuity aware: validated=%v definition=%q", constraint, validated, definition)
		}
	}
}

func TestResetForTestClearsSourceFormationAudit(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	continuityID, err := store.ConfirmWorkspaceBinding(ctx, "reset-source-formation", "/fixtures/reset-source-formation")
	if err != nil {
		t.Fatal(err)
	}
	target, err := NewGovernanceService(store, "reset-source-formation").AddSource(ctx, "/fixtures/reset-source-formation", GovernanceWriteRequest{
		OperationID: "reset-source-formation-target",
		MemoryKey:   "reset.key",
		Content:     "Reset quote.",
		SourceRef:   "fixture:reset-target",
	})
	if err != nil {
		t.Fatal(err)
	}
	var runID string
	if err := store.pool.QueryRow(ctx, `
INSERT INTO source_formation_runs (
  tenant_id, continuity_id, operation_id, request_fingerprint,
  source_ref, source_sha256, source_bytes, active_snapshot,
  active_snapshot_fingerprint, provider_name, requested_model, status
) VALUES (
  'reset-source-formation', $1::uuid, 'reset-source-formation-op', repeat('a', 64),
  'fixture:reset', repeat('b', 64), 16, '[]'::jsonb, repeat('c', 64),
  'test-provider', 'test-model', 'pending'
)
RETURNING id::text`, continuityID).Scan(&runID); err != nil {
		t.Fatal(err)
	}
	observation, err := store.CommitObservation(ctx, "reset-source-formation", continuityID, CommitObservationRequest{
		OperationID: "reset-source-formation-item",
		Kind:        ObservationKindSourceCandidate,
		MemoryKey:   "reset.key",
		Content:     "Reset quote.",
		SourceRef:   "fixture:reset",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.pool.Exec(ctx, `
INSERT INTO source_formation_items (
  tenant_id, continuity_id, run_id, ordinal, decision, memory_key,
  quote, quote_occurrence, byte_start, byte_end, content, reason,
  target_memory_id, observation_id
) VALUES (
  'reset-source-formation', $1::uuid, $2::uuid, 1, 'unchanged', 'reset.key',
  'Reset quote.', 1, 0, 12, 'Reset quote.', 'Reset fixture.', $3::uuid, $4::uuid
)`, continuityID, runID, target.Memory.MemoryID, observation.ObservationID); err != nil {
		t.Fatal(err)
	}
	if err := store.ResetForTest(ctx); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"source_formation_items", "source_formation_runs"} {
		var count int
		if err := store.pool.QueryRow(ctx, `SELECT count(*) FROM `+table).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("%s survived reset: %d", table, count)
		}
	}
}
