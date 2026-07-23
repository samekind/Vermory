package runtime

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
)

const sourceFormationSelectColumns = `
	id::text, continuity_id::text, operation_id, request_fingerprint,
	source_ref, source_sha256, source_bytes, input_kind, input_manifest,
	input_manifest_fingerprint, active_snapshot,
	active_snapshot_fingerprint, provider_name, requested_model, resolved_model,
status, provider_output, provider_artifact_sha256, reason, failure_code,
created_at, completed_at`

const sourceFormationPendingExpiry = defaultSourceMatchProviderTimeout + time.Minute

var sourceFormationMemoryKeyPattern = regexp.MustCompile(`^[a-z0-9]+([._-][a-z0-9]+)*$`)

type sourceFormationRow interface {
	Scan(dest ...any) error
}

type validatedSourceFormationItem struct {
	item                  SourceFormationProviderItem
	evidenceObservationID string
	byteStart             int
	byteEnd               int
	target                SourceMatchCandidate
	hasTarget             bool
}

func (s *Store) BeginSourceFormation(ctx context.Context, tenantID, continuityID string, request SourceFormationBeginRequest) (SourceFormationReceipt, error) {
	ctx, err := withTenantContext(ctx, tenantID)
	if err != nil {
		return SourceFormationReceipt{}, err
	}
	request, err = normalizeSourceFormationBeginRequest(request)
	if err != nil {
		return SourceFormationReceipt{}, err
	}
	fingerprint, err := sourceFormationRequestFingerprint(continuityID, request)
	if err != nil {
		return SourceFormationReceipt{}, err
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return SourceFormationReceipt{}, fmt.Errorf("begin source formation: %w", err)
	}
	defer tx.Rollback(ctx)
	snapshot, err := currentEligibilitySnapshotTx(ctx, tx)
	if err != nil {
		return SourceFormationReceipt{}, err
	}

	existing, found, err := lookupSourceFormationOperationTx(ctx, tx, tenantID, request.OperationID)
	if err != nil {
		return SourceFormationReceipt{}, err
	}
	if found {
		if existing.ContinuityID != continuityID || existing.RequestFingerprint != fingerprint {
			return SourceFormationReceipt{}, fmt.Errorf("operation_id is already bound to another logical source formation")
		}
		currentSnapshot, err := listSourceMatchCandidatesTx(ctx, tx, tenantID, continuityID, snapshot.AsOf, false)
		if err != nil {
			return SourceFormationReceipt{}, err
		}
		_, currentFingerprint, err := canonicalSourceMatchCandidates(currentSnapshot)
		if err != nil {
			return SourceFormationReceipt{}, err
		}
		if currentFingerprint != existing.ActiveSnapshotFingerprint {
			return SourceFormationReceipt{}, fmt.Errorf("operation_id active snapshot has changed")
		}
		if existing.InputKind == SourceFormationInputConversation {
			if err := verifyConversationFormationManifestTx(ctx, tx, tenantID, continuityID, existing.InputManifest); err != nil {
				return SourceFormationReceipt{}, fmt.Errorf("operation_id input manifest has changed: %w", err)
			}
		}
		if existing.Status == SourceFormationPending && time.Since(existing.CreatedAt) >= sourceFormationPendingExpiry {
			existing, err = updateTerminalSourceFormation(ctx, tx, tenantID, existing.ID, SourceFormationCompletion{
				Status:        SourceFormationFailed,
				FailureCode:   "pending_expired",
				Reason:        "previous source formation attempt expired before completion",
				ResolvedModel: existing.ResolvedModel,
			})
			if err != nil {
				return SourceFormationReceipt{}, err
			}
		}
		existing.Replayed = true
		if err := tx.Commit(ctx); err != nil {
			return SourceFormationReceipt{}, fmt.Errorf("commit replayed source formation: %w", err)
		}
		return existing, nil
	}

	continuityLine := string(request.InputKind)
	if request.InputKind == SourceFormationInputDocument {
		continuityLine = "workspace"
	}
	var validContinuity bool
	if err := tx.QueryRow(ctx, `
SELECT EXISTS (
  SELECT 1 FROM continuity_spaces
  WHERE id = $1::uuid AND tenant_id = $2
    AND continuity_line = $3 AND state = 'active'
)`, continuityID, tenantID, continuityLine).Scan(&validContinuity); err != nil {
		return SourceFormationReceipt{}, fmt.Errorf("check source formation continuity: %w", err)
	}
	if !validContinuity {
		return SourceFormationReceipt{}, fmt.Errorf("%s continuity is not active for this tenant", continuityLine)
	}
	if request.InputKind == SourceFormationInputConversation {
		if err := verifyConversationFormationManifestTx(ctx, tx, tenantID, continuityID, request.InputManifest); err != nil {
			return SourceFormationReceipt{}, err
		}
	}
	activeSnapshot, err := listSourceMatchCandidatesTx(ctx, tx, tenantID, continuityID, snapshot.AsOf, false)
	if err != nil {
		return SourceFormationReceipt{}, err
	}
	snapshotJSON, snapshotFingerprint, err := canonicalSourceMatchCandidates(activeSnapshot)
	if err != nil {
		return SourceFormationReceipt{}, err
	}
	receipt, err := scanSourceFormation(tx.QueryRow(ctx, `
	INSERT INTO source_formation_runs (
	  tenant_id, continuity_id, operation_id, request_fingerprint,
	  source_ref, source_sha256, source_bytes, input_kind, input_manifest,
	  input_manifest_fingerprint, active_snapshot, active_snapshot_fingerprint,
	  provider_name, requested_model, status
	)
	VALUES ($1, $2::uuid, $3, $4, $5, $6, $7, $8, $9::jsonb, $10, $11::jsonb, $12, $13, $14, 'pending')
RETURNING `+sourceFormationSelectColumns,
		tenantID,
		continuityID,
		request.OperationID,
		fingerprint,
		request.SourceRef,
		request.SourceSHA256,
		request.SourceBytes,
		request.InputKind,
		mustSourceFormationInputManifestJSON(request.InputManifest),
		sourceFormationInputManifestFingerprint(request.InputManifest),
		snapshotJSON,
		snapshotFingerprint,
		request.ProviderName,
		request.RequestedModel,
	))
	if err != nil {
		return SourceFormationReceipt{}, fmt.Errorf("insert source formation run: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return SourceFormationReceipt{}, fmt.Errorf("commit source formation begin: %w", err)
	}
	return receipt, nil
}

func (s *Store) CompleteSourceFormation(ctx context.Context, tenantID, runID string, sourceDocument []byte, completion SourceFormationCompletion) (SourceFormationReceipt, error) {
	var err error
	ctx, err = withTenantContext(ctx, tenantID)
	if err != nil {
		return SourceFormationReceipt{}, err
	}
	return s.completeSourceFormation(ctx, tenantID, runID, sourceDocument, completion)
}

func (s *Store) CompleteConversationFormation(ctx context.Context, tenantID, runID string, completion SourceFormationCompletion) (SourceFormationReceipt, error) {
	var err error
	ctx, err = withTenantContext(ctx, tenantID)
	if err != nil {
		return SourceFormationReceipt{}, err
	}
	return s.completeSourceFormation(ctx, tenantID, runID, nil, completion)
}

func (s *Store) completeSourceFormation(ctx context.Context, tenantID, runID string, sourceDocument []byte, completion SourceFormationCompletion) (SourceFormationReceipt, error) {
	ctx, err := withTenantContext(ctx, tenantID)
	if err != nil {
		return SourceFormationReceipt{}, err
	}
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return SourceFormationReceipt{}, fmt.Errorf("source formation run_id is required")
	}
	completion, err = normalizeSourceFormationCompletion(completion)
	if err != nil {
		return SourceFormationReceipt{}, err
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return SourceFormationReceipt{}, fmt.Errorf("begin source formation completion: %w", err)
	}
	defer tx.Rollback(ctx)
	snapshot, err := currentEligibilitySnapshotTx(ctx, tx)
	if err != nil {
		return SourceFormationReceipt{}, err
	}

	run, err := lockSourceFormationTx(ctx, tx, tenantID, runID)
	if err != nil {
		return SourceFormationReceipt{}, err
	}
	if run.Status != SourceFormationPending {
		run.Replayed = true
		if err := tx.Commit(ctx); err != nil {
			return SourceFormationReceipt{}, fmt.Errorf("commit replayed source formation completion: %w", err)
		}
		return run, nil
	}
	if time.Since(run.CreatedAt) >= sourceFormationPendingExpiry {
		receipt, err := updateTerminalSourceFormation(ctx, tx, tenantID, run.ID, SourceFormationCompletion{
			Status:                 SourceFormationFailed,
			ResolvedModel:          completion.ResolvedModel,
			ProviderOutput:         completion.ProviderOutput,
			ProviderArtifactSHA256: completion.ProviderArtifactSHA256,
			Reason:                 "source formation attempt expired before completion",
			FailureCode:            "pending_expired",
		})
		if err != nil {
			return SourceFormationReceipt{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return SourceFormationReceipt{}, fmt.Errorf("commit expired source formation: %w", err)
		}
		return receipt, nil
	}
	if completion.Status == SourceFormationFailed {
		receipt, err := updateTerminalSourceFormation(ctx, tx, tenantID, run.ID, completion)
		if err != nil {
			return SourceFormationReceipt{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return SourceFormationReceipt{}, fmt.Errorf("commit failed source formation: %w", err)
		}
		return receipt, nil
	}

	conversationEvidence := map[string]ConversationObservation(nil)
	switch run.InputKind {
	case SourceFormationInputDocument:
		if failureCode, reason := validateSourceFormationDocument(run, sourceDocument); failureCode != "" {
			return commitInvalidSourceFormation(ctx, tx, tenantID, run.ID, completion, failureCode, reason)
		}
	case SourceFormationInputConversation:
		if err := verifyConversationFormationManifestTx(ctx, tx, tenantID, run.ContinuityID, run.InputManifest); err != nil {
			return commitInvalidSourceFormation(ctx, tx, tenantID, run.ID, completion, "input_manifest_changed", err.Error())
		}
		conversationEvidence, err = loadConversationFormationEvidenceTx(ctx, tx, tenantID, run.ContinuityID, run.InputManifest)
		if err != nil {
			return commitInvalidSourceFormation(ctx, tx, tenantID, run.ID, completion, "input_manifest_changed", err.Error())
		}
	default:
		return commitInvalidSourceFormation(ctx, tx, tenantID, run.ID, completion, "invalid_input_kind", "source formation run has an unsupported input kind")
	}
	currentSnapshot, err := listSourceMatchCandidatesTx(ctx, tx, tenantID, run.ContinuityID, snapshot.AsOf, true)
	if err != nil {
		return SourceFormationReceipt{}, err
	}
	_, currentFingerprint, err := canonicalSourceMatchCandidates(currentSnapshot)
	if err != nil {
		return SourceFormationReceipt{}, err
	}
	if currentFingerprint != run.ActiveSnapshotFingerprint {
		return commitInvalidSourceFormation(ctx, tx, tenantID, run.ID, completion, "active_snapshot_changed", "active keyed facts changed while the provider was forming memory")
	}
	if completion.Status == SourceFormationAbstained {
		receipt, err := updateTerminalSourceFormation(ctx, tx, tenantID, run.ID, completion)
		if err != nil {
			return SourceFormationReceipt{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return SourceFormationReceipt{}, fmt.Errorf("commit abstained source formation: %w", err)
		}
		return receipt, nil
	}

	validated, failureCode, reason := validateSourceFormationItems(run.InputKind, sourceDocument, conversationEvidence, completion.Items, run.ActiveSnapshot)
	if failureCode != "" {
		return commitInvalidSourceFormation(ctx, tx, tenantID, run.ID, completion, failureCode, reason)
	}
	for ordinal, item := range validated {
		sourceRef := run.SourceRef
		if item.evidenceObservationID != "" {
			sourceRef = "observation:" + item.evidenceObservationID
		}
		observationRequest := CommitObservationRequest{
			OperationID: "source-formation:" + run.ID + ":" + fmt.Sprint(ordinal+1),
			Kind:        ObservationKindSourceCandidate,
			Content:     item.item.Content,
			SourceRef:   sourceRef,
			MemoryKey:   item.item.MemoryKey,
		}
		if item.hasTarget && item.item.Decision == SourceFormationUpdate {
			observationRequest.SupersedesMemoryID = item.target.MemoryID
		}
		if err := observationRequest.Validate(); err != nil {
			return SourceFormationReceipt{}, err
		}
		observation, err := commitObservationTx(ctx, tx, tenantID, run.ContinuityID, observationRequest)
		if err != nil {
			return SourceFormationReceipt{}, err
		}
		candidateMemoryID := ""
		if item.item.Decision != SourceFormationUnchanged {
			memory, err := governObservationTx(ctx, tx, tenantID, run.ContinuityID, observation.ObservationID, observationRequest)
			if err != nil {
				return SourceFormationReceipt{}, err
			}
			candidateMemoryID = memory.MemoryID
		}
		targetMemoryID := ""
		if item.hasTarget {
			targetMemoryID = item.target.MemoryID
		}
		if _, err := tx.Exec(ctx, `
	INSERT INTO source_formation_items (
	  tenant_id, continuity_id, run_id, ordinal, decision, memory_key,
	  quote, quote_occurrence, byte_start, byte_end, content, reason,
	  target_memory_id, evidence_observation_id, observation_id, candidate_memory_id
	)
	VALUES (
	  $1, $2::uuid, $3::uuid, $4, $5, $6,
	  $7, $8, $9, $10, $11, $12,
	  NULLIF($13, '')::uuid, NULLIF($14, '')::uuid, $15::uuid, NULLIF($16, '')::uuid
	)`, tenantID, run.ContinuityID, run.ID, ordinal+1, item.item.Decision, item.item.MemoryKey,
			item.item.Quote, item.item.Occurrence, item.byteStart, item.byteEnd, item.item.Content, item.item.Reason,
			targetMemoryID, item.evidenceObservationID, observation.ObservationID, candidateMemoryID); err != nil {
			return SourceFormationReceipt{}, fmt.Errorf("insert source formation item: %w", err)
		}
	}
	receipt, err := updateTerminalSourceFormation(ctx, tx, tenantID, run.ID, completion)
	if err != nil {
		return SourceFormationReceipt{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return SourceFormationReceipt{}, fmt.Errorf("commit completed source formation: %w", err)
	}
	return receipt, nil
}

func (s *Store) FailSourceFormation(ctx context.Context, tenantID, runID string, completion SourceFormationCompletion) (SourceFormationReceipt, error) {
	var err error
	ctx, err = withTenantContext(ctx, tenantID)
	if err != nil {
		return SourceFormationReceipt{}, err
	}
	completion.Status = SourceFormationFailed
	return s.completeSourceFormation(ctx, tenantID, runID, nil, completion)
}

func (s *Store) InspectSourceFormation(ctx context.Context, tenantID, continuityID, operationID string) (SourceFormationReceipt, error) {
	ctx, err := withTenantContext(ctx, tenantID)
	if err != nil {
		return SourceFormationReceipt{}, err
	}
	continuityID = strings.TrimSpace(continuityID)
	operationID = strings.TrimSpace(operationID)
	if continuityID == "" || operationID == "" {
		return SourceFormationReceipt{}, fmt.Errorf("source formation continuity_id and operation_id are required")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return SourceFormationReceipt{}, fmt.Errorf("begin source formation inspection: %w", err)
	}
	defer tx.Rollback(ctx)
	receipt, err := scanSourceFormation(tx.QueryRow(ctx, `
SELECT `+sourceFormationSelectColumns+`
FROM source_formation_runs
WHERE tenant_id = $1 AND continuity_id = $2::uuid AND operation_id = $3`, tenantID, continuityID, operationID))
	if errors.Is(err, pgx.ErrNoRows) {
		return SourceFormationReceipt{}, fmt.Errorf("source formation run does not belong to this workspace continuity")
	}
	if err != nil {
		return SourceFormationReceipt{}, fmt.Errorf("inspect source formation run: %w", err)
	}
	receipt.Items, err = listSourceFormationItemsTx(ctx, tx, tenantID, receipt.ContinuityID, receipt.ID)
	if err != nil {
		return SourceFormationReceipt{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return SourceFormationReceipt{}, fmt.Errorf("commit source formation inspection: %w", err)
	}
	return receipt, nil
}

func (s *Store) LookupSourceFormation(ctx context.Context, tenantID, continuityID, operationID string) (SourceFormationReceipt, bool, error) {
	ctx, err := withTenantContext(ctx, tenantID)
	if err != nil {
		return SourceFormationReceipt{}, false, err
	}
	continuityID = strings.TrimSpace(continuityID)
	operationID = strings.TrimSpace(operationID)
	if continuityID == "" || operationID == "" {
		return SourceFormationReceipt{}, false, fmt.Errorf("source formation continuity_id and operation_id are required")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return SourceFormationReceipt{}, false, fmt.Errorf("begin source formation lookup: %w", err)
	}
	defer tx.Rollback(ctx)
	receipt, err := scanSourceFormation(tx.QueryRow(ctx, `
SELECT `+sourceFormationSelectColumns+`
FROM source_formation_runs
WHERE tenant_id = $1 AND continuity_id = $2::uuid AND operation_id = $3`, tenantID, continuityID, operationID))
	if errors.Is(err, pgx.ErrNoRows) {
		return SourceFormationReceipt{}, false, nil
	}
	if err != nil {
		return SourceFormationReceipt{}, false, fmt.Errorf("lookup source formation run: %w", err)
	}
	receipt.Items, err = listSourceFormationItemsTx(ctx, tx, tenantID, receipt.ContinuityID, receipt.ID)
	if err != nil {
		return SourceFormationReceipt{}, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return SourceFormationReceipt{}, false, fmt.Errorf("commit source formation lookup: %w", err)
	}
	return receipt, true, nil
}

func lookupSourceFormationOperationTx(ctx context.Context, tx pgx.Tx, tenantID, operationID string) (SourceFormationReceipt, bool, error) {
	receipt, err := scanSourceFormation(tx.QueryRow(ctx, `
SELECT `+sourceFormationSelectColumns+`
FROM source_formation_runs
WHERE tenant_id = $1 AND operation_id = $2
FOR UPDATE`, tenantID, operationID))
	if errors.Is(err, pgx.ErrNoRows) {
		return SourceFormationReceipt{}, false, nil
	}
	if err != nil {
		return SourceFormationReceipt{}, false, fmt.Errorf("lookup source formation operation: %w", err)
	}
	receipt.Items, err = listSourceFormationItemsTx(ctx, tx, tenantID, receipt.ContinuityID, receipt.ID)
	if err != nil {
		return SourceFormationReceipt{}, false, err
	}
	return receipt, true, nil
}

func lockSourceFormationTx(ctx context.Context, tx pgx.Tx, tenantID, runID string) (SourceFormationReceipt, error) {
	receipt, err := scanSourceFormation(tx.QueryRow(ctx, `
SELECT `+sourceFormationSelectColumns+`
FROM source_formation_runs
WHERE id = $1::uuid AND tenant_id = $2
FOR UPDATE`, runID, tenantID))
	if errors.Is(err, pgx.ErrNoRows) {
		return SourceFormationReceipt{}, fmt.Errorf("source formation run does not belong to this tenant")
	}
	if err != nil {
		return SourceFormationReceipt{}, fmt.Errorf("lock source formation run: %w", err)
	}
	receipt.Items, err = listSourceFormationItemsTx(ctx, tx, tenantID, receipt.ContinuityID, receipt.ID)
	if err != nil {
		return SourceFormationReceipt{}, err
	}
	return receipt, nil
}

func updateTerminalSourceFormation(ctx context.Context, tx pgx.Tx, tenantID, runID string, completion SourceFormationCompletion) (SourceFormationReceipt, error) {
	receipt, err := scanSourceFormation(tx.QueryRow(ctx, `
UPDATE source_formation_runs
SET resolved_model = $3,
    status = $4,
    provider_output = $5,
    provider_artifact_sha256 = $6,
    reason = $7,
    failure_code = $8,
    completed_at = now()
WHERE id = $1::uuid AND tenant_id = $2 AND status = 'pending'
RETURNING `+sourceFormationSelectColumns,
		runID,
		tenantID,
		completion.ResolvedModel,
		completion.Status,
		completion.ProviderOutput,
		completion.ProviderArtifactSHA256,
		completion.Reason,
		completion.FailureCode,
	))
	if err != nil {
		return SourceFormationReceipt{}, fmt.Errorf("complete source formation run: %w", err)
	}
	receipt.Items, err = listSourceFormationItemsTx(ctx, tx, tenantID, receipt.ContinuityID, receipt.ID)
	if err != nil {
		return SourceFormationReceipt{}, err
	}
	return receipt, nil
}

func commitInvalidSourceFormation(ctx context.Context, tx pgx.Tx, tenantID, runID string, completion SourceFormationCompletion, failureCode, reason string) (SourceFormationReceipt, error) {
	completion.Status = SourceFormationFailed
	completion.FailureCode = failureCode
	completion.Reason = truncateSourceFormationText(reason, 512)
	completion.Items = nil
	receipt, err := updateTerminalSourceFormation(ctx, tx, tenantID, runID, completion)
	if err != nil {
		return SourceFormationReceipt{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return SourceFormationReceipt{}, fmt.Errorf("commit invalid source formation: %w", err)
	}
	return receipt, nil
}

func listSourceFormationItemsTx(ctx context.Context, tx pgx.Tx, tenantID, continuityID, runID string) ([]SourceFormationItemReceipt, error) {
	rows, err := tx.Query(ctx, `
SELECT item.id::text, item.ordinal, item.decision, item.memory_key,
       item.quote, item.quote_occurrence, item.byte_start, item.byte_end,
       item.content, item.reason,
	       COALESCE(item.target_memory_id::text, ''), COALESCE(item.evidence_observation_id::text, ''),
	       item.observation_id::text,
       COALESCE(item.candidate_memory_id::text, ''),
       COALESCE(candidate.lifecycle_status, ''), item.created_at
FROM source_formation_items item
LEFT JOIN governed_memories candidate
  ON candidate.tenant_id = item.tenant_id
 AND candidate.continuity_id = item.continuity_id
 AND candidate.id = item.candidate_memory_id
WHERE item.tenant_id = $1 AND item.continuity_id = $2::uuid AND item.run_id = $3::uuid
ORDER BY item.ordinal ASC`, tenantID, continuityID, runID)
	if err != nil {
		return nil, fmt.Errorf("list source formation items: %w", err)
	}
	defer rows.Close()
	items := make([]SourceFormationItemReceipt, 0)
	for rows.Next() {
		var item SourceFormationItemReceipt
		if err := rows.Scan(
			&item.ID,
			&item.Ordinal,
			&item.Decision,
			&item.MemoryKey,
			&item.Quote,
			&item.Occurrence,
			&item.ByteStart,
			&item.ByteEnd,
			&item.Content,
			&item.Reason,
			&item.TargetMemoryID,
			&item.EvidenceObservationID,
			&item.ObservationID,
			&item.CandidateMemoryID,
			&item.CandidateStatus,
			&item.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan source formation item: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate source formation items: %w", err)
	}
	return items, nil
}

func scanSourceFormation(row sourceFormationRow) (SourceFormationReceipt, error) {
	var receipt SourceFormationReceipt
	var inputManifestJSON []byte
	var snapshotJSON []byte
	if err := row.Scan(
		&receipt.ID,
		&receipt.ContinuityID,
		&receipt.OperationID,
		&receipt.RequestFingerprint,
		&receipt.SourceRef,
		&receipt.SourceSHA256,
		&receipt.SourceBytes,
		&receipt.InputKind,
		&inputManifestJSON,
		&receipt.InputManifestFingerprint,
		&snapshotJSON,
		&receipt.ActiveSnapshotFingerprint,
		&receipt.ProviderName,
		&receipt.RequestedModel,
		&receipt.ResolvedModel,
		&receipt.Status,
		&receipt.ProviderOutput,
		&receipt.ProviderArtifactSHA256,
		&receipt.Reason,
		&receipt.FailureCode,
		&receipt.CreatedAt,
		&receipt.CompletedAt,
	); err != nil {
		return SourceFormationReceipt{}, err
	}
	if err := json.Unmarshal(inputManifestJSON, &receipt.InputManifest); err != nil {
		return SourceFormationReceipt{}, fmt.Errorf("decode source formation input manifest: %w", err)
	}
	if err := json.Unmarshal(snapshotJSON, &receipt.ActiveSnapshot); err != nil {
		return SourceFormationReceipt{}, fmt.Errorf("decode source formation active snapshot: %w", err)
	}
	return receipt, nil
}

func normalizeSourceFormationBeginRequest(request SourceFormationBeginRequest) (SourceFormationBeginRequest, error) {
	request.OperationID = strings.TrimSpace(request.OperationID)
	request.SourceRef = strings.TrimSpace(request.SourceRef)
	request.SourceSHA256 = strings.ToLower(strings.TrimSpace(request.SourceSHA256))
	request.ProviderName = strings.TrimSpace(request.ProviderName)
	request.RequestedModel = strings.TrimSpace(request.RequestedModel)
	if request.InputKind == "" {
		request.InputKind = SourceFormationInputDocument
	}
	if request.OperationID == "" || request.SourceRef == "" || request.ProviderName == "" || request.RequestedModel == "" {
		return SourceFormationBeginRequest{}, fmt.Errorf("operation_id, source_ref, provider_name, and requested_model are required")
	}
	if len(request.SourceRef) > 512 || strings.ContainsAny(request.SourceRef, "\r\n") {
		return SourceFormationBeginRequest{}, fmt.Errorf("source_ref must be one line of at most 512 bytes")
	}
	if request.SourceBytes <= 0 || request.SourceBytes > 65536 {
		return SourceFormationBeginRequest{}, fmt.Errorf("source_bytes must be between 1 and 65536")
	}
	if err := validateSourceFormationSHA256(request.SourceSHA256, "source"); err != nil {
		return SourceFormationBeginRequest{}, err
	}
	manifest, err := normalizeSourceFormationInputManifest(request.InputKind, request.InputManifest)
	if err != nil {
		return SourceFormationBeginRequest{}, err
	}
	request.InputManifest = manifest
	return request, nil
}

func normalizeSourceFormationCompletion(completion SourceFormationCompletion) (SourceFormationCompletion, error) {
	completion.ResolvedModel = strings.TrimSpace(completion.ResolvedModel)
	completion.ProviderOutput = strings.TrimSpace(completion.ProviderOutput)
	completion.ProviderArtifactSHA256 = strings.ToLower(strings.TrimSpace(completion.ProviderArtifactSHA256))
	completion.Reason = truncateSourceFormationText(strings.TrimSpace(completion.Reason), 512)
	completion.FailureCode = strings.TrimSpace(completion.FailureCode)
	if completion.ProviderArtifactSHA256 != "" {
		if err := validateSourceFormationSHA256(completion.ProviderArtifactSHA256, "provider artifact"); err != nil {
			return SourceFormationCompletion{}, err
		}
	}
	switch completion.Status {
	case SourceFormationCompleted:
		if completion.FailureCode != "" {
			return SourceFormationCompletion{}, fmt.Errorf("completed source formation cannot have failure_code")
		}
	case SourceFormationAbstained:
		if completion.Reason == "" || len(completion.Items) != 0 || completion.FailureCode != "" {
			return SourceFormationCompletion{}, fmt.Errorf("abstained source formation requires a reason and no items or failure_code")
		}
	case SourceFormationFailed:
		if completion.FailureCode == "" || len(completion.Items) != 0 {
			return SourceFormationCompletion{}, fmt.Errorf("failed source formation requires failure_code and no items")
		}
	default:
		return SourceFormationCompletion{}, fmt.Errorf("source formation status %q is unsupported", completion.Status)
	}
	return completion, nil
}

func sourceFormationRequestFingerprint(continuityID string, request SourceFormationBeginRequest) (string, error) {
	payload := struct {
		ContinuityID             string                   `json:"continuity_id"`
		SourceRef                string                   `json:"source_ref"`
		SourceSHA256             string                   `json:"source_sha256"`
		SourceBytes              int                      `json:"source_bytes"`
		InputKind                SourceFormationInputKind `json:"input_kind"`
		InputManifestFingerprint string                   `json:"input_manifest_fingerprint"`
		ProviderName             string                   `json:"provider_name"`
		RequestedModel           string                   `json:"requested_model"`
	}{
		ContinuityID:             continuityID,
		SourceRef:                request.SourceRef,
		SourceSHA256:             request.SourceSHA256,
		SourceBytes:              request.SourceBytes,
		InputKind:                request.InputKind,
		InputManifestFingerprint: sourceFormationInputManifestFingerprint(request.InputManifest),
		ProviderName:             request.ProviderName,
		RequestedModel:           request.RequestedModel,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("encode source formation request fingerprint: %w", err)
	}
	return sourceMatchSHA256(raw), nil
}

func normalizeSourceFormationInputManifest(kind SourceFormationInputKind, manifest []SourceFormationInputObservation) ([]SourceFormationInputObservation, error) {
	switch kind {
	case SourceFormationInputDocument:
		if len(manifest) != 0 {
			return nil, fmt.Errorf("document formation input manifest must be empty")
		}
		return []SourceFormationInputObservation{}, nil
	case SourceFormationInputConversation:
		if len(manifest) == 0 || len(manifest) > maxRecentConversationObservations {
			return nil, fmt.Errorf("conversation formation requires between 1 and %d input observations", maxRecentConversationObservations)
		}
	default:
		return nil, fmt.Errorf("source formation input kind %q is unsupported", kind)
	}

	normalized := make([]SourceFormationInputObservation, len(manifest))
	seen := make(map[string]struct{}, len(manifest))
	totalBytes := 0
	var previousSequence int64
	for index, raw := range manifest {
		entry := raw
		entry.ID = strings.TrimSpace(entry.ID)
		entry.SHA256 = strings.ToLower(strings.TrimSpace(entry.SHA256))
		if entry.Kind == "" {
			entry.Kind = ObservationKindUserMessage
		}
		if entry.ID == "" || len(entry.ID) > 64 {
			return nil, fmt.Errorf("conversation formation input observation %d has an invalid id", index+1)
		}
		if _, exists := seen[entry.ID]; exists {
			return nil, fmt.Errorf("conversation formation input contains duplicate observation %q", entry.ID)
		}
		seen[entry.ID] = struct{}{}
		if entry.Sequence <= previousSequence {
			return nil, fmt.Errorf("conversation formation input observations must follow authoritative sequence order")
		}
		previousSequence = entry.Sequence
		if !eligibleConversationFormationObservationKind(entry.Kind) {
			return nil, fmt.Errorf("conversation formation input observation %q has an ineligible kind", entry.ID)
		}
		if entry.Bytes <= 0 || entry.Bytes > 65536 {
			return nil, fmt.Errorf("conversation formation input observation %q has invalid byte length", entry.ID)
		}
		totalBytes += entry.Bytes
		if totalBytes > 65536 {
			return nil, fmt.Errorf("conversation formation input exceeds 65536 bytes")
		}
		if err := validateSourceFormationSHA256(entry.SHA256, "conversation observation"); err != nil {
			return nil, err
		}
		normalized[index] = entry
	}
	return normalized, nil
}

func mustSourceFormationInputManifestJSON(manifest []SourceFormationInputObservation) []byte {
	raw, err := json.Marshal(manifest)
	if err != nil {
		panic(err)
	}
	return raw
}

func sourceFormationInputManifestFingerprint(manifest []SourceFormationInputObservation) string {
	return sourceMatchSHA256(mustSourceFormationInputManifestJSON(manifest))
}

func verifyConversationFormationManifestTx(
	ctx context.Context,
	tx pgx.Tx,
	tenantID string,
	continuityID string,
	manifest []SourceFormationInputObservation,
) error {
	normalized, err := normalizeSourceFormationInputManifest(SourceFormationInputConversation, manifest)
	if err != nil {
		return err
	}
	for _, expected := range normalized {
		var sequence int64
		var kind ObservationKind
		var content string
		err := tx.QueryRow(ctx, `
SELECT observation_seq, observation_kind, content
FROM observations
WHERE tenant_id = $1 AND continuity_id = $2::uuid AND id::text = $3
FOR SHARE`, tenantID, continuityID, expected.ID).Scan(&sequence, &kind, &content)
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("conversation formation input observation %q does not belong to this continuity", expected.ID)
		}
		if err != nil {
			return fmt.Errorf("verify conversation formation input observation: %w", err)
		}
		if kind != expected.Kind || !eligibleConversationFormationObservationKind(kind) {
			return fmt.Errorf("conversation formation input observation %q is not eligible", expected.ID)
		}
		eligible, err := conversationFormationObservationCompleted(ctx, tx, tenantID, continuityID, expected.ID, kind)
		if err != nil {
			return err
		}
		if !eligible {
			return fmt.Errorf("conversation formation input observation %q is not eligible", expected.ID)
		}
		if content == "" || content == "[redacted]" || !utf8.ValidString(content) || strings.IndexByte(content, 0) >= 0 {
			return fmt.Errorf("conversation formation input observation %q is unavailable", expected.ID)
		}
		if sequence != expected.Sequence || len([]byte(content)) != expected.Bytes || sourceMatchSHA256([]byte(content)) != expected.SHA256 {
			return fmt.Errorf("conversation formation input observation %q changed", expected.ID)
		}
	}
	return nil
}

func (s *Store) SelectConversationFormationObservations(
	ctx context.Context,
	tenantID string,
	continuityID string,
	observationIDs []string,
	recentLimit int,
) ([]ConversationObservation, error) {
	ctx, err := withTenantContext(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	if len(observationIDs) > maxRecentConversationObservations {
		return nil, fmt.Errorf("conversation formation accepts at most %d observation IDs", maxRecentConversationObservations)
	}
	if recentLimit <= 0 {
		recentLimit = defaultRecentConversationObservations
	}
	if recentLimit > maxRecentConversationObservations {
		return nil, fmt.Errorf("conversation formation recent limit may not exceed %d", maxRecentConversationObservations)
	}

	observations := make([]ConversationObservation, 0, max(recentLimit, len(observationIDs)))
	if len(observationIDs) == 0 {
		rows, err := s.pool.Query(ctx, `
SELECT id::text, observation_seq, observation_kind, content
FROM observations
WHERE tenant_id = $1 AND continuity_id = $2::uuid
  AND observation_kind IN ('user_message', 'tool_result') AND content <> '[redacted]'
  AND (
    (observation_kind = 'user_message' AND EXISTS (
      SELECT 1 FROM conversation_turns turn
      WHERE turn.tenant_id = observations.tenant_id
        AND turn.continuity_id = observations.continuity_id
        AND turn.user_observation_id = observations.id
        AND turn.status = 'completed'
    ))
    OR
    (observation_kind = 'tool_result' AND EXISTS (
      SELECT 1
      FROM conversation_tool_results result
      JOIN conversation_turns turn
        ON turn.tenant_id = result.tenant_id
       AND turn.continuity_id = result.continuity_id
       AND turn.id = result.turn_id
      WHERE result.tenant_id = observations.tenant_id
        AND result.continuity_id = observations.continuity_id
        AND result.observation_id = observations.id
        AND turn.status = 'completed'
    ))
  )
ORDER BY observation_seq DESC
LIMIT $3`, tenantID, continuityID, recentLimit)
		if err != nil {
			return nil, fmt.Errorf("select recent conversation formation observations: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var observation ConversationObservation
			if err := rows.Scan(&observation.ID, &observation.Sequence, &observation.Kind, &observation.Content); err != nil {
				return nil, fmt.Errorf("scan recent conversation formation observation: %w", err)
			}
			observations = append(observations, observation)
		}
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("iterate recent conversation formation observations: %w", err)
		}
		slicesReverseConversationObservations(observations)
	} else {
		seen := make(map[string]struct{}, len(observationIDs))
		for _, rawID := range observationIDs {
			id := strings.TrimSpace(rawID)
			if id == "" || len(id) > 64 {
				return nil, fmt.Errorf("conversation formation observation_id is invalid")
			}
			if _, exists := seen[id]; exists {
				return nil, fmt.Errorf("conversation formation contains duplicate observation_id %q", id)
			}
			seen[id] = struct{}{}
			var observation ConversationObservation
			err := s.pool.QueryRow(ctx, `
SELECT id::text, observation_seq, observation_kind, content
FROM observations
WHERE tenant_id = $1 AND continuity_id = $2::uuid AND id::text = $3`, tenantID, continuityID, id).Scan(
				&observation.ID,
				&observation.Sequence,
				&observation.Kind,
				&observation.Content,
			)
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, fmt.Errorf("conversation formation observation %q does not belong to this continuity", id)
			}
			if err != nil {
				return nil, fmt.Errorf("select conversation formation observation: %w", err)
			}
			eligible, err := conversationFormationObservationCompleted(ctx, s.pool, tenantID, continuityID, observation.ID, observation.Kind)
			if err != nil {
				return nil, err
			}
			if !eligible {
				return nil, fmt.Errorf("conversation formation observation %q is not eligible", id)
			}
			observations = append(observations, observation)
		}
		sort.Slice(observations, func(i, j int) bool { return observations[i].Sequence < observations[j].Sequence })
	}
	if len(observations) == 0 {
		return nil, fmt.Errorf("conversation formation found no eligible user observations")
	}
	totalBytes := 0
	for _, observation := range observations {
		if !eligibleConversationFormationObservationKind(observation.Kind) {
			return nil, fmt.Errorf("conversation formation observation %q is not eligible", observation.ID)
		}
		if observation.Content == "" || observation.Content == "[redacted]" || !utf8.ValidString(observation.Content) || strings.IndexByte(observation.Content, 0) >= 0 {
			return nil, fmt.Errorf("conversation formation observation %q is unavailable", observation.ID)
		}
		totalBytes += len([]byte(observation.Content))
		if totalBytes > 65536 {
			return nil, fmt.Errorf("conversation formation input exceeds 65536 bytes")
		}
	}
	return observations, nil
}

func slicesReverseConversationObservations(observations []ConversationObservation) {
	for left, right := 0, len(observations)-1; left < right; left, right = left+1, right-1 {
		observations[left], observations[right] = observations[right], observations[left]
	}
}

func sourceFormationManifestFromObservations(observations []ConversationObservation) []SourceFormationInputObservation {
	manifest := make([]SourceFormationInputObservation, len(observations))
	for index, observation := range observations {
		manifest[index] = SourceFormationInputObservation{
			ID:       observation.ID,
			Sequence: observation.Sequence,
			Kind:     observation.Kind,
			SHA256:   sourceMatchSHA256([]byte(observation.Content)),
			Bytes:    len([]byte(observation.Content)),
		}
	}
	return manifest
}

type conversationFormationQueryRower interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func eligibleConversationFormationObservationKind(kind ObservationKind) bool {
	return kind == ObservationKindUserMessage || kind == ObservationKindToolResult
}

func conversationFormationObservationCompleted(
	ctx context.Context,
	query conversationFormationQueryRower,
	tenantID string,
	continuityID string,
	observationID string,
	kind ObservationKind,
) (bool, error) {
	var eligible bool
	if err := query.QueryRow(ctx, `
SELECT CASE
  WHEN $4 = 'user_message' THEN EXISTS (
    SELECT 1 FROM conversation_turns turn
    WHERE turn.tenant_id = $1 AND turn.continuity_id = $2::uuid
      AND turn.user_observation_id = $3::uuid AND turn.status = 'completed'
  )
  WHEN $4 = 'tool_result' THEN EXISTS (
    SELECT 1
    FROM conversation_tool_results result
    JOIN conversation_turns turn
      ON turn.tenant_id = result.tenant_id
     AND turn.continuity_id = result.continuity_id
     AND turn.id = result.turn_id
    WHERE result.tenant_id = $1 AND result.continuity_id = $2::uuid
      AND result.observation_id = $3::uuid AND turn.status = 'completed'
  )
  ELSE false
END`, tenantID, continuityID, observationID, kind).Scan(&eligible); err != nil {
		return false, fmt.Errorf("check conversation formation observation eligibility: %w", err)
	}
	return eligible, nil
}

func validateSourceFormationDocument(run SourceFormationReceipt, sourceDocument []byte) (string, string) {
	if len(sourceDocument) == 0 || len(sourceDocument) > 65536 {
		return "invalid_source_document", "source document must contain between 1 and 65536 bytes"
	}
	if !utf8.Valid(sourceDocument) || bytes.IndexByte(sourceDocument, 0) >= 0 {
		return "invalid_source_document", "source document must be valid UTF-8 without NUL bytes"
	}
	if len(sourceDocument) != run.SourceBytes || sourceMatchSHA256(sourceDocument) != run.SourceSHA256 {
		return "source_document_mismatch", "source document bytes do not match the bound source metadata"
	}
	return "", ""
}

func validateSourceFormationItems(
	inputKind SourceFormationInputKind,
	sourceDocument []byte,
	conversationEvidence map[string]ConversationObservation,
	items []SourceFormationProviderItem,
	snapshot []SourceMatchCandidate,
) ([]validatedSourceFormationItem, string, string) {
	if len(items) == 0 {
		return nil, "empty_formation_batch", "completed source formation requires at least one item"
	}
	if len(items) > 16 {
		return nil, "too_many_formation_items", "source formation may contain at most 16 items"
	}
	byKey := make(map[string][]SourceMatchCandidate, len(snapshot))
	for _, candidate := range snapshot {
		byKey[candidate.MemoryKey] = append(byKey[candidate.MemoryKey], candidate)
	}
	seenKeys := make(map[string]struct{}, len(items))
	validated := make([]validatedSourceFormationItem, 0, len(items))
	for _, raw := range items {
		item := raw
		item.MemoryKey = strings.TrimSpace(item.MemoryKey)
		item.SourceObservationID = strings.TrimSpace(item.SourceObservationID)
		item.Content = strings.TrimSpace(item.Content)
		item.Reason = strings.TrimSpace(item.Reason)
		if len(item.MemoryKey) == 0 || len(item.MemoryKey) > 160 || !sourceFormationMemoryKeyPattern.MatchString(item.MemoryKey) {
			return nil, "invalid_memory_key", "formation item memory_key is invalid"
		}
		if _, exists := seenKeys[item.MemoryKey]; exists {
			return nil, "duplicate_memory_key", "formation batch contains a duplicate memory_key"
		}
		seenKeys[item.MemoryKey] = struct{}{}
		if strings.TrimSpace(item.Quote) == "" || len(item.Quote) > 2048 {
			return nil, "invalid_quote", "formation item quote is required and may contain at most 2048 bytes"
		}
		if item.Occurrence <= 0 {
			return nil, "invalid_quote_occurrence", "formation item quote occurrence must be positive"
		}
		if item.Content == "" || len(item.Content) > 2048 {
			return nil, "invalid_content", "formation item content is required and may contain at most 2048 bytes"
		}
		if item.Reason == "" || len(item.Reason) > 512 {
			return nil, "invalid_reason", "formation item reason is required and may contain at most 512 bytes"
		}
		quoteSource := sourceDocument
		evidenceObservationID := ""
		switch inputKind {
		case SourceFormationInputDocument:
			if item.SourceObservationID != "" {
				return nil, "unexpected_evidence_observation", "document formation item cannot reference a conversation observation"
			}
		case SourceFormationInputConversation:
			if item.SourceObservationID == "" {
				return nil, "missing_evidence_observation", "conversation formation item requires source_observation_id"
			}
			evidence, exists := conversationEvidence[item.SourceObservationID]
			if !exists {
				return nil, "evidence_observation_outside_manifest", "conversation formation item references an observation outside the input manifest"
			}
			evidenceObservationID = evidence.ID
			quoteSource = []byte(evidence.Content)
		default:
			return nil, "invalid_input_kind", "source formation input kind is unsupported"
		}
		byteStart, byteEnd, ok := sourceFormationQuoteSpan(quoteSource, []byte(item.Quote), item.Occurrence)
		if !ok {
			return nil, "quote_occurrence_not_found", "formation item quote occurrence was not found exactly in its bound evidence"
		}
		entry := validatedSourceFormationItem{item: item, evidenceObservationID: evidenceObservationID, byteStart: byteStart, byteEnd: byteEnd}
		matches := byKey[item.MemoryKey]
		switch item.Decision {
		case SourceFormationNew:
			if len(matches) != 0 {
				return nil, "new_key_already_active", "new formation item targets an existing active memory_key"
			}
		case SourceFormationUpdate:
			if len(matches) != 1 {
				return nil, "update_target_not_unique", "update formation item requires exactly one active memory_key target"
			}
			if item.Content == matches[0].Content {
				return nil, "update_content_unchanged", "update formation item content must differ from the active fact"
			}
			entry.target = matches[0]
			entry.hasTarget = true
		case SourceFormationUnchanged:
			if len(matches) != 1 {
				return nil, "unchanged_target_not_unique", "unchanged formation item requires exactly one active memory_key target"
			}
			if item.Content != matches[0].Content {
				return nil, "unchanged_content_changed", "unchanged formation item content must equal the active fact"
			}
			entry.target = matches[0]
			entry.hasTarget = true
		default:
			return nil, "invalid_formation_decision", "formation item decision must be new, update, or unchanged"
		}
		for _, prior := range validated {
			if evidenceObservationID == prior.evidenceObservationID && byteStart < prior.byteEnd && prior.byteStart < byteEnd {
				return nil, "overlapping_source_spans", "formation item source spans must not overlap"
			}
		}
		validated = append(validated, entry)
	}
	return validated, "", ""
}

func loadConversationFormationEvidenceTx(
	ctx context.Context,
	tx pgx.Tx,
	tenantID string,
	continuityID string,
	manifest []SourceFormationInputObservation,
) (map[string]ConversationObservation, error) {
	evidence := make(map[string]ConversationObservation, len(manifest))
	for _, expected := range manifest {
		var observation ConversationObservation
		err := tx.QueryRow(ctx, `
SELECT id::text, observation_seq, observation_kind, content
FROM observations
WHERE tenant_id = $1 AND continuity_id = $2::uuid AND id::text = $3
FOR SHARE`, tenantID, continuityID, expected.ID).Scan(
			&observation.ID,
			&observation.Sequence,
			&observation.Kind,
			&observation.Content,
		)
		if err != nil {
			return nil, fmt.Errorf("load conversation formation evidence %q: %w", expected.ID, err)
		}
		evidence[observation.ID] = observation
	}
	return evidence, nil
}

func sourceFormationQuoteSpan(document, quote []byte, occurrence int) (int, int, bool) {
	searchStart := 0
	for current := 1; current <= occurrence; current++ {
		index := bytes.Index(document[searchStart:], quote)
		if index < 0 {
			return 0, 0, false
		}
		absolute := searchStart + index
		if current == occurrence {
			return absolute, absolute + len(quote), true
		}
		searchStart = absolute + len(quote)
	}
	return 0, 0, false
}

func validateSourceFormationSHA256(value, label string) error {
	if len(value) != 64 {
		return fmt.Errorf("%s SHA-256 must contain 64 hexadecimal characters", label)
	}
	if _, err := hex.DecodeString(value); err != nil {
		return fmt.Errorf("%s SHA-256 is invalid", label)
	}
	return nil
}

type sourceFormationEvidenceSpan struct {
	observationID string
	byteStart     int
	byteEnd       int
}

func redactSourceFormationMemoryTx(ctx context.Context, tx pgx.Tx, tenantID, continuityID, memoryID string) error {
	evidenceRows, err := tx.Query(ctx, `
SELECT DISTINCT evidence_observation_id::text, byte_start, byte_end
FROM source_formation_items
WHERE tenant_id = $1 AND continuity_id = $2::uuid
  AND candidate_memory_id = $3::uuid
	AND evidence_observation_id IS NOT NULL
ORDER BY evidence_observation_id::text, byte_start, byte_end`, tenantID, continuityID, memoryID)
	if err != nil {
		return fmt.Errorf("list source formation evidence spans for redaction: %w", err)
	}
	evidenceSpans := make([]sourceFormationEvidenceSpan, 0)
	evidenceObservationIDs := make([]string, 0)
	seenEvidenceObservationIDs := make(map[string]struct{})
	for evidenceRows.Next() {
		var span sourceFormationEvidenceSpan
		if err := evidenceRows.Scan(&span.observationID, &span.byteStart, &span.byteEnd); err != nil {
			evidenceRows.Close()
			return fmt.Errorf("scan source formation evidence span for redaction: %w", err)
		}
		evidenceSpans = append(evidenceSpans, span)
		if _, seen := seenEvidenceObservationIDs[span.observationID]; !seen {
			seenEvidenceObservationIDs[span.observationID] = struct{}{}
			evidenceObservationIDs = append(evidenceObservationIDs, span.observationID)
		}
	}
	if err := evidenceRows.Err(); err != nil {
		evidenceRows.Close()
		return fmt.Errorf("iterate source formation evidence spans for redaction: %w", err)
	}
	evidenceRows.Close()
	spansByObservation := make(map[string][]sourceFormationEvidenceSpan, len(evidenceObservationIDs))
	for _, span := range evidenceSpans {
		spansByObservation[span.observationID] = append(spansByObservation[span.observationID], span)
	}
	for _, observationID := range evidenceObservationIDs {
		shared, err := sourceFormationEvidenceHasSurvivingSiblingTx(ctx, tx, tenantID, continuityID, observationID, memoryID)
		if err != nil {
			return err
		}
		if shared {
			if err := redactSourceFormationEvidenceSpansTx(ctx, tx, tenantID, continuityID, observationID, spansByObservation[observationID]); err != nil {
				return err
			}
			continue
		}
		if err := redactConversationObservationTx(ctx, tx, tenantID, continuityID, observationID); err != nil {
			return err
		}
	}

	rows, err := tx.Query(ctx, `
SELECT run.id::text, run.active_snapshot, run.status, run.failure_code
FROM source_formation_runs run
WHERE run.tenant_id = $1 AND run.continuity_id = $2::uuid
  AND (
    run.active_snapshot @> jsonb_build_array(jsonb_build_object('memory_id', $3::text))
    OR EXISTS (
      SELECT 1
      FROM source_formation_items item
      WHERE item.tenant_id = run.tenant_id
        AND item.continuity_id = run.continuity_id
        AND item.run_id = run.id
        AND (item.target_memory_id = $3::uuid OR item.candidate_memory_id = $3::uuid)
    )
    OR (
      run.input_kind = 'conversation'
      AND EXISTS (
        SELECT 1
        FROM jsonb_array_elements(run.input_manifest) manifest
        WHERE manifest->>'id' = ANY($4::text[])
      )
    )
  )
FOR UPDATE`, tenantID, continuityID, memoryID, evidenceObservationIDs)
	if err != nil {
		return fmt.Errorf("list source formation audit rows for redaction: %w", err)
	}
	type affectedRun struct {
		id          string
		snapshot    []SourceMatchCandidate
		status      SourceFormationStatus
		failureCode string
	}
	affected := make([]affectedRun, 0)
	for rows.Next() {
		var run affectedRun
		var snapshotJSON []byte
		if err := rows.Scan(&run.id, &snapshotJSON, &run.status, &run.failureCode); err != nil {
			rows.Close()
			return fmt.Errorf("scan source formation audit row for redaction: %w", err)
		}
		if err := json.Unmarshal(snapshotJSON, &run.snapshot); err != nil {
			rows.Close()
			return fmt.Errorf("decode source formation snapshot for redaction: %w", err)
		}
		affected = append(affected, run)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("iterate source formation audit rows for redaction: %w", err)
	}
	rows.Close()

	for _, run := range affected {
		for index := range run.snapshot {
			if run.snapshot[index].MemoryID == memoryID {
				run.snapshot[index].Content = "[redacted]"
				run.snapshot[index].SourceRef = "[redacted]"
			}
		}
		snapshotJSON, snapshotFingerprint, err := canonicalSourceMatchCandidates(run.snapshot)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
UPDATE observations observation
SET content = '[redacted]', source_ref = '[redacted]'
WHERE observation.tenant_id = $1
  AND observation.continuity_id = $2::uuid
  AND observation.id IN (
    SELECT item.observation_id
    FROM source_formation_items item
    WHERE item.tenant_id = $1
      AND item.continuity_id = $2::uuid
      AND item.run_id = $3::uuid
      AND (item.target_memory_id = $4::uuid OR item.candidate_memory_id = $4::uuid)
  )`, tenantID, continuityID, run.id, memoryID); err != nil {
			return fmt.Errorf("redact source formation observations: %w", err)
		}
		if _, err := tx.Exec(ctx, `
UPDATE source_formation_items
SET quote = '[redacted]',
    byte_end = byte_start + octet_length('[redacted]'),
    content = '[redacted]',
    reason = '[redacted]'
WHERE tenant_id = $1 AND continuity_id = $2::uuid AND run_id = $3::uuid
  AND (target_memory_id = $4::uuid OR candidate_memory_id = $4::uuid)`, tenantID, continuityID, run.id, memoryID); err != nil {
			return fmt.Errorf("redact source formation items: %w", err)
		}
		status := run.status
		failureCode := run.failureCode
		reason := "[redacted]"
		completePending := false
		if run.status == SourceFormationPending {
			status = SourceFormationFailed
			failureCode = "referenced_memory_deleted"
			reason = "referenced memory was deleted during source formation"
			completePending = true
		}
		if _, err := tx.Exec(ctx, `
UPDATE source_formation_runs
SET active_snapshot = $3::jsonb,
    active_snapshot_fingerprint = $4,
    source_ref = '[redacted]',
    provider_output = '[redacted]',
    reason = $5,
    status = $6,
    failure_code = $7,
    completed_at = CASE WHEN $8 THEN now() ELSE completed_at END
WHERE id = $1::uuid AND tenant_id = $2`, run.id, tenantID, snapshotJSON, snapshotFingerprint, reason, status, failureCode, completePending); err != nil {
			return fmt.Errorf("redact forgotten memory from source formation audit: %w", err)
		}
	}
	return nil
}

func sourceFormationEvidenceHasSurvivingSiblingTx(
	ctx context.Context,
	tx pgx.Tx,
	tenantID string,
	continuityID string,
	observationID string,
	memoryID string,
) (bool, error) {
	var shared bool
	if err := tx.QueryRow(ctx, `
SELECT EXISTS (
  SELECT 1
  FROM source_formation_items item
  LEFT JOIN governed_memories candidate
    ON candidate.tenant_id = item.tenant_id
   AND candidate.continuity_id = item.continuity_id
   AND candidate.id = item.candidate_memory_id
  LEFT JOIN governed_memories target
    ON target.tenant_id = item.tenant_id
   AND target.continuity_id = item.continuity_id
   AND target.id = item.target_memory_id
  WHERE item.tenant_id = $1
    AND item.continuity_id = $2::uuid
    AND item.evidence_observation_id = $3::uuid
    AND item.candidate_memory_id IS DISTINCT FROM $4::uuid
    AND (
      (item.candidate_memory_id IS NOT NULL AND candidate.lifecycle_status <> 'deleted')
      OR
      (item.candidate_memory_id IS NULL AND item.target_memory_id IS NOT NULL AND target.lifecycle_status <> 'deleted')
    )
)`, tenantID, continuityID, observationID, memoryID).Scan(&shared); err != nil {
		return false, fmt.Errorf("inspect shared source formation evidence: %w", err)
	}
	return shared, nil
}

func redactSourceFormationEvidenceSpansTx(
	ctx context.Context,
	tx pgx.Tx,
	tenantID string,
	continuityID string,
	observationID string,
	spans []sourceFormationEvidenceSpan,
) error {
	var content string
	if err := tx.QueryRow(ctx, `
SELECT content
FROM observations
WHERE tenant_id = $1 AND continuity_id = $2::uuid AND id = $3::uuid
FOR UPDATE`, tenantID, continuityID, observationID).Scan(&content); err != nil {
		return fmt.Errorf("load shared source formation evidence for redaction: %w", err)
	}
	contentBytes := []byte(content)
	sort.Slice(spans, func(left, right int) bool {
		if spans[left].byteStart == spans[right].byteStart {
			return spans[left].byteEnd < spans[right].byteEnd
		}
		return spans[left].byteStart < spans[right].byteStart
	})
	lastEnd := -1
	for _, span := range spans {
		if span.byteStart < 0 || span.byteEnd <= span.byteStart || span.byteEnd > len(contentBytes) {
			return fmt.Errorf("source formation evidence span [%d,%d) is outside observation %s", span.byteStart, span.byteEnd, observationID)
		}
		if span.byteStart < lastEnd {
			return fmt.Errorf("source formation evidence redaction spans overlap for observation %s", observationID)
		}
		copy(contentBytes[span.byteStart:span.byteEnd], sourceFormationRedactionBytes(span.byteEnd-span.byteStart))
		lastEnd = span.byteEnd
	}
	if !utf8.Valid(contentBytes) {
		return fmt.Errorf("source formation evidence redaction produced invalid UTF-8 for observation %s", observationID)
	}
	if _, err := tx.Exec(ctx, `
UPDATE observations
SET content = $4
WHERE tenant_id = $1 AND continuity_id = $2::uuid AND id = $3::uuid`, tenantID, continuityID, observationID, string(contentBytes)); err != nil {
		return fmt.Errorf("redact shared source formation evidence spans: %w", err)
	}
	return nil
}

func sourceFormationRedactionBytes(length int) []byte {
	redacted := bytes.Repeat([]byte{'*'}, length)
	if length >= len("[redacted]") {
		copy(redacted, "[redacted]")
	}
	return redacted
}

func truncateSourceFormationText(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	end := limit
	for end > 0 && !utf8.ValidString(value[:end]) {
		end--
	}
	return value[:end]
}
