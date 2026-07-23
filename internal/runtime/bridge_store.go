package runtime

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type selectedBridgeMemory struct {
	ID      string
	Content string
}

func (s *Store) ReplayBridgeOperation(ctx context.Context, tenantID, operationID string, action BridgeAction, fingerprint string) (BridgeReceipt, bool, error) {
	ctx, err := withTenantContext(ctx, tenantID)
	if err != nil {
		return BridgeReceipt{}, false, err
	}
	var bridgeID string
	var existingAction BridgeAction
	var existingFingerprint string
	err = s.pool.QueryRow(ctx, `
SELECT id::text, action, request_fingerprint
FROM bridge_operations
WHERE tenant_id = $1 AND operation_id = $2`, tenantID, operationID).Scan(&bridgeID, &existingAction, &existingFingerprint)
	if errors.Is(err, pgx.ErrNoRows) {
		return BridgeReceipt{}, false, nil
	}
	if err != nil {
		return BridgeReceipt{}, false, fmt.Errorf("lookup bridge operation replay: %w", err)
	}
	if existingAction != action || existingFingerprint != fingerprint {
		return BridgeReceipt{}, false, fmt.Errorf("operation_id is already bound to another bridge request")
	}
	receipt, err := s.InspectBridge(ctx, tenantID, bridgeID)
	if err != nil {
		return BridgeReceipt{}, false, err
	}
	receipt.Replayed = true
	return receipt, true, nil
}

