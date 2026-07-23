package runtime

import (
	"context"
	"strings"
	"testing"
)

func TestBridgeLedgerIsDurableIdempotentAndAudited(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	input := bridgeLedgerInput{
		TenantID:           "local",
		OperationID:        "bridge-ledger-create",
		Action:             BridgeActionExport,
		RequestFingerprint: bridgeRequestFingerprint("workspace:/repo/release", "team_handoff", "memory:one"),
		SourceAnchor:       "/repo/release",
		TargetProfile:      "team_handoff",
		Title:              "Release handoff",
		ExportBody:         "Release handoff\n\n- Use checkout_eta_v2.",
	}

	first := createBridgeLedgerForTest(t, store, input)
	if first.ID == "" || first.Status != BridgeStatusActive || first.Action != BridgeActionExport || first.Replayed {
		t.Fatalf("unexpected bridge receipt: %#v", first)
	}
	if len(first.Events) != 1 || first.Events[0].EventType != BridgeEventCreated || first.Events[0].OperationID != input.OperationID {
		t.Fatalf("bridge creation was not audited: %#v", first.Events)
	}

	replay := createBridgeLedgerForTest(t, store, input)
	if !replay.Replayed || replay.ID != first.ID || len(replay.Events) != 1 {
		t.Fatalf("bridge replay was not stable: first=%#v replay=%#v", first, replay)
	}

	conflict := input
	conflict.RequestFingerprint = bridgeRequestFingerprint("workspace:/repo/other", "team_handoff", "memory:one")
	_, err := createBridgeLedger(t, store, conflict)
	if err == nil || !strings.Contains(err.Error(), "operation_id") {
		t.Fatalf("conflicting bridge replay was accepted: %v", err)
	}

	inspection, err := store.InspectBridge(ctx, "local", first.ID)
	requireNoError(t, err)
	if inspection.ID != first.ID || inspection.ExportBody != input.ExportBody || len(inspection.Events) != 1 {
		t.Fatalf("durable bridge inspection lost state: %#v", inspection)
	}
	if _, err := store.InspectBridge(ctx, "other", first.ID); err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("cross-tenant bridge inspection was accepted: %v", err)
	}
}

func TestBridgeLedgerReversalIsIdempotentAndAppendOnly(t *testing.T) {
	store := openTestStore(t)
	created := createBridgeLedgerForTest(t, store, bridgeLedgerInput{
		TenantID:           "local",
		OperationID:        "bridge-ledger-reversible",
		Action:             BridgeActionAdopt,
		RequestFingerprint: bridgeRequestFingerprint("/repo/old", "/repo/alias"),
		SourceAnchor:       "/repo/old",
		TargetAnchor:       "/repo/alias",
	})

	first := reverseBridgeLedgerForTest(t, store, "local", created.ID, "bridge-ledger-reverse", BridgeStatusReversed)
	if first.Status != BridgeStatusReversed || first.Replayed || len(first.Events) != 2 || first.Events[1].EventType != BridgeEventReversed {
		t.Fatalf("unexpected reversal receipt: %#v", first)
	}
	replay := reverseBridgeLedgerForTest(t, store, "local", created.ID, "bridge-ledger-reverse", BridgeStatusReversed)
	if !replay.Replayed || replay.ID != first.ID || len(replay.Events) != 2 {
		t.Fatalf("reversal replay was not stable: first=%#v replay=%#v", first, replay)
	}

	tx, err := store.pool.Begin(context.Background())
	requireNoError(t, err)
	defer tx.Rollback(context.Background())
	_, err = markBridgeOperationReversedTx(context.Background(), tx, "local", created.ID, "different-reversal", BridgeStatusReversed)
	if err == nil || !strings.Contains(err.Error(), "already") {
		t.Fatalf("second logical reversal was accepted: %v", err)
	}
}

func TestBridgeLedgerDatabaseRejectsInvalidActionAndStatus(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()

	_, err := store.pool.Exec(ctx, `
INSERT INTO bridge_operations (tenant_id, operation_id, action, status, request_fingerprint)
VALUES ('local', 'invalid-action', 'merge_everything', 'active', 'fingerprint')`)
	if err == nil {
		t.Fatal("database accepted an invalid bridge action")
	}
	_, err = store.pool.Exec(ctx, `
INSERT INTO bridge_operations (tenant_id, operation_id, action, status, request_fingerprint)
VALUES ('local', 'invalid-status', 'link', 'half_done', 'fingerprint')`)
	if err == nil {
		t.Fatal("database accepted an invalid bridge status")
	}
}

func createBridgeLedgerForTest(t *testing.T, store *Store, input bridgeLedgerInput) BridgeReceipt {
	t.Helper()
	receipt, err := createBridgeLedger(t, store, input)
	requireNoError(t, err)
	return receipt
}

func createBridgeLedger(t *testing.T, store *Store, input bridgeLedgerInput) (BridgeReceipt, error) {
	t.Helper()
	ctx := context.Background()
	tx, err := store.pool.Begin(ctx)
	if err != nil {
		return BridgeReceipt{}, err
	}
	defer tx.Rollback(ctx)
	receipt, _, err := createBridgeOperationTx(ctx, tx, input)
	if err != nil {
		return BridgeReceipt{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return BridgeReceipt{}, err
	}
	inspection, err := store.InspectBridge(ctx, input.TenantID, receipt.ID)
	inspection.Replayed = receipt.Replayed
	return inspection, err
}

func reverseBridgeLedgerForTest(t *testing.T, store *Store, tenantID, bridgeID, operationID string, status BridgeStatus) BridgeReceipt {
	t.Helper()
	ctx := context.Background()
	tx, err := store.pool.Begin(ctx)
	requireNoError(t, err)
	defer tx.Rollback(ctx)
	receipt, err := markBridgeOperationReversedTx(ctx, tx, tenantID, bridgeID, operationID, status)
	requireNoError(t, err)
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	result, err := store.InspectBridge(ctx, tenantID, receipt.ID)
	requireNoError(t, err)
	result.Replayed = receipt.Replayed
	return result
}
