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

const (
	MemoryEligibilityActionSetValidity = "set_validity"
	MemoryEligibilityActionArchive     = "archive"
)

type SetMemoryValidityRequest struct {
	OperationID  string
	TenantID     string
	ContinuityID string
	MemoryID     string
	ValidFrom    *time.Time
	ValidUntil   *time.Time
}

func (r SetMemoryValidityRequest) Validate() error {
	_, err := r.normalized()
	return err
}

func (r SetMemoryValidityRequest) normalized() (SetMemoryValidityRequest, error) {
	r.OperationID = strings.TrimSpace(r.OperationID)
	r.TenantID = strings.TrimSpace(r.TenantID)
	r.ContinuityID = strings.TrimSpace(r.ContinuityID)
	r.MemoryID = strings.TrimSpace(r.MemoryID)
	if err := validateMemoryEligibilityIDs(r.OperationID, r.TenantID, r.ContinuityID, r.MemoryID); err != nil {
		return SetMemoryValidityRequest{}, err
	}
	if err := requireUTCEligibilityTime("valid_from", r.ValidFrom); err != nil {
		return SetMemoryValidityRequest{}, err
	}
	if err := requireUTCEligibilityTime("valid_until", r.ValidUntil); err != nil {
		return SetMemoryValidityRequest{}, err
	}
	validity, err := (MemoryValidity{ValidFrom: r.ValidFrom, ValidUntil: r.ValidUntil}).Normalized()
	if err != nil {
		return SetMemoryValidityRequest{}, err
	}
	r.ValidFrom = validity.ValidFrom
	r.ValidUntil = validity.ValidUntil
	return r, nil
}

type ArchiveMemoryRequest struct {
	OperationID  string
	TenantID     string
	ContinuityID string
	MemoryID     string
}

func (r ArchiveMemoryRequest) Validate() error {
	_, err := r.normalized()
	return err
}

func (r ArchiveMemoryRequest) normalized() (ArchiveMemoryRequest, error) {
	r.OperationID = strings.TrimSpace(r.OperationID)
	r.TenantID = strings.TrimSpace(r.TenantID)
	r.ContinuityID = strings.TrimSpace(r.ContinuityID)
	r.MemoryID = strings.TrimSpace(r.MemoryID)
	if err := validateMemoryEligibilityIDs(r.OperationID, r.TenantID, r.ContinuityID, r.MemoryID); err != nil {
		return ArchiveMemoryRequest{}, err
	}
	return r, nil
}

type MemoryEligibilityReceipt struct {
	OperationID      string               `json:"operation_id"`
	ContinuityID     string               `json:"continuity_id"`
	MemoryID         string               `json:"memory_id"`
	Action           string               `json:"action"`
	PreviousState    MemoryEffectiveState `json:"previous_state"`
	ResultState      MemoryEffectiveState `json:"result_state"`
	PreviousValidity MemoryValidity       `json:"previous_validity"`
	ResultValidity   MemoryValidity       `json:"result_validity"`
	Replayed         bool                 `json:"replayed"`
}

type memoryEligibilityMutation struct {
	OperationID  string
	TenantID     string
	ContinuityID string
	MemoryID     string
	Action       string
	Validity     MemoryValidity
	Fingerprint  string
}

type memoryEligibilityRow interface {
	Scan(dest ...any) error
}

const memoryEligibilityOperationColumns = `
operation_id, continuity_id::text, memory_id::text, action,
previous_lifecycle_status, previous_valid_from, previous_valid_until,
result_lifecycle_status, result_valid_from, result_valid_until, created_at`