func (s *Store) PromoteConversationMemory(ctx context.Context, tenantID, operationID, sourceContinuityID, targetContinuityID, sourceAnchor, targetAnchor string, memoryIDs []string) (BridgeReceipt, error) {
	ctx, err := withTenantContext(ctx, tenantID)
	if err != nil {
		return BridgeReceipt{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return BridgeReceipt{}, fmt.Errorf("begin bridge promotion: %w", err)
	}
	defer tx.Rollback(ctx)
	fingerprint := bridgeRequestFingerprint(sourceAnchor, targetAnchor, strings.Join(memoryIDs, ","))
	operation, replayed, err := createBridgeOperationTx(ctx, tx, bridgeLedgerInput{
		TenantID:           tenantID,
		OperationID:        operationID,
		Action:             BridgeActionPromote,
		RequestFingerprint: fingerprint,
		SourceContinuityID: sourceContinuityID,
		TargetContinuityID: targetContinuityID,
		SourceAnchor:       sourceAnchor,
		TargetAnchor:       targetAnchor,
	})
	if err != nil {
		return BridgeReceipt{}, err
	}
	if !replayed {
		snapshot, err := currentEligibilitySnapshotTx(ctx, tx)
		if err != nil {
			return BridgeReceipt{}, err
		}
		memories, err := loadSelectedEligibleMemoriesTx(ctx, tx, tenantID, sourceContinuityID, memoryIDs, snapshot.AsOf)
		if err != nil {
			return BridgeReceipt{}, err
		}
		for index, memory := range memories {
			observation, err := commitObservationTx(ctx, tx, tenantID, targetContinuityID, CommitObservationRequest{
				OperationID: operationID + ":promote:" + fmt.Sprint(index),
				Kind:        ObservationKindBridgePromote,
				Content:     memory.Content,
				SourceRef:   "bridge:" + operation.ID + ":memory:" + memory.ID,
			})
			if err != nil {
				return BridgeReceipt{}, err
			}
			var targetMemoryID string
			err = tx.QueryRow(ctx, `
INSERT INTO governed_memories (
  tenant_id, continuity_id, origin_observation_id, memory_kind, lifecycle_status, content
)
VALUES ($1, $2::uuid, $3::uuid, 'bridge_promoted', 'active', $4)
RETURNING id::text`, tenantID, targetContinuityID, observation.ObservationID, memory.Content).Scan(&targetMemoryID)
			if err != nil {
				return BridgeReceipt{}, fmt.Errorf("create promoted memory: %w", err)
			}
			if _, err := tx.Exec(ctx, `
INSERT INTO memory_search_documents (memory_id, tenant_id, continuity_id, content, search_document)
VALUES ($1::uuid, $2, $3::uuid, $4, to_tsvector('simple', $4))`, targetMemoryID, tenantID, targetContinuityID, memory.Content); err != nil {
				return BridgeReceipt{}, fmt.Errorf("project promoted memory: %w", err)
			}
			if _, err := tx.Exec(ctx, `
INSERT INTO bridge_memory_effects (
  bridge_id, tenant_id, effect_kind, source_memory_id, target_memory_id, order_index
)
VALUES ($1::uuid, $2, 'promote', $3::uuid, $4::uuid, $5)`, operation.ID, tenantID, memory.ID, targetMemoryID, index); err != nil {
				return BridgeReceipt{}, fmt.Errorf("record promoted memory effect: %w", err)
			}
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return BridgeReceipt{}, fmt.Errorf("commit bridge promotion: %w", err)
	}
	receipt, err := s.InspectBridge(ctx, tenantID, operation.ID)
	receipt.Replayed = replayed
	return receipt, err
}

func (s *Store) ExportWorkspaceMemory(ctx context.Context, tenantID, operationID, sourceContinuityID, sourceAnchor string, memoryIDs []string, title, targetProfile string) (BridgeReceipt, error) {
	ctx, err := withTenantContext(ctx, tenantID)
	if err != nil {
		return BridgeReceipt{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return BridgeReceipt{}, fmt.Errorf("begin bridge export: %w", err)
	}
	defer tx.Rollback(ctx)
	operation, replayed, err := createBridgeOperationTx(ctx, tx, bridgeLedgerInput{
		TenantID:           tenantID,
		OperationID:        operationID,
		Action:             BridgeActionExport,
		RequestFingerprint: bridgeRequestFingerprint(sourceAnchor, strings.Join(memoryIDs, ","), title, targetProfile),
		SourceContinuityID: sourceContinuityID,
		SourceAnchor:       sourceAnchor,
		TargetProfile:      targetProfile,
		Title:              title,
	})
	if err != nil {
		return BridgeReceipt{}, err
	}
	if !replayed {
		snapshot, err := currentEligibilitySnapshotTx(ctx, tx)
		if err != nil {
			return BridgeReceipt{}, err
		}
		memories, err := loadSelectedEligibleMemoriesTx(ctx, tx, tenantID, sourceContinuityID, memoryIDs, snapshot.AsOf)
		if err != nil {
			return BridgeReceipt{}, err
		}
		lines := make([]string, 0, len(memories))
		for _, memory := range memories {
			lines = append(lines, "- "+memory.Content)
		}
		body := strings.TrimSpace(title) + "\n\n" + strings.Join(lines, "\n")
		if _, err := tx.Exec(ctx, `
UPDATE bridge_operations SET export_body = $1 WHERE id = $2::uuid`, body, operation.ID); err != nil {
			return BridgeReceipt{}, fmt.Errorf("store bridge export body: %w", err)
		}
		for index, memory := range memories {
			if _, err := tx.Exec(ctx, `
INSERT INTO bridge_memory_effects (
  bridge_id, tenant_id, effect_kind, source_memory_id, order_index
)
VALUES ($1::uuid, $2, 'export', $3::uuid, $4)`, operation.ID, tenantID, memory.ID, index); err != nil {
				return BridgeReceipt{}, fmt.Errorf("record exported memory effect: %w", err)
			}
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return BridgeReceipt{}, fmt.Errorf("commit bridge export: %w", err)
	}
	receipt, err := s.InspectBridge(ctx, tenantID, operation.ID)
	receipt.Replayed = replayed
	return receipt, err
}

func (s *Store) LinkConversationContinuities(ctx context.Context, tenantID, operationID, primaryContinuityID, linkedContinuityID, primaryAnchor, linkedAnchor string) (BridgeReceipt, error) {
	ctx, err := withTenantContext(ctx, tenantID)
	if err != nil {
		return BridgeReceipt{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return BridgeReceipt{}, fmt.Errorf("begin conversation link: %w", err)
	}
	defer tx.Rollback(ctx)
	operation, replayed, err := createBridgeOperationTx(ctx, tx, bridgeLedgerInput{
		TenantID:           tenantID,
		OperationID:        operationID,
		Action:             BridgeActionLink,
		RequestFingerprint: bridgeRequestFingerprint(primaryAnchor, linkedAnchor),
		SourceContinuityID: primaryContinuityID,
		TargetContinuityID: linkedContinuityID,
		SourceAnchor:       primaryAnchor,
		TargetAnchor:       linkedAnchor,
	})
	if err != nil {
		return BridgeReceipt{}, err
	}
	if !replayed {
		var validContinuities int
		if err := tx.QueryRow(ctx, `
SELECT count(*)
FROM continuity_spaces
WHERE tenant_id = $1 AND id IN ($2::uuid, $3::uuid)
  AND continuity_line = 'conversation' AND state = 'active'`, tenantID, primaryContinuityID, linkedContinuityID).Scan(&validContinuities); err != nil {
			return BridgeReceipt{}, fmt.Errorf("validate linked conversations: %w", err)
		}
		if validContinuities != 2 {
			return BridgeReceipt{}, fmt.Errorf("both bridge endpoints must be active conversations in this tenant")
		}
		var primaryIsChild, linkedIsPrimary, linkedIsChild bool
		if err := tx.QueryRow(ctx, `
SELECT
  EXISTS (SELECT 1 FROM conversation_links WHERE tenant_id = $1 AND linked_continuity_id = $2::uuid AND link_state = 'active'),
  EXISTS (SELECT 1 FROM conversation_links WHERE tenant_id = $1 AND primary_continuity_id = $3::uuid AND link_state = 'active'),
  EXISTS (SELECT 1 FROM conversation_links WHERE tenant_id = $1 AND linked_continuity_id = $3::uuid AND link_state = 'active')`,
			tenantID, primaryContinuityID, linkedContinuityID).Scan(&primaryIsChild, &linkedIsPrimary, &linkedIsChild); err != nil {
			return BridgeReceipt{}, fmt.Errorf("validate conversation link graph: %w", err)
		}
		if primaryIsChild {
			return BridgeReceipt{}, fmt.Errorf("primary conversation is already linked under another primary")
		}
		if linkedIsPrimary {
			return BridgeReceipt{}, fmt.Errorf("linked conversation is already a primary conversation")
		}
		if linkedIsChild {
			return BridgeReceipt{}, fmt.Errorf("linked conversation already belongs to an active link group")
		}
		if _, err := tx.Exec(ctx, `
INSERT INTO conversation_links (
  bridge_id, tenant_id, primary_continuity_id, linked_continuity_id, link_state
)
VALUES ($1::uuid, $2, $3::uuid, $4::uuid, 'active')`, operation.ID, tenantID, primaryContinuityID, linkedContinuityID); err != nil {
			return BridgeReceipt{}, fmt.Errorf("create conversation link: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return BridgeReceipt{}, fmt.Errorf("commit conversation link: %w", err)
	}
	receipt, err := s.InspectBridge(ctx, tenantID, operation.ID)
	receipt.Replayed = replayed
	return receipt, err
}

func (s *Store) AdoptWorkspaceBinding(ctx context.Context, tenantID, operationID, continuityID string, existingAnchor, newAnchor WorkspaceAnchor) (BridgeReceipt, error) {
	ctx, err := withTenantContext(ctx, tenantID)
	if err != nil {
		return BridgeReceipt{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return BridgeReceipt{}, fmt.Errorf("begin workspace adopt: %w", err)
	}
	defer tx.Rollback(ctx)
	existingAnchor, err = existingAnchor.Normalized()
	if err != nil {
		return BridgeReceipt{}, err
	}
	newAnchor, err = newAnchor.Normalized()
	if err != nil {
		return BridgeReceipt{}, err
	}
	operation, replayed, err := createBridgeOperationTx(ctx, tx, bridgeLedgerInput{
		TenantID:           tenantID,
		OperationID:        operationID,
		Action:             BridgeActionAdopt,
		RequestFingerprint: bridgeRequestFingerprint(existingAnchor.FilesystemNamespace, existingAnchor.RepoRoot, newAnchor.FilesystemNamespace, newAnchor.RepoRoot),
		SourceContinuityID: continuityID,
		TargetContinuityID: continuityID,
		SourceAnchor:       existingAnchor.RepoRoot,
		TargetAnchor:       newAnchor.RepoRoot,
		SourceNamespaceID:  existingAnchor.FilesystemNamespace,
		TargetNamespaceID:  newAnchor.FilesystemNamespace,
	})
	if err != nil {
		return BridgeReceipt{}, err
	}
	if !replayed {
		if err := requireConfirmedWorkspaceBindingTx(ctx, tx, tenantID, continuityID, existingAnchor); err != nil {
			return BridgeReceipt{}, err
		}
		if err := requireUnboundWorkspaceRootTx(ctx, tx, tenantID, newAnchor); err != nil {
			return BridgeReceipt{}, err
		}
		if _, err := tx.Exec(ctx, `
		INSERT INTO continuity_bindings (continuity_id, tenant_id, filesystem_namespace, repo_root, binding_state)
		VALUES ($1::uuid, $2, $3, $4, 'confirmed')`, continuityID, tenantID, newAnchor.FilesystemNamespace, newAnchor.RepoRoot); err != nil {
			return BridgeReceipt{}, fmt.Errorf("create adopted workspace binding: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return BridgeReceipt{}, fmt.Errorf("commit workspace adopt: %w", err)
	}
	receipt, err := s.InspectBridge(ctx, tenantID, operation.ID)
	receipt.Replayed = replayed
	return receipt, err
}

func (s *Store) RebindWorkspaceBinding(ctx context.Context, tenantID, operationID, continuityID string, oldAnchor, newAnchor WorkspaceAnchor) (BridgeReceipt, error) {
	ctx, err := withTenantContext(ctx, tenantID)
	if err != nil {
		return BridgeReceipt{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return BridgeReceipt{}, fmt.Errorf("begin workspace rebind: %w", err)
	}
	defer tx.Rollback(ctx)
	oldAnchor, err = oldAnchor.Normalized()
	if err != nil {
		return BridgeReceipt{}, err
	}
	newAnchor, err = newAnchor.Normalized()
	if err != nil {
		return BridgeReceipt{}, err
	}
	operation, replayed, err := createBridgeOperationTx(ctx, tx, bridgeLedgerInput{
		TenantID:           tenantID,
		OperationID:        operationID,
		Action:             BridgeActionRebind,
		RequestFingerprint: bridgeRequestFingerprint(oldAnchor.FilesystemNamespace, oldAnchor.RepoRoot, newAnchor.FilesystemNamespace, newAnchor.RepoRoot),
		SourceContinuityID: continuityID,
		TargetContinuityID: continuityID,
		SourceAnchor:       oldAnchor.RepoRoot,
		TargetAnchor:       newAnchor.RepoRoot,
		SourceNamespaceID:  oldAnchor.FilesystemNamespace,
		TargetNamespaceID:  newAnchor.FilesystemNamespace,
	})
	if err != nil {
		return BridgeReceipt{}, err
	}
	if !replayed {
		bindingID, err := confirmedWorkspaceBindingIDTx(ctx, tx, tenantID, continuityID, oldAnchor)
		if err != nil {
			return BridgeReceipt{}, err
		}
		if err := requireUnboundWorkspaceRootTx(ctx, tx, tenantID, newAnchor); err != nil {
			return BridgeReceipt{}, err
		}
		if _, err := tx.Exec(ctx, `
			UPDATE continuity_bindings SET binding_state = 'retired' WHERE id = $1::uuid`, bindingID); err != nil {
			return BridgeReceipt{}, fmt.Errorf("retire old workspace binding: %w", err)
		}
		if _, err := tx.Exec(ctx, `
		INSERT INTO continuity_bindings (continuity_id, tenant_id, filesystem_namespace, repo_root, binding_state)
		VALUES ($1::uuid, $2, $3, $4, 'confirmed')`, continuityID, tenantID, newAnchor.FilesystemNamespace, newAnchor.RepoRoot); err != nil {
			return BridgeReceipt{}, fmt.Errorf("create rebound workspace binding: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return BridgeReceipt{}, fmt.Errorf("commit workspace rebind: %w", err)
	}
	receipt, err := s.InspectBridge(ctx, tenantID, operation.ID)
	receipt.Replayed = replayed
	return receipt, err
}

func (s *Store) ReverseBridge(ctx context.Context, tenantID, operationID, bridgeID string) (BridgeReceipt, error) {
	ctx, err := withTenantContext(ctx, tenantID)
	if err != nil {
		return BridgeReceipt{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return BridgeReceipt{}, fmt.Errorf("begin bridge reversal: %w", err)
	}
	defer tx.Rollback(ctx)
	var action BridgeAction
	var status BridgeStatus
	var targetContinuityID string
	var sourceAnchor, targetAnchor string
	var sourceNamespaceID, targetNamespaceID string
	err = tx.QueryRow(ctx, `
SELECT action, status, COALESCE(target_continuity_id::text, ''),
       source_anchor, target_anchor, source_filesystem_namespace,
       target_filesystem_namespace
FROM bridge_operations
WHERE id = $1::uuid AND tenant_id = $2
FOR UPDATE`, bridgeID, tenantID).Scan(&action, &status, &targetContinuityID,
		&sourceAnchor, &targetAnchor, &sourceNamespaceID, &targetNamespaceID)
	if errors.Is(err, pgx.ErrNoRows) {
		return BridgeReceipt{}, fmt.Errorf("bridge does not exist")
	}
	if err != nil {
		return BridgeReceipt{}, fmt.Errorf("lock bridge for reversal: %w", err)
	}
	targetStatus := BridgeStatusReversed
	if action == BridgeActionExport {
		targetStatus = BridgeStatusRevoked
	}
	if status == BridgeStatusActive {
		switch action {
		case BridgeActionPromote:
			rows, err := tx.Query(ctx, `
SELECT target_memory_id::text
FROM bridge_memory_effects
WHERE bridge_id = $1::uuid AND tenant_id = $2 AND target_memory_id IS NOT NULL
ORDER BY order_index ASC`, bridgeID, tenantID)
			if err != nil {
				return BridgeReceipt{}, fmt.Errorf("list promoted memories for reversal: %w", err)
			}
			targetMemoryIDs := make([]string, 0)
			for rows.Next() {
				var targetMemoryID string
				if err := rows.Scan(&targetMemoryID); err != nil {
					rows.Close()
					return BridgeReceipt{}, fmt.Errorf("scan promoted memory for reversal: %w", err)
				}
				targetMemoryIDs = append(targetMemoryIDs, targetMemoryID)
			}
			if err := rows.Err(); err != nil {
				rows.Close()
				return BridgeReceipt{}, fmt.Errorf("iterate promoted memories for reversal: %w", err)
			}
			rows.Close()
			for _, targetMemoryID := range targetMemoryIDs {
				if err := deleteMemoryTx(ctx, tx, tenantID, targetContinuityID, targetMemoryID); err != nil {
					return BridgeReceipt{}, err
				}
			}
		case BridgeActionExport:
			if _, err := tx.Exec(ctx, `UPDATE bridge_operations SET export_body = '[revoked]' WHERE id = $1::uuid`, bridgeID); err != nil {
				return BridgeReceipt{}, fmt.Errorf("redact revoked export: %w", err)
			}
		case BridgeActionLink:
			command, err := tx.Exec(ctx, `
UPDATE conversation_links
SET link_state = 'reversed', updated_at = now()
WHERE bridge_id = $1::uuid AND tenant_id = $2 AND link_state = 'active'`, bridgeID, tenantID)
			if err != nil {
				return BridgeReceipt{}, fmt.Errorf("reverse conversation link: %w", err)
			}
			if command.RowsAffected() != 1 {
				return BridgeReceipt{}, fmt.Errorf("active conversation link effect is missing")
			}
		case BridgeActionAdopt:
			command, err := tx.Exec(ctx, `
UPDATE continuity_bindings
SET binding_state = 'retired'
WHERE tenant_id = $1 AND continuity_id = $2::uuid
  AND filesystem_namespace = $3 AND repo_root = $4 AND binding_state = 'confirmed'`,
				tenantID, targetContinuityID, targetNamespaceID, targetAnchor)
			if err != nil {
				return BridgeReceipt{}, fmt.Errorf("reverse adopted workspace binding: %w", err)
			}
			if command.RowsAffected() != 1 {
				return BridgeReceipt{}, fmt.Errorf("active adopted workspace binding is missing")
			}
		case BridgeActionRebind:
			command, err := tx.Exec(ctx, `
UPDATE continuity_bindings
SET binding_state = 'retired'
WHERE tenant_id = $1 AND continuity_id = $2::uuid
  AND filesystem_namespace = $3 AND repo_root = $4 AND binding_state = 'confirmed'`,
				tenantID, targetContinuityID, targetNamespaceID, targetAnchor)
			if err != nil {
				return BridgeReceipt{}, fmt.Errorf("retire rebound workspace target: %w", err)
			}
			if command.RowsAffected() != 1 {
				return BridgeReceipt{}, fmt.Errorf("active rebound workspace target is missing")
			}
			command, err = tx.Exec(ctx, `
WITH candidate AS (
  SELECT id
  FROM continuity_bindings
  WHERE tenant_id = $1 AND continuity_id = $2::uuid
    AND filesystem_namespace = $3 AND repo_root = $4 AND binding_state = 'retired'
  ORDER BY created_at DESC, id DESC
  LIMIT 1
  FOR UPDATE
)
UPDATE continuity_bindings
SET binding_state = 'confirmed'
WHERE id = (SELECT id FROM candidate)`, tenantID, targetContinuityID, sourceNamespaceID, sourceAnchor)
			if err != nil {
				return BridgeReceipt{}, fmt.Errorf("restore original workspace binding: %w", err)
			}
			if command.RowsAffected() != 1 {
				return BridgeReceipt{}, fmt.Errorf("retired original workspace binding is missing")
			}
		default:
			return BridgeReceipt{}, fmt.Errorf("bridge action %q reversal is not implemented", action)
		}
	}
	marked, err := markBridgeOperationReversedTx(ctx, tx, tenantID, bridgeID, operationID, targetStatus)
	if err != nil {
		return BridgeReceipt{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return BridgeReceipt{}, fmt.Errorf("commit bridge reversal: %w", err)
	}
	receipt, err := s.InspectBridge(ctx, tenantID, bridgeID)
	receipt.Replayed = marked.Replayed
	return receipt, err
}

func requireConfirmedWorkspaceBindingTx(ctx context.Context, tx pgx.Tx, tenantID, continuityID string, anchor WorkspaceAnchor) error {
	_, err := confirmedWorkspaceBindingIDTx(ctx, tx, tenantID, continuityID, anchor)
	return err
}

func confirmedWorkspaceBindingIDTx(ctx context.Context, tx pgx.Tx, tenantID, continuityID string, anchor WorkspaceAnchor) (string, error) {
	var bindingID string
	err := tx.QueryRow(ctx, `
SELECT id::text
FROM continuity_bindings
WHERE tenant_id = $1 AND continuity_id = $2::uuid
  AND filesystem_namespace = $3 AND repo_root = $4 AND binding_state = 'confirmed'
FOR UPDATE`, tenantID, continuityID, anchor.FilesystemNamespace, anchor.RepoRoot).Scan(&bindingID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("workspace binding is not confirmed for this continuity")
	}
	if err != nil {
		return "", fmt.Errorf("lock confirmed workspace binding: %w", err)
	}
	return bindingID, nil
}

func requireUnboundWorkspaceRootTx(ctx context.Context, tx pgx.Tx, tenantID string, anchor WorkspaceAnchor) error {
	var confirmed bool
	if err := tx.QueryRow(ctx, `
SELECT EXISTS (
  SELECT 1 FROM continuity_bindings
  WHERE tenant_id = $1 AND filesystem_namespace = $2 AND repo_root = $3 AND binding_state = 'confirmed'
)`, tenantID, anchor.FilesystemNamespace, anchor.RepoRoot).Scan(&confirmed); err != nil {
		return fmt.Errorf("check workspace target binding: %w", err)
	}
	if confirmed {
		return fmt.Errorf("workspace target root is already confirmed")
	}
	return nil
}

func loadSelectedEligibleMemoriesTx(ctx context.Context, tx pgx.Tx, tenantID, continuityID string, memoryIDs []string, asOf time.Time) ([]selectedBridgeMemory, error) {
	memories := make([]selectedBridgeMemory, 0, len(memoryIDs))
	for _, memoryID := range memoryIDs {
		var memory selectedBridgeMemory
		err := tx.QueryRow(ctx, `
SELECT id::text, content
FROM governed_memories
WHERE id = $1::uuid AND tenant_id = $2 AND continuity_id = $3::uuid
	AND memory_is_eligible(lifecycle_status, content, valid_from, valid_until, $4)
FOR SHARE`, memoryID, tenantID, continuityID, asOf).Scan(&memory.ID, &memory.Content)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("memory %s must be active and currently eligible in the selected source continuity", memoryID)
		}
		if err != nil {
			return nil, fmt.Errorf("load selected bridge memory: %w", err)
		}
		if memory.Content == "" || memory.Content == "[redacted]" {
			return nil, fmt.Errorf("memory %s must contain active eligible semantic content", memoryID)
		}
		memories = append(memories, memory)
	}
	return memories, nil
}

func createBridgeOperationTx(ctx context.Context, tx pgx.Tx, input bridgeLedgerInput) (BridgeReceipt, bool, error) {
	if err := input.normalize(); err != nil {
		return BridgeReceipt{}, false, err
	}
	var existing BridgeReceipt
	var existingFingerprint string
	err := tx.QueryRow(ctx, `
SELECT id::text, operation_id, action, status, request_fingerprint
FROM bridge_operations
WHERE tenant_id = $1 AND operation_id = $2`, input.TenantID, input.OperationID).Scan(
		&existing.ID,
		&existing.OperationID,
		&existing.Action,
		&existing.Status,
		&existingFingerprint,
	)
	if err == nil {
		if existing.Action != input.Action || existingFingerprint != input.RequestFingerprint {
			return BridgeReceipt{}, false, fmt.Errorf("operation_id is already bound to another bridge request")
		}
		existing.Replayed = true
		return existing, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return BridgeReceipt{}, false, fmt.Errorf("lookup bridge operation: %w", err)
	}

	var receipt BridgeReceipt
	err = tx.QueryRow(ctx, `
INSERT INTO bridge_operations (
  tenant_id, operation_id, action, status,
  source_continuity_id, target_continuity_id,
  source_anchor, target_anchor, source_filesystem_namespace,
  target_filesystem_namespace, target_profile, title, export_body, request_fingerprint
)
VALUES (
  $1, $2, $3, 'active',
  NULLIF($4, '')::uuid, NULLIF($5, '')::uuid,
  $6, $7, $8, $9, $10, $11, $12, $13
)
RETURNING id::text, operation_id, action, status`,
		input.TenantID,
		input.OperationID,
		input.Action,
		input.SourceContinuityID,
		input.TargetContinuityID,
		input.SourceAnchor,
		input.TargetAnchor,
		input.SourceNamespaceID,
		input.TargetNamespaceID,
		input.TargetProfile,
		input.Title,
		input.ExportBody,
		input.RequestFingerprint,
	).Scan(&receipt.ID, &receipt.OperationID, &receipt.Action, &receipt.Status)
	if err != nil {
		return BridgeReceipt{}, false, fmt.Errorf("create bridge operation: %w", err)
	}
	if _, err := tx.Exec(ctx, `
INSERT INTO bridge_events (tenant_id, bridge_id, event_type, operation_id)
VALUES ($1, $2::uuid, 'created', $3)`, input.TenantID, receipt.ID, input.OperationID); err != nil {
		return BridgeReceipt{}, false, fmt.Errorf("record bridge creation event: %w", err)
	}
	return receipt, false, nil
}

func markBridgeOperationReversedTx(ctx context.Context, tx pgx.Tx, tenantID, bridgeID, operationID string, targetStatus BridgeStatus) (BridgeReceipt, error) {
	tenantID = strings.TrimSpace(tenantID)
	bridgeID = strings.TrimSpace(bridgeID)
	operationID = strings.TrimSpace(operationID)
	if tenantID == "" {
		return BridgeReceipt{}, fmt.Errorf("tenant_id is required")
	}
	if bridgeID == "" {
		return BridgeReceipt{}, fmt.Errorf("bridge_id is required")
	}
	if operationID == "" {
		return BridgeReceipt{}, fmt.Errorf("operation_id is required")
	}
	if targetStatus != BridgeStatusReversed && targetStatus != BridgeStatusRevoked {
		return BridgeReceipt{}, fmt.Errorf("bridge reversal status %q is unsupported", targetStatus)
	}

	var receipt BridgeReceipt
	err := tx.QueryRow(ctx, `
SELECT id::text, operation_id, action, status, reverse_operation_id
FROM bridge_operations
WHERE id = $1::uuid AND tenant_id = $2
FOR UPDATE`, bridgeID, tenantID).Scan(
		&receipt.ID,
		&receipt.OperationID,
		&receipt.Action,
		&receipt.Status,
		&receipt.ReverseOperationID,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return BridgeReceipt{}, fmt.Errorf("bridge does not exist")
	}
	if err != nil {
		return BridgeReceipt{}, fmt.Errorf("lock bridge operation: %w", err)
	}
	if receipt.Status != BridgeStatusActive {
		if receipt.Status == targetStatus && receipt.ReverseOperationID == operationID {
			receipt.Replayed = true
			return receipt, nil
		}
		return BridgeReceipt{}, fmt.Errorf("bridge is already %s", receipt.Status)
	}
	var operationUsed bool
	if err := tx.QueryRow(ctx, `
SELECT EXISTS (
  SELECT 1 FROM bridge_operations
  WHERE tenant_id = $1 AND (operation_id = $2 OR reverse_operation_id = $2)
)`, tenantID, operationID).Scan(&operationUsed); err != nil {
		return BridgeReceipt{}, fmt.Errorf("check bridge reversal operation: %w", err)
	}
	if operationUsed {
		return BridgeReceipt{}, fmt.Errorf("operation_id is already bound to another bridge request")
	}
	eventType := BridgeEventReversed
	if targetStatus == BridgeStatusRevoked {
		eventType = BridgeEventRevoked
	}
	if _, err := tx.Exec(ctx, `
UPDATE bridge_operations
SET status = $1, reverse_operation_id = $2, reversed_at = now()
WHERE id = $3::uuid`, targetStatus, operationID, bridgeID); err != nil {
		return BridgeReceipt{}, fmt.Errorf("reverse bridge operation: %w", err)
	}
	if _, err := tx.Exec(ctx, `
INSERT INTO bridge_events (tenant_id, bridge_id, event_type, operation_id)
VALUES ($1, $2::uuid, $3, $4)`, tenantID, bridgeID, eventType, operationID); err != nil {
		return BridgeReceipt{}, fmt.Errorf("record bridge reversal event: %w", err)
	}
	receipt.Status = targetStatus
	receipt.ReverseOperationID = operationID
	return receipt, nil
}

func (s *Store) InspectBridge(ctx context.Context, tenantID, bridgeID string) (BridgeReceipt, error) {
	ctx, err := withTenantContext(ctx, tenantID)
	if err != nil {
		return BridgeReceipt{}, err
	}
	tenantID = strings.TrimSpace(tenantID)
	bridgeID = strings.TrimSpace(bridgeID)
	var receipt BridgeReceipt
	err = s.pool.QueryRow(ctx, `
SELECT id::text, operation_id, action, status,
       COALESCE(source_continuity_id::text, ''), COALESCE(target_continuity_id::text, ''),
       source_anchor, target_anchor, source_filesystem_namespace,
       target_filesystem_namespace, target_profile, title, export_body, reverse_operation_id
FROM bridge_operations
WHERE id = $1::uuid AND tenant_id = $2`, bridgeID, tenantID).Scan(
		&receipt.ID,
		&receipt.OperationID,
		&receipt.Action,
		&receipt.Status,
		&receipt.SourceContinuityID,
		&receipt.TargetContinuityID,
		&receipt.SourceAnchor,
		&receipt.TargetAnchor,
		&receipt.SourceNamespaceID,
		&receipt.TargetNamespaceID,
		&receipt.TargetProfile,
		&receipt.Title,
		&receipt.ExportBody,
		&receipt.ReverseOperationID,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return BridgeReceipt{}, fmt.Errorf("bridge does not exist")
	}
	if err != nil {
		return BridgeReceipt{}, fmt.Errorf("inspect bridge operation: %w", err)
	}
	events, err := s.listBridgeEvents(ctx, tenantID, bridgeID)
	if err != nil {
		return BridgeReceipt{}, err
	}
	effects, err := s.listBridgeMemoryEffects(ctx, tenantID, bridgeID)
	if err != nil {
		return BridgeReceipt{}, err
	}
	receipt.Events = events
	receipt.MemoryEffects = effects
	return receipt, nil
}

func (s *Store) listBridgeEvents(ctx context.Context, tenantID, bridgeID string) ([]BridgeEvent, error) {
	rows, err := s.pool.Query(ctx, `
SELECT id::text, event_type, operation_id, created_at::text
FROM bridge_events
WHERE tenant_id = $1 AND bridge_id = $2::uuid
ORDER BY created_at ASC, id ASC`, tenantID, bridgeID)
	if err != nil {
		return nil, fmt.Errorf("list bridge events: %w", err)
	}
	defer rows.Close()
	events := make([]BridgeEvent, 0)
	for rows.Next() {
		var event BridgeEvent
		if err := rows.Scan(&event.ID, &event.EventType, &event.OperationID, &event.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan bridge event: %w", err)
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate bridge events: %w", err)
	}
	return events, nil
}

func (s *Store) listBridgeMemoryEffects(ctx context.Context, tenantID, bridgeID string) ([]BridgeMemoryEffect, error) {
	rows, err := s.pool.Query(ctx, `
SELECT effect_kind, source_memory_id::text, COALESCE(target_memory_id::text, ''), order_index
FROM bridge_memory_effects
WHERE tenant_id = $1 AND bridge_id = $2::uuid
ORDER BY order_index ASC, source_memory_id ASC`, tenantID, bridgeID)
	if err != nil {
		return nil, fmt.Errorf("list bridge memory effects: %w", err)
	}
	defer rows.Close()
	effects := make([]BridgeMemoryEffect, 0)
	for rows.Next() {
		var effect BridgeMemoryEffect
		if err := rows.Scan(&effect.EffectKind, &effect.SourceMemoryID, &effect.TargetMemoryID, &effect.OrderIndex); err != nil {
			return nil, fmt.Errorf("scan bridge memory effect: %w", err)
		}
		effects = append(effects, effect)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate bridge memory effects: %w", err)
	}
	return effects, nil
}
