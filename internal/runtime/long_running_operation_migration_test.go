package runtime

import (
	"context"
	"strings"
	"testing"
)

func TestLongRunningOperationMigrationExtendsConversationTurnsCoherently(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()

	var columns []string
	if err := store.pool.QueryRow(ctx, `
SELECT COALESCE(array_agg(column_name ORDER BY ordinal_position), ARRAY[]::text[])
FROM information_schema.columns
WHERE table_schema = 'public' AND table_name = 'conversation_turns'`).Scan(&columns); err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		"operation_protocol", "attempt_id", "lease_generation", "lease_expires_at",
		"last_heartbeat_at", "checkpoint_sequence", "checkpoint_payload",
		"checkpoint_fingerprint", "checkpoint_updated_at", "cancellation_code",
		"cancellation_message",
	} {
		if !containsString(columns, expected) {
			t.Fatalf("conversation_turns column %q is missing: %#v", expected, columns)
		}
	}

	var checks []string
	if err := store.pool.QueryRow(ctx, `
SELECT COALESCE(array_agg(pg_get_constraintdef(oid) ORDER BY conname), ARRAY[]::text[])
FROM pg_constraint
WHERE conrelid = 'public.conversation_turns'::regclass AND contype = 'c'`).Scan(&checks); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(checks, "\n")
	for _, expected := range []string{
		"cancelled", "bounded_v1", "leased_v1", "lease_generation",
		"checkpoint_sequence", "checkpoint_fingerprint", "cancellation_code",
	} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("conversation turn checks do not constrain %q: %s", expected, joined)
		}
	}
}