func (s *Store) SetMemoryValidity(ctx context.Context, request SetMemoryValidityRequest) (MemoryEligibilityReceipt, error) {
	request, err := request.normalized()
	if err != nil {
		return MemoryEligibilityReceipt{}, err
	}
	fingerprint, err := memoryEligibilityFingerprint(
		MemoryEligibilityActionSetValidity, request.TenantID, request.ContinuityID,
		request.MemoryID, MemoryValidity{ValidFrom: request.ValidFrom, ValidUntil: request.ValidUntil},
	)
	if err != nil {
		return MemoryEligibilityReceipt{}, err
	}
	return s.applyMemoryEligibilityMutation(ctx, memoryEligibilityMutation{
		OperationID: request.OperationID, TenantID: request.TenantID,
		ContinuityID: request.ContinuityID, MemoryID: request.MemoryID,
		Action:      MemoryEligibilityActionSetValidity,
		Validity:    MemoryValidity{ValidFrom: request.ValidFrom, ValidUntil: request.ValidUntil},
		Fingerprint: fingerprint,
	})
}

func (s *Store) ArchiveMemory(ctx context.Context, request ArchiveMemoryRequest) (MemoryEligibilityReceipt, error) {
	request, err := request.normalized()
	if err != nil {
		return MemoryEligibilityReceipt{}, err
	}
	fingerprint, err := memoryEligibilityFingerprint(
		MemoryEligibilityActionArchive, request.TenantID, request.ContinuityID,
		request.MemoryID, MemoryValidity{},
	)
	if err != nil {
		return MemoryEligibilityReceipt{}, err
	}
	return s.applyMemoryEligibilityMutation(ctx, memoryEligibilityMutation{
		OperationID: request.OperationID, TenantID: request.TenantID,
		ContinuityID: request.ContinuityID, MemoryID: request.MemoryID,
		Action: MemoryEligibilityActionArchive, Fingerprint: fingerprint,
	})
}

