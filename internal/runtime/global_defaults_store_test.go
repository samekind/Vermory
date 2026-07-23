package runtime

import (
	"context"
	"strings"
	"testing"
)

func TestGlobalDefaultStoreKeyedLifecycleAndReplay(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)

	continuityID, err := store.EnsureGlobalDefaultsContinuity(ctx, "local")
	requireNoError(t, err)
	replayedContinuityID, err := store.EnsureGlobalDefaultsContinuity(ctx, "local")
	requireNoError(t, err)
	if replayedContinuityID != continuityID {
		t.Fatalf("global continuity changed: first=%s replay=%s", continuityID, replayedContinuityID)
	}

	content := "Default user-facing replies to Chinese unless the active task explicitly requests another language."
	created, err := store.SetGlobalDefault(ctx, "local", "global-set-language", "reply_language", content)
	requireNoError(t, err)
	if created.Memory.Status != "active" || created.Memory.MemoryID == "" {
		t.Fatalf("global default was not active: %#v", created)
	}

	replay, err := store.SetGlobalDefault(ctx, "local", "global-set-language", "reply_language", content)
	requireNoError(t, err)
	if !replay.Observation.Replayed || !replay.Memory.Replayed || replay.Memory.MemoryID != created.Memory.MemoryID {
		t.Fatalf("global set replay was not idempotent: first=%#v replay=%#v", created, replay)
	}

	_, err = store.SetGlobalDefault(ctx, "local", "global-set-language", "reply_language", "Use English by default.")
	if err == nil || !strings.Contains(err.Error(), "operation_id") {
		t.Fatalf("conflicting operation replay was accepted: %v", err)
	}
	_, err = store.SetGlobalDefault(ctx, "local", "global-set-language-again", "reply_language", content)
	if err == nil || !strings.Contains(err.Error(), "already has an active value") {
		t.Fatalf("second active value was accepted: %v", err)
	}

	active, err := store.ListActiveGlobalDefaults(ctx, "local")
	requireNoError(t, err)
	if len(active) != 1 || active[0].ID != created.Memory.MemoryID || active[0].Content != content {
		t.Fatalf("unexpected active defaults: %#v", active)
	}

	replacement := "Default user-facing replies to Chinese."
	corrected, err := store.CorrectGlobalDefault(ctx, "local", "global-correct-language", created.Memory.MemoryID, replacement)
	requireNoError(t, err)
	if corrected.Memory.Status != "active" || corrected.Memory.MemoryID == created.Memory.MemoryID {
		t.Fatalf("global correction did not create an active revision: %#v", corrected)
	}

	continuity, memories, err := store.ListGlobalDefaults(ctx, "local")
	requireNoError(t, err)
	if continuity != continuityID || len(memories) != 2 {
		t.Fatalf("unexpected global inspection: continuity=%s memories=%#v", continuity, memories)
	}
	if memories[0].ID != created.Memory.MemoryID || memories[0].LifecycleStatus != "superseded" || memories[0].MemoryKey != "reply_language" {
		t.Fatalf("original default did not retain its key and superseded state: %#v", memories[0])
	}
	if memories[1].ID != corrected.Memory.MemoryID || memories[1].LifecycleStatus != "active" || memories[1].MemoryKey != "reply_language" {
		t.Fatalf("replacement default did not retain its key: %#v", memories[1])
	}

	_, err = store.CorrectGlobalDefault(ctx, "other", "global-cross-tenant", corrected.Memory.MemoryID, "Must not cross tenant.")
	if err == nil || !strings.Contains(err.Error(), "does not belong") {
		t.Fatalf("cross-tenant correction was accepted: %v", err)
	}
}

func TestGlobalDefaultStoreDeletionRedactsCrossContinuityDeliveriesAndProjection(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	content := "Default user-facing replies to Chinese unless the active task explicitly requests another language."

	globalContinuityID, err := store.EnsureGlobalDefaultsContinuity(ctx, "local")
	requireNoError(t, err)
	created, err := store.SetGlobalDefault(ctx, "local", "global-set-redaction", "reply_language", content)
	requireNoError(t, err)

	workspaceContinuityID, err := store.ConfirmWorkspaceBinding(ctx, "local", "/repo/global-default-redaction")
	requireNoError(t, err)
	delivery, err := store.RecordDelivery(ctx, "local", workspaceContinuityID, "global-default-delivery", "answer the task", "Global defaults:\n"+content)
	requireNoError(t, err)

	forgotten, err := store.ForgetGlobalDefault(ctx, "local", "global-forget-redaction", created.Memory.MemoryID)
	requireNoError(t, err)
	if forgotten.Memory.Status != "deleted" || forgotten.Memory.MemoryID != created.Memory.MemoryID {
		t.Fatalf("global default was not deleted: %#v", forgotten)
	}

	var memoryContent, observationContent, deliveryContent string
	err = store.pool.QueryRow(ctx, `SELECT content FROM governed_memories WHERE id = $1::uuid`, created.Memory.MemoryID).Scan(&memoryContent)
	requireNoError(t, err)
	err = store.pool.QueryRow(ctx, `SELECT content FROM observations WHERE id = $1::uuid`, created.Observation.ObservationID).Scan(&observationContent)
	requireNoError(t, err)
	err = store.pool.QueryRow(ctx, `SELECT context_body FROM memory_deliveries WHERE id = $1::uuid`, delivery.DeliveryID).Scan(&deliveryContent)
	requireNoError(t, err)
	if memoryContent != "[redacted]" || observationContent != "[redacted]" || strings.Contains(deliveryContent, content) {
		t.Fatalf("deleted default residue: memory=%q observation=%q delivery=%q", memoryContent, observationContent, deliveryContent)
	}

	requireNoError(t, store.RebuildProjection(ctx, "local", globalContinuityID))
	if matches := mustSearch(t, store, globalContinuityID, "Chinese user-facing replies"); len(matches) != 0 {
		t.Fatalf("deleted default returned after projection rebuild: %#v", matches)
	}
	active, err := store.ListActiveGlobalDefaults(ctx, "local")
	requireNoError(t, err)
	if len(active) != 0 {
		t.Fatalf("deleted default remained active: %#v", active)
	}
}

func TestGlobalDefaultStoreDatabaseEnforcesSingletonContinuity(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	_, err := store.EnsureGlobalDefaultsContinuity(ctx, "local")
	requireNoError(t, err)

	_, err = store.pool.Exec(ctx, `
INSERT INTO continuity_spaces (tenant_id, continuity_line, state)
VALUES ('local', 'global_defaults', 'active')`)
	if err == nil {
		t.Fatal("database accepted a second active global-default continuity")
	}
}
