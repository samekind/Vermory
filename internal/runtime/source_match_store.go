package runtime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

const sourceMatchSelectColumns = `
id::text, continuity_id::text, operation_id, request_fingerprint,
source_ref, source_content, candidate_set, candidate_set_fingerprint,
provider_name, requested_model, resolved_model, status, provider_output,
provider_artifact_sha256, reason, failure_code, selected_memory_key,
COALESCE(target_memory_id::text, ''), COALESCE(observation_id::text, ''),
COALESCE(candidate_memory_id::text, ''), created_at, completed_at`

type sourceMatchRow interface {
	Scan(dest ...any) error
}

const sourceMatchPendingExpiry = defaultSourceMatchProviderTimeout + time.Minute

func (s *Store) BeginSourceMatch(ctx context.Context, tenantID, continuityID string, request SourceMatchBeginRequest) (SourceMatchReceipt, error) {
	ctx, err := withTenantContext(ctx, tenantID)
	if err != nil {
		return SourceMatchReceipt{}, err
	}
	request, err = normalizeSourceMatchBeginRequest(request)
	if err != nil {
		return SourceMatchReceipt{}, err
	}
	fingerprint, err := sourceMatchRequestFingerprint(continuityID, request)
	if err != nil {
		return SourceMatchReceipt{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return SourceMatchReceipt{}, fmt.Errorf("begin source match: %w", err)
	}
	defer tx.Rollback(ctx)
	snapshot, err := currentEligibilitySnapshotTx(ctx, tx)
	if err != nil {
		return SourceMatchReceipt{}, err
	}

	existing, found, err := lookupSourceMatchOperationTx(ctx, tx, tenantID, request.OperationID)
	if err != nil {
		return SourceMatchReceipt{}, err
	}
	if found {
		if existing.ContinuityID != continuityID || existing.RequestFingerprint != fingerprint {
			return SourceMatchReceipt{}, fmt.Errorf("operation_id is already bound to another logical source match")
		}
		currentCandidates, err := listSourceMatchCandidatesTx(ctx, tx, tenantID, continuityID, snapshot.AsOf, false)
		if err != nil {
			return SourceMatchReceipt{}, err
		}
		_, currentCandidateFingerprint, err := canonicalSourceMatchCandidates(currentCandidates)
		if err != nil {
			return SourceMatchReceipt{}, err
		}
		if currentCandidateFingerprint != existing.CandidateSetFingerprint {
			return SourceMatchReceipt{}, fmt.Errorf("operation_id candidate snapshot has changed")
		}
		if existing.Status == SourceMatchPending && time.Since(existing.CreatedAt) >= sourceMatchPendingExpiry {
			expired, err := updateTerminalSourceMatch(ctx, tx, tenantID, existing.ID, SourceMatchCompletion{
				Decision:      SourceMatchFailed,
				ResolvedModel: existing.ResolvedModel,
				Reason:        "previous source match attempt expired before completion",
				FailureCode:   "pending_expired",
			}, "", "", "", "", "")
			if err != nil {
				return SourceMatchReceipt{}, err
			}
			expired.Replayed = true
			if err := tx.Commit(ctx); err != nil {
				return SourceMatchReceipt{}, fmt.Errorf("commit expired source match: %w", err)
			}
			return expired, nil
		}
		existing.Replayed = true
		if err := tx.Commit(ctx); err != nil {
			return SourceMatchReceipt{}, fmt.Errorf("commit replayed source match: %w", err)
		}
		return existing, nil
	}

	var validContinuity bool
	if err := tx.QueryRow(ctx, `
SELECT EXISTS (
  SELECT 1 FROM continuity_spaces
  WHERE id = $1::uuid AND tenant_id = $2
    AND continuity_line = 'workspace' AND state = 'active'
)`, continuityID, tenantID).Scan(&validContinuity); err != nil {
		return SourceMatchReceipt{}, fmt.Errorf("check source match continuity: %w", err)
	}
	if !validContinuity {
		return SourceMatchReceipt{}, fmt.Errorf("workspace continuity is not active for this tenant")
	}
	candidates, err := listSourceMatchCandidatesTx(ctx, tx, tenantID, continuityID, snapshot.AsOf, false)
	if err != nil {
		return SourceMatchReceipt{}, err
	}
	candidateJSON, candidateFingerprint, err := canonicalSourceMatchCandidates(candidates)
	if err != nil {
		return SourceMatchReceipt{}, err
	}

	row := tx.QueryRow(ctx, `
INSERT INTO source_match_decisions (
  tenant_id, continuity_id, operation_id, request_fingerprint,
  source_ref, source_content, candidate_set, candidate_set_fingerprint,
  provider_name, requested_model, status
)
VALUES ($1, $2::uuid, $3, $4, $5, $6, $7::jsonb, $8, $9, $10, 'pending')
RETURNING `+sourceMatchSelectColumns,
		tenantID,
		continuityID,
		request.OperationID,
		fingerprint,
		request.SourceRef,
		request.SourceContent,
		candidateJSON,
		candidateFingerprint,
		request.ProviderName,
		request.RequestedModel,
	)
	receipt, err := scanSourceMatch(row)
	if err != nil {
		return SourceMatchReceipt{}, fmt.Errorf("insert source match decision: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return SourceMatchReceipt{}, fmt.Errorf("commit source match begin: %w", err)
	}
	return receipt, nil
}

func (s *Store) CompleteSourceMatch(ctx context.Context, tenantID, decisionID string, completion SourceMatchCompletion) (SourceMatchReceipt, error) {
	ctx, err := withTenantContext(ctx, tenantID)
	if err != nil {
		return SourceMatchReceipt{}, err
	}
	decisionID = strings.TrimSpace(decisionID)
	if decisionID == "" {
		return SourceMatchReceipt{}, fmt.Errorf("source match decision_id is required")
	}
	completion, err = normalizeSourceMatchCompletion(completion)
	if err != nil {
		return SourceMatchReceipt{}, err
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return SourceMatchReceipt{}, fmt.Errorf("begin source match completion: %w", err)
	}
	defer tx.Rollback(ctx)
	snapshot, err := currentEligibilitySnapshotTx(ctx, tx)
	if err != nil {
		return SourceMatchReceipt{}, err
	}

	decision, err := scanSourceMatch(tx.QueryRow(ctx, `
SELECT `+sourceMatchSelectColumns+`
FROM source_match_decisions
WHERE id = $1::uuid AND tenant_id = $2
FOR UPDATE`, decisionID, tenantID))
	if errors.Is(err, pgx.ErrNoRows) {
		return SourceMatchReceipt{}, fmt.Errorf("source match decision does not belong to this tenant")
	}
	if err != nil {
		return SourceMatchReceipt{}, fmt.Errorf("lock source match decision: %w", err)
	}
	if decision.Status != SourceMatchPending {
		decision.Replayed = true
		if err := tx.Commit(ctx); err != nil {
			return SourceMatchReceipt{}, fmt.Errorf("commit replayed source match completion: %w", err)
		}
		return decision, nil
	}

	if completion.Decision == SourceMatchMatched {
		return completeMatchedSourceMatch(ctx, tx, tenantID, decision, completion, snapshot.AsOf)
	}
	receipt, err := updateTerminalSourceMatch(ctx, tx, tenantID, decision.ID, completion, "", "", "", "", "")
	if err != nil {
		return SourceMatchReceipt{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return SourceMatchReceipt{}, fmt.Errorf("commit terminal source match: %w", err)
	}
	return receipt, nil
}

func (s *Store) InspectSourceMatch(ctx context.Context, tenantID, continuityID, operationID string) (SourceMatchReceipt, error) {
	ctx, err := withTenantContext(ctx, tenantID)
	if err != nil {
		return SourceMatchReceipt{}, err
	}
	operationID = strings.TrimSpace(operationID)
	if operationID == "" {
		return SourceMatchReceipt{}, fmt.Errorf("source match operation_id is required")
	}
	receipt, err := scanSourceMatch(s.pool.QueryRow(ctx, `
SELECT `+sourceMatchSelectColumns+`
FROM source_match_decisions
WHERE tenant_id = $1 AND continuity_id = $2::uuid AND operation_id = $3`, tenantID, continuityID, operationID))
	if errors.Is(err, pgx.ErrNoRows) {
		return SourceMatchReceipt{}, fmt.Errorf("source match decision does not belong to this workspace continuity")
	}
	if err != nil {
		return SourceMatchReceipt{}, fmt.Errorf("inspect source match decision: %w", err)
	}
	return receipt, nil
}

func completeMatchedSourceMatch(ctx context.Context, tx pgx.Tx, tenantID string, decision SourceMatchReceipt, completion SourceMatchCompletion, asOf time.Time) (SourceMatchReceipt, error) {
	currentCandidates, err := listSourceMatchCandidatesTx(ctx, tx, tenantID, decision.ContinuityID, asOf, true)
	if err != nil {
		return SourceMatchReceipt{}, err
	}
	_, currentFingerprint, err := canonicalSourceMatchCandidates(currentCandidates)
	if err != nil {
		return SourceMatchReceipt{}, err
	}
	if currentFingerprint != decision.CandidateSetFingerprint {
		completion.Decision = SourceMatchFailed
		completion.FailureCode = "candidate_set_changed"
		completion.Reason = "active keyed facts changed while the provider was deciding"
		receipt, err := updateTerminalSourceMatch(ctx, tx, tenantID, decision.ID, completion, completion.SelectedMemoryKey, "", "", "", "")
		if err != nil {
			return SourceMatchReceipt{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return SourceMatchReceipt{}, fmt.Errorf("commit drifted source match: %w", err)
		}
		return receipt, nil
	}

	matches := make([]SourceMatchCandidate, 0, 1)
	for _, candidate := range decision.CandidateSet {
		if candidate.MemoryKey == completion.SelectedMemoryKey {
			matches = append(matches, candidate)
		}
	}
	if len(matches) != 1 {
		completion.Decision = SourceMatchFailed
		if len(matches) == 0 {
			completion.FailureCode = "selected_key_outside_candidate_set"
			completion.Reason = "provider selected a key outside the closed candidate set"
		} else {
			completion.FailureCode = "selected_key_is_ambiguous"
			completion.Reason = "provider selected a key with multiple active candidate rows"
		}
		receipt, err := updateTerminalSourceMatch(ctx, tx, tenantID, decision.ID, completion, completion.SelectedMemoryKey, "", "", "", "")
		if err != nil {
			return SourceMatchReceipt{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return SourceMatchReceipt{}, fmt.Errorf("commit invalid source match: %w", err)
		}
		return receipt, nil
	}

	target := matches[0]
	request := CommitObservationRequest{
		OperationID:        "source-match:" + decision.ID,
		Kind:               ObservationKindSourceCandidate,
		Content:            decision.SourceContent,
		SourceRef:          decision.SourceRef,
		MemoryKey:          completion.SelectedMemoryKey,
		SupersedesMemoryID: target.MemoryID,
	}
	if err := request.Validate(); err != nil {
		return SourceMatchReceipt{}, err
	}
	observation, err := commitObservationTx(ctx, tx, tenantID, decision.ContinuityID, request)
	if err != nil {
		return SourceMatchReceipt{}, err
	}
	candidateID := ""
	disposition := SourceCandidateUnchanged
	if target.Content != decision.SourceContent {
		memory, err := governObservationTx(ctx, tx, tenantID, decision.ContinuityID, observation.ObservationID, request)
		if err != nil {
			return SourceMatchReceipt{}, err
		}
		candidateID = memory.MemoryID
		disposition = SourceCandidateReplacement
	}
	receipt, err := updateTerminalSourceMatch(
		ctx,
		tx,
		tenantID,
		decision.ID,
		completion,
		completion.SelectedMemoryKey,
		target.MemoryID,
		observation.ObservationID,
		candidateID,
		disposition,
	)
	if err != nil {
		return SourceMatchReceipt{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return SourceMatchReceipt{}, fmt.Errorf("commit matched source candidate: %w", err)
	}
	return receipt, nil
}

func updateTerminalSourceMatch(
	ctx context.Context,
	tx pgx.Tx,
	tenantID string,
	decisionID string,
	completion SourceMatchCompletion,
	selectedKey string,
	targetMemoryID string,
	observationID string,
	candidateMemoryID string,
	disposition SourceCandidateDisposition,
) (SourceMatchReceipt, error) {
	receipt, err := scanSourceMatch(tx.QueryRow(ctx, `
UPDATE source_match_decisions
SET resolved_model = $3,
    status = $4,
    provider_output = $5,
    provider_artifact_sha256 = $6,
    reason = $7,
    failure_code = $8,
    selected_memory_key = $9,
    target_memory_id = NULLIF($10, '')::uuid,
    observation_id = NULLIF($11, '')::uuid,
    candidate_memory_id = NULLIF($12, '')::uuid,
    completed_at = now()
WHERE id = $1::uuid AND tenant_id = $2 AND status = 'pending'
RETURNING `+sourceMatchSelectColumns,
		decisionID,
		tenantID,
		completion.ResolvedModel,
		completion.Decision,
		completion.ProviderOutput,
		completion.ProviderArtifactSHA256,
		completion.Reason,
		completion.FailureCode,
		selectedKey,
		targetMemoryID,
		observationID,
		candidateMemoryID,
	))
	if err != nil {
		return SourceMatchReceipt{}, fmt.Errorf("complete source match decision: %w", err)
	}
	receipt.Disposition = disposition
	return receipt, nil
}

func lookupSourceMatchOperationTx(ctx context.Context, tx pgx.Tx, tenantID, operationID string) (SourceMatchReceipt, bool, error) {
	receipt, err := scanSourceMatch(tx.QueryRow(ctx, `
SELECT `+sourceMatchSelectColumns+`
FROM source_match_decisions
WHERE tenant_id = $1 AND operation_id = $2
FOR UPDATE`, tenantID, operationID))
	if errors.Is(err, pgx.ErrNoRows) {
		return SourceMatchReceipt{}, false, nil
	}
	if err != nil {
		return SourceMatchReceipt{}, false, fmt.Errorf("lookup source match operation: %w", err)
	}
	return receipt, true, nil
}

func listSourceMatchCandidatesTx(ctx context.Context, tx pgx.Tx, tenantID, continuityID string, asOf time.Time, lock bool) ([]SourceMatchCandidate, error) {
	query := `
SELECT memory.id::text, memory.memory_key, memory.content,
       COALESCE(observation.source_ref, '')
FROM governed_memories memory
LEFT JOIN observations observation
  ON observation.tenant_id = memory.tenant_id
 AND observation.id = memory.origin_observation_id
WHERE memory.tenant_id = $1 AND memory.continuity_id = $2::uuid
	AND memory_is_eligible(
	  memory.lifecycle_status, memory.content, memory.valid_from, memory.valid_until, $3
	)
	AND btrim(memory.memory_key) <> ''
ORDER BY memory.memory_key ASC, memory.id ASC`
	if lock {
		query += " FOR UPDATE OF memory"
	}
	rows, err := tx.Query(ctx, query, tenantID, continuityID, asOf)
	if err != nil {
		return nil, fmt.Errorf("list source match candidates: %w", err)
	}
	defer rows.Close()
	candidates := make([]SourceMatchCandidate, 0)
	for rows.Next() {
		var candidate SourceMatchCandidate
		if err := rows.Scan(&candidate.MemoryID, &candidate.MemoryKey, &candidate.Content, &candidate.SourceRef); err != nil {
			return nil, fmt.Errorf("scan source match candidate: %w", err)
		}
		candidates = append(candidates, candidate)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate source match candidates: %w", err)
	}
	return candidates, nil
}

func scanSourceMatch(row sourceMatchRow) (SourceMatchReceipt, error) {
	var receipt SourceMatchReceipt
	var candidateJSON []byte
	err := row.Scan(
		&receipt.ID,
		&receipt.ContinuityID,
		&receipt.OperationID,
		&receipt.RequestFingerprint,
		&receipt.SourceRef,
		&receipt.SourceContent,
		&candidateJSON,
		&receipt.CandidateSetFingerprint,
		&receipt.ProviderName,
		&receipt.RequestedModel,
		&receipt.ResolvedModel,
		&receipt.Status,
		&receipt.ProviderOutput,
		&receipt.ProviderArtifactSHA256,
		&receipt.Reason,
		&receipt.FailureCode,
		&receipt.SelectedMemoryKey,
		&receipt.TargetMemoryID,
		&receipt.ObservationID,
		&receipt.CandidateMemoryID,
		&receipt.CreatedAt,
		&receipt.CompletedAt,
	)
	if err != nil {
		return SourceMatchReceipt{}, err
	}
	if err := json.Unmarshal(candidateJSON, &receipt.CandidateSet); err != nil {
		return SourceMatchReceipt{}, fmt.Errorf("decode source match candidate set: %w", err)
	}
	if receipt.Status == SourceMatchMatched {
		if receipt.CandidateMemoryID == "" {
			receipt.Disposition = SourceCandidateUnchanged
		} else {
			receipt.Disposition = SourceCandidateReplacement
		}
	}
	return receipt, nil
}

func normalizeSourceMatchBeginRequest(request SourceMatchBeginRequest) (SourceMatchBeginRequest, error) {
	request.OperationID = strings.TrimSpace(request.OperationID)
	request.SourceRef = strings.TrimSpace(request.SourceRef)
	request.SourceContent = strings.TrimSpace(request.SourceContent)
	request.ProviderName = strings.TrimSpace(request.ProviderName)
	request.RequestedModel = strings.TrimSpace(request.RequestedModel)
	if request.OperationID == "" || request.SourceRef == "" || request.SourceContent == "" || request.ProviderName == "" || request.RequestedModel == "" {
		return SourceMatchBeginRequest{}, fmt.Errorf("operation_id, source_ref, source_content, provider_name, and requested_model are required")
	}
	return request, nil
}

func normalizeSourceMatchCompletion(completion SourceMatchCompletion) (SourceMatchCompletion, error) {
	completion.SelectedMemoryKey = strings.TrimSpace(completion.SelectedMemoryKey)
	completion.ResolvedModel = strings.TrimSpace(completion.ResolvedModel)
	completion.ProviderOutput = strings.TrimSpace(completion.ProviderOutput)
	completion.ProviderArtifactSHA256 = strings.TrimSpace(completion.ProviderArtifactSHA256)
	completion.Reason = truncateSourceMatchText(strings.TrimSpace(completion.Reason), 2000)
	completion.FailureCode = strings.TrimSpace(completion.FailureCode)
	if completion.ProviderArtifactSHA256 != "" {
		if len(completion.ProviderArtifactSHA256) != 64 {
			return SourceMatchCompletion{}, fmt.Errorf("provider artifact SHA-256 must contain 64 hexadecimal characters")
		}
		if _, err := hex.DecodeString(completion.ProviderArtifactSHA256); err != nil {
			return SourceMatchCompletion{}, fmt.Errorf("provider artifact SHA-256 is invalid")
		}
	}
	switch completion.Decision {
	case SourceMatchMatched:
		if completion.SelectedMemoryKey == "" {
			return SourceMatchCompletion{}, fmt.Errorf("selected memory_key is required for a matched source")
		}
		completion.FailureCode = ""
	case SourceMatchAbstained:
		if completion.SelectedMemoryKey != "" || completion.Reason == "" {
			return SourceMatchCompletion{}, fmt.Errorf("an abstained source match requires an empty key and a reason")
		}
		completion.FailureCode = ""
	case SourceMatchFailed:
		if completion.FailureCode == "" {
			return SourceMatchCompletion{}, fmt.Errorf("failure_code is required for a failed source match")
		}
	default:
		return SourceMatchCompletion{}, fmt.Errorf("source match completion decision %q is unsupported", completion.Decision)
	}
	return completion, nil
}

func sourceMatchRequestFingerprint(continuityID string, request SourceMatchBeginRequest) (string, error) {
	payload := struct {
		ContinuityID   string `json:"continuity_id"`
		SourceRef      string `json:"source_ref"`
		SourceContent  string `json:"source_content"`
		ProviderName   string `json:"provider_name"`
		RequestedModel string `json:"requested_model"`
	}{continuityID, request.SourceRef, request.SourceContent, request.ProviderName, request.RequestedModel}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("encode source match request fingerprint: %w", err)
	}
	return sourceMatchSHA256(raw), nil
}

func canonicalSourceMatchCandidates(candidates []SourceMatchCandidate) ([]byte, string, error) {
	raw, err := json.Marshal(candidates)
	if err != nil {
		return nil, "", fmt.Errorf("encode source match candidate set: %w", err)
	}
	return raw, sourceMatchSHA256(raw), nil
}

func sourceMatchSHA256(raw []byte) string {
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}

func truncateSourceMatchText(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return value[:limit]
}

func redactSourceMatchMemoryTx(ctx context.Context, tx pgx.Tx, tenantID, continuityID, memoryID, memoryContent string) error {
	rows, err := tx.Query(ctx, `
SELECT id::text, candidate_set, status, source_content,
       COALESCE(candidate_memory_id::text, '')
FROM source_match_decisions
WHERE tenant_id = $1 AND continuity_id = $2::uuid
  AND (
    target_memory_id = $3::uuid
    OR candidate_memory_id = $3::uuid
    OR candidate_set @> jsonb_build_array(jsonb_build_object('memory_id', $3))
  )
FOR UPDATE`, tenantID, continuityID, memoryID)
	if err != nil {
		return fmt.Errorf("list source match audit rows for redaction: %w", err)
	}
	type affectedDecision struct {
		id                string
		candidates        []SourceMatchCandidate
		status            SourceMatchStatus
		sourceContent     string
		candidateMemoryID string
	}
	affected := make([]affectedDecision, 0)
	for rows.Next() {
		var decision affectedDecision
		var candidateJSON []byte
		if err := rows.Scan(&decision.id, &candidateJSON, &decision.status, &decision.sourceContent, &decision.candidateMemoryID); err != nil {
			rows.Close()
			return fmt.Errorf("scan source match audit row for redaction: %w", err)
		}
		if err := json.Unmarshal(candidateJSON, &decision.candidates); err != nil {
			rows.Close()
			return fmt.Errorf("decode source match audit candidates for redaction: %w", err)
		}
		affected = append(affected, decision)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("iterate source match audit rows for redaction: %w", err)
	}
	rows.Close()

	for _, decision := range affected {
		for index := range decision.candidates {
			if decision.candidates[index].MemoryID == memoryID {
				decision.candidates[index].Content = "[redacted]"
				decision.candidates[index].SourceRef = "[redacted]"
			}
		}
		candidateJSON, candidateFingerprint, err := canonicalSourceMatchCandidates(decision.candidates)
		if err != nil {
			return err
		}
		redactSource := decision.candidateMemoryID == memoryID || decision.sourceContent == memoryContent
		status := decision.status
		failureCode := ""
		reason := "[redacted]"
		completePending := false
		if decision.status == SourceMatchPending {
			status = SourceMatchFailed
			failureCode = "referenced_memory_deleted"
			reason = "referenced memory was deleted during source matching"
			completePending = true
		}
		if _, err := tx.Exec(ctx, `
UPDATE source_match_decisions
SET candidate_set = $3::jsonb,
    candidate_set_fingerprint = $4,
    source_ref = CASE WHEN $5 THEN '[redacted]' ELSE source_ref END,
    source_content = CASE WHEN $5 THEN '[redacted]' ELSE source_content END,
    provider_output = '[redacted]',
    reason = $6,
    status = $7,
    failure_code = $8,
    completed_at = CASE WHEN $9 THEN now() ELSE completed_at END
WHERE id = $1::uuid AND tenant_id = $2`,
			decision.id,
			tenantID,
			candidateJSON,
			candidateFingerprint,
			redactSource,
			reason,
			status,
			failureCode,
			completePending,
		); err != nil {
			return fmt.Errorf("redact forgotten memory from source match audit: %w", err)
		}
	}
	return nil
}