func (s *Store) applyMemoryEligibilityMutation(ctx context.Context, mutation memoryEligibilityMutation) (MemoryEligibilityReceipt, error) {
	ctx, err := withTenantContext(ctx, mutation.TenantID)
	if err != nil {
		return MemoryEligibilityReceipt{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return MemoryEligibilityReceipt{}, fmt.Errorf("begin memory eligibility operation: %w", err)
	}
	defer tx.Rollback(ctx)
	operationLockKey := fmt.Sprintf("%d:%s%d:%s", len(mutation.TenantID), mutation.TenantID, len(mutation.OperationID), mutation.OperationID)
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, operationLockKey); err != nil {
		return MemoryEligibilityReceipt{}, fmt.Errorf("lock memory eligibility operation: %w", err)
	}
	existing, found, err := lookupMemoryEligibilityOperationTx(ctx, tx, mutation.TenantID, mutation.OperationID)
	if err != nil {
		return MemoryEligibilityReceipt{}, err
	}
	if found {
		var action, fingerprint string
		if err := tx.QueryRow(ctx, `
SELECT action, request_fingerprint
FROM memory_eligibility_operations
WHERE tenant_id = $1 AND operation_id = $2`, mutation.TenantID, mutation.OperationID).Scan(&action, &fingerprint); err != nil {
			return MemoryEligibilityReceipt{}, fmt.Errorf("verify memory eligibility replay: %w", err)
		}
		if action != mutation.Action || fingerprint != mutation.Fingerprint {
			return MemoryEligibilityReceipt{}, fmt.Errorf("operation_id is already bound to another memory eligibility request")
		}
		existing.Replayed = true
		if err := tx.Commit(ctx); err != nil {
			return MemoryEligibilityReceipt{}, fmt.Errorf("commit memory eligibility replay: %w", err)
		}
		return existing, nil
	}

	var lifecycle, content string
	var validFrom, validUntil *time.Time
	err = tx.QueryRow(ctx, `
SELECT lifecycle_status, content, valid_from, valid_until
FROM governed_memories
WHERE tenant_id = $1 AND continuity_id = $2::uuid AND id = $3::uuid
FOR UPDATE`, mutation.TenantID, mutation.ContinuityID, mutation.MemoryID).Scan(
		&lifecycle, &content, &validFrom, &validUntil,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return MemoryEligibilityReceipt{}, fmt.Errorf("memory does not belong to this tenant and continuity")
	}
	if err != nil {
		return MemoryEligibilityReceipt{}, fmt.Errorf("lock memory eligibility target: %w", err)
	}
	if s.memoryEligibilityAfterTargetLock != nil {
		s.memoryEligibilityAfterTargetLock()
	}
	if lifecycle != "active" || content == "[redacted]" {
		return MemoryEligibilityReceipt{}, fmt.Errorf("memory must be an active non-redacted fact for %s", mutation.Action)
	}

	previousValidity := MemoryValidity{ValidFrom: normalizedEligibilityTime(validFrom), ValidUntil: normalizedEligibilityTime(validUntil)}
	resultLifecycle := lifecycle
	resultValidity := previousValidity
	switch mutation.Action {
	case MemoryEligibilityActionSetValidity:
		resultValidity = mutation.Validity
		command, err := tx.Exec(ctx, `
UPDATE governed_memories
SET valid_from = $1, valid_until = $2, updated_at = now()
WHERE tenant_id = $3 AND continuity_id = $4::uuid AND id = $5::uuid
  AND lifecycle_status = 'active' AND content <> '[redacted]'`,
			resultValidity.ValidFrom, resultValidity.ValidUntil,
			mutation.TenantID, mutation.ContinuityID, mutation.MemoryID,
		)
		if err != nil {
			return MemoryEligibilityReceipt{}, fmt.Errorf("set memory validity: %w", err)
		}
		if command.RowsAffected() != 1 {
			return MemoryEligibilityReceipt{}, fmt.Errorf("memory validity target changed concurrently")
		}
	case MemoryEligibilityActionArchive:
		resultLifecycle = "archived"
		command, err := tx.Exec(ctx, `
UPDATE governed_memories
SET lifecycle_status = 'archived', updated_at = now()
WHERE tenant_id = $1 AND continuity_id = $2::uuid AND id = $3::uuid
  AND lifecycle_status = 'active' AND content <> '[redacted]'`,
			mutation.TenantID, mutation.ContinuityID, mutation.MemoryID,
		)
		if err != nil {
			return MemoryEligibilityReceipt{}, fmt.Errorf("archive memory: %w", err)
		}
		if command.RowsAffected() != 1 {
			return MemoryEligibilityReceipt{}, fmt.Errorf("archive target changed concurrently")
		}
		if _, err := tx.Exec(ctx, `DELETE FROM memory_search_documents WHERE memory_id = $1::uuid`, mutation.MemoryID); err != nil {
			return MemoryEligibilityReceipt{}, fmt.Errorf("remove archived search document: %w", err)
		}
	default:
		return MemoryEligibilityReceipt{}, fmt.Errorf("memory eligibility action %q is unsupported", mutation.Action)
	}

	receipt, err := scanMemoryEligibilityOperation(tx.QueryRow(ctx, `
INSERT INTO memory_eligibility_operations (
  tenant_id, continuity_id, memory_id, operation_id, action, request_fingerprint,
  previous_lifecycle_status, previous_valid_from, previous_valid_until,
  result_lifecycle_status, result_valid_from, result_valid_until
) VALUES (
  $1, $2::uuid, $3::uuid, $4, $5, $6,
  $7, $8, $9,
  $10, $11, $12
)
RETURNING `+memoryEligibilityOperationColumns,
		mutation.TenantID, mutation.ContinuityID, mutation.MemoryID,
		mutation.OperationID, mutation.Action, mutation.Fingerprint,
		lifecycle, previousValidity.ValidFrom, previousValidity.ValidUntil,
		resultLifecycle, resultValidity.ValidFrom, resultValidity.ValidUntil,
	))
	if err != nil {
		return MemoryEligibilityReceipt{}, fmt.Errorf("record memory eligibility operation: %w", err)
	}
	if s.memoryEligibilityBeforeCommit != nil {
		s.memoryEligibilityBeforeCommit()
	}
	if err := tx.Commit(ctx); err != nil {
		return MemoryEligibilityReceipt{}, fmt.Errorf("commit memory eligibility operation: %w", err)
	}
	return receipt, nil
}

func lookupMemoryEligibilityOperationTx(ctx context.Context, tx pgx.Tx, tenantID, operationID string) (MemoryEligibilityReceipt, bool, error) {
	receipt, err := scanMemoryEligibilityOperation(tx.QueryRow(ctx, `
SELECT `+memoryEligibilityOperationColumns+`
FROM memory_eligibility_operations
WHERE tenant_id = $1 AND operation_id = $2`, tenantID, operationID))
	if errors.Is(err, pgx.ErrNoRows) {
		return MemoryEligibilityReceipt{}, false, nil
	}
	if err != nil {
		return MemoryEligibilityReceipt{}, false, fmt.Errorf("lookup memory eligibility operation: %w", err)
	}
	return receipt, true, nil
}

func scanMemoryEligibilityOperation(row memoryEligibilityRow) (MemoryEligibilityReceipt, error) {
	var receipt MemoryEligibilityReceipt
	var previousLifecycle, resultLifecycle string
	var createdAt time.Time
	if err := row.Scan(
		&receipt.OperationID, &receipt.ContinuityID, &receipt.MemoryID, &receipt.Action,
		&previousLifecycle, &receipt.PreviousValidity.ValidFrom, &receipt.PreviousValidity.ValidUntil,
		&resultLifecycle, &receipt.ResultValidity.ValidFrom, &receipt.ResultValidity.ValidUntil,
		&createdAt,
	); err != nil {
		return MemoryEligibilityReceipt{}, err
	}
	receipt.PreviousValidity.ValidFrom = normalizedEligibilityTime(receipt.PreviousValidity.ValidFrom)
	receipt.PreviousValidity.ValidUntil = normalizedEligibilityTime(receipt.PreviousValidity.ValidUntil)
	receipt.ResultValidity.ValidFrom = normalizedEligibilityTime(receipt.ResultValidity.ValidFrom)
	receipt.ResultValidity.ValidUntil = normalizedEligibilityTime(receipt.ResultValidity.ValidUntil)
	receipt.PreviousState = EffectiveMemoryState(previousLifecycle, "[authorized]", receipt.PreviousValidity, createdAt)
	receipt.ResultState = EffectiveMemoryState(resultLifecycle, "[authorized]", receipt.ResultValidity, createdAt)
	return receipt, nil
}

func validateMemoryEligibilityIDs(operationID, tenantID, continuityID, memoryID string) error {
	if operationID == "" {
		return fmt.Errorf("operation_id is required")
	}
	if len(operationID) > 128 {
		return fmt.Errorf("operation_id is too long")
	}
	if tenantID == "" {
		return fmt.Errorf("tenant_id is required")
	}
	if continuityID == "" {
		return fmt.Errorf("continuity_id is required")
	}
	if memoryID == "" {
		return fmt.Errorf("memory_id is required")
	}
	return nil
}

func requireUTCEligibilityTime(name string, value *time.Time) error {
	if value == nil {
		return nil
	}
	if value.IsZero() {
		return fmt.Errorf("%s must not be zero", name)
	}
	_, offset := value.Zone()
	if offset != 0 {
		return fmt.Errorf("%s must use UTC", name)
	}
	return nil
}

func memoryEligibilityFingerprint(action, tenantID, continuityID, memoryID string, validity MemoryValidity) (string, error) {
	type fingerprintInput struct {
		Action       string  `json:"action"`
		TenantID     string  `json:"tenant_id"`
		ContinuityID string  `json:"continuity_id"`
		MemoryID     string  `json:"memory_id"`
		ValidFrom    *string `json:"valid_from"`
		ValidUntil   *string `json:"valid_until"`
	}
	format := func(value *time.Time) *string {
		if value == nil {
			return nil
		}
		formatted := value.UTC().Format(time.RFC3339Nano)
		return &formatted
	}
	encoded, err := json.Marshal(fingerprintInput{
		Action: action, TenantID: tenantID, ContinuityID: continuityID, MemoryID: memoryID,
		ValidFrom: format(validity.ValidFrom), ValidUntil: format(validity.ValidUntil),
	})
	if err != nil {
		return "", fmt.Errorf("encode memory eligibility fingerprint: %w", err)
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}
