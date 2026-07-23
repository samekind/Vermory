package runtime

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"vermory/internal/resolver"
)

type BridgeAction string

const (
	BridgeActionPromote BridgeAction = "promote"
	BridgeActionLink    BridgeAction = "link"
	BridgeActionExport  BridgeAction = "export"
	BridgeActionAdopt   BridgeAction = "adopt"
	BridgeActionRebind  BridgeAction = "rebind"
)

func (a BridgeAction) Valid() bool {
	switch a {
	case BridgeActionPromote, BridgeActionLink, BridgeActionExport, BridgeActionAdopt, BridgeActionRebind:
		return true
	default:
		return false
	}
}

type BridgeStatus string

const (
	BridgeStatusActive   BridgeStatus = "active"
	BridgeStatusReversed BridgeStatus = "reversed"
	BridgeStatusRevoked  BridgeStatus = "revoked"
)

type BridgeEventType string

const (
	BridgeEventCreated  BridgeEventType = "created"
	BridgeEventReversed BridgeEventType = "reversed"
	BridgeEventRevoked  BridgeEventType = "revoked"
)

type BridgeEvent struct {
	ID          string          `json:"id"`
	EventType   BridgeEventType `json:"event_type"`
	OperationID string          `json:"operation_id"`
	CreatedAt   string          `json:"created_at"`
}

type BridgeMemoryEffect struct {
	EffectKind     string `json:"effect_kind"`
	SourceMemoryID string `json:"source_memory_id"`
	TargetMemoryID string `json:"target_memory_id,omitempty"`
	OrderIndex     int    `json:"order_index"`
}

type BridgeReceipt struct {
	ID                 string               `json:"bridge_id"`
	OperationID        string               `json:"operation_id"`
	Action             BridgeAction         `json:"action"`
	Status             BridgeStatus         `json:"status"`
	SourceContinuityID string               `json:"source_continuity_id,omitempty"`
	TargetContinuityID string               `json:"target_continuity_id,omitempty"`
	SourceAnchor       string               `json:"source_anchor,omitempty"`
	TargetAnchor       string               `json:"target_anchor,omitempty"`
	SourceNamespaceID  string               `json:"source_filesystem_namespace,omitempty"`
	TargetNamespaceID  string               `json:"target_filesystem_namespace,omitempty"`
	TargetProfile      string               `json:"target_profile,omitempty"`
	Title              string               `json:"title,omitempty"`
	ExportBody         string               `json:"export_body,omitempty"`
	ReverseOperationID string               `json:"reverse_operation_id,omitempty"`
	Replayed           bool                 `json:"replayed"`
	Events             []BridgeEvent        `json:"events"`
	MemoryEffects      []BridgeMemoryEffect `json:"memory_effects"`
}

type PromoteConversationToWorkspaceRequest struct {
	OperationID    string             `json:"operation_id"`
	Source         ConversationAnchor `json:"source"`
	TargetRepoRoot string             `json:"target_repo_root"`
	MemoryIDs      []string           `json:"memory_ids"`
}

func (r *PromoteConversationToWorkspaceRequest) Validate() error {
	if err := normalizeBridgeOperationID(&r.OperationID); err != nil {
		return err
	}
	anchor, err := r.Source.Normalized()
	if err != nil {
		return err
	}
	r.Source = anchor
	target, err := normalizeAbsolutePath(r.TargetRepoRoot)
	if err != nil {
		return fmt.Errorf("target_repo_root: %w", err)
	}
	r.TargetRepoRoot = target
	r.MemoryIDs, err = normalizeBridgeMemoryIDs(r.MemoryIDs)
	return err
}

type ExportWorkspaceRequest struct {
	OperationID   string   `json:"operation_id"`
	RepoRoot      string   `json:"repo_root"`
	MemoryIDs     []string `json:"memory_ids"`
	Title         string   `json:"title"`
	TargetProfile string   `json:"target_profile"`
}

type LinkConversationsRequest struct {
	OperationID string             `json:"operation_id"`
	Primary     ConversationAnchor `json:"primary"`
	Linked      ConversationAnchor `json:"linked"`
}

type AdoptWorkspaceAnchorRequest struct {
	OperationID         string `json:"operation_id"`
	ExistingRepoRoot    string `json:"existing_repo_root"`
	NewRepoRoot         string `json:"new_repo_root"`
	ExistingNamespaceID string `json:"existing_filesystem_namespace,omitempty"`
	NewNamespaceID      string `json:"new_filesystem_namespace,omitempty"`
}

func (r *AdoptWorkspaceAnchorRequest) Validate() error {
	if err := normalizeBridgeOperationID(&r.OperationID); err != nil {
		return err
	}
	existing, err := normalizeAbsolutePath(r.ExistingRepoRoot)
	if err != nil {
		return fmt.Errorf("existing_repo_root: %w", err)
	}
	newRoot, err := normalizeAbsolutePath(r.NewRepoRoot)
	if err != nil {
		return fmt.Errorf("new_repo_root: %w", err)
	}
	if existing == newRoot {
		return fmt.Errorf("existing_repo_root and new_repo_root must be different")
	}
	r.ExistingRepoRoot = existing
	r.NewRepoRoot = newRoot
	if r.ExistingNamespaceID, err = normalizeWorkspaceNamespace(r.ExistingNamespaceID); err != nil {
		return fmt.Errorf("existing_filesystem_namespace: %w", err)
	}
	if r.NewNamespaceID, err = normalizeWorkspaceNamespace(r.NewNamespaceID); err != nil {
		return fmt.Errorf("new_filesystem_namespace: %w", err)
	}
	return nil
}

type RebindWorkspaceRequest struct {
	OperationID    string `json:"operation_id"`
	OldRepoRoot    string `json:"old_repo_root"`
	NewRepoRoot    string `json:"new_repo_root"`
	OldNamespaceID string `json:"old_filesystem_namespace,omitempty"`
	NewNamespaceID string `json:"new_filesystem_namespace,omitempty"`
}

func (r *RebindWorkspaceRequest) Validate() error {
	if err := normalizeBridgeOperationID(&r.OperationID); err != nil {
		return err
	}
	oldRoot, err := normalizeAbsolutePath(r.OldRepoRoot)
	if err != nil {
		return fmt.Errorf("old_repo_root: %w", err)
	}
	newRoot, err := normalizeAbsolutePath(r.NewRepoRoot)
	if err != nil {
		return fmt.Errorf("new_repo_root: %w", err)
	}
	if oldRoot == newRoot {
		return fmt.Errorf("old_repo_root and new_repo_root must be different")
	}
	r.OldRepoRoot = oldRoot
	r.NewRepoRoot = newRoot
	if r.OldNamespaceID, err = normalizeWorkspaceNamespace(r.OldNamespaceID); err != nil {
		return fmt.Errorf("old_filesystem_namespace: %w", err)
	}
	if r.NewNamespaceID, err = normalizeWorkspaceNamespace(r.NewNamespaceID); err != nil {
		return fmt.Errorf("new_filesystem_namespace: %w", err)
	}
	return nil
}

func (r *LinkConversationsRequest) Validate() error {
	if err := normalizeBridgeOperationID(&r.OperationID); err != nil {
		return err
	}
	primary, err := r.Primary.Normalized()
	if err != nil {
		return fmt.Errorf("primary: %w", err)
	}
	linked, err := r.Linked.Normalized()
	if err != nil {
		return fmt.Errorf("linked: %w", err)
	}
	r.Primary = primary
	r.Linked = linked
	if primary.Channel == linked.Channel && primary.ThreadID == linked.ThreadID {
		return fmt.Errorf("primary and linked conversations must be different")
	}
	return nil
}

func (r *ExportWorkspaceRequest) Validate() error {
	if err := normalizeBridgeOperationID(&r.OperationID); err != nil {
		return err
	}
	repoRoot, err := normalizeAbsolutePath(r.RepoRoot)
	if err != nil {
		return fmt.Errorf("repo_root: %w", err)
	}
	r.RepoRoot = repoRoot
	r.MemoryIDs, err = normalizeBridgeMemoryIDs(r.MemoryIDs)
	if err != nil {
		return err
	}
	r.Title = strings.TrimSpace(r.Title)
	r.TargetProfile = strings.TrimSpace(r.TargetProfile)
	if r.Title == "" {
		return fmt.Errorf("title is required")
	}
	if len(r.Title) > 256 {
		return fmt.Errorf("title is too long")
	}
	if r.TargetProfile == "" {
		return fmt.Errorf("target_profile is required")
	}
	if len(r.TargetProfile) > 128 {
		return fmt.Errorf("target_profile is too long")
	}
	return nil
}

type ReverseBridgeRequest struct {
	OperationID string `json:"operation_id"`
	BridgeID    string `json:"bridge_id"`
}

func (r *ReverseBridgeRequest) Validate() error {
	if err := normalizeBridgeOperationID(&r.OperationID); err != nil {
		return err
	}
	r.BridgeID = strings.TrimSpace(r.BridgeID)
	if r.BridgeID == "" {
		return fmt.Errorf("bridge_id is required")
	}
	return nil
}

type bridgeLedgerInput struct {
	TenantID           string
	OperationID        string
	Action             BridgeAction
	RequestFingerprint string
	SourceContinuityID string
	TargetContinuityID string
	SourceAnchor       string
	TargetAnchor       string
	SourceNamespaceID  string
	TargetNamespaceID  string
	TargetProfile      string
	Title              string
	ExportBody         string
}

func (i *bridgeLedgerInput) normalize() error {
	i.TenantID = strings.TrimSpace(i.TenantID)
	i.OperationID = strings.TrimSpace(i.OperationID)
	i.RequestFingerprint = strings.TrimSpace(i.RequestFingerprint)
	i.SourceContinuityID = strings.TrimSpace(i.SourceContinuityID)
	i.TargetContinuityID = strings.TrimSpace(i.TargetContinuityID)
	i.SourceAnchor = strings.TrimSpace(i.SourceAnchor)
	i.TargetAnchor = strings.TrimSpace(i.TargetAnchor)
	var err error
	if i.SourceNamespaceID, err = normalizeWorkspaceNamespace(i.SourceNamespaceID); err != nil {
		return fmt.Errorf("source_filesystem_namespace: %w", err)
	}
	if i.TargetNamespaceID, err = normalizeWorkspaceNamespace(i.TargetNamespaceID); err != nil {
		return fmt.Errorf("target_filesystem_namespace: %w", err)
	}
	i.TargetProfile = strings.TrimSpace(i.TargetProfile)
	i.Title = strings.TrimSpace(i.Title)
	i.ExportBody = strings.TrimSpace(i.ExportBody)
	if i.TenantID == "" {
		return fmt.Errorf("tenant_id is required")
	}
	if i.OperationID == "" {
		return fmt.Errorf("operation_id is required")
	}
	if len(i.OperationID) > 512 {
		return fmt.Errorf("operation_id is too long")
	}
	if !i.Action.Valid() {
		return fmt.Errorf("bridge action %q is unsupported", i.Action)
	}
	if i.RequestFingerprint == "" {
		return fmt.Errorf("request fingerprint is required")
	}
	return nil
}

func normalizeWorkspaceNamespace(value string) (string, error) {
	return resolver.NormalizeFilesystemNamespace(value, true)
}

func bridgeRequestFingerprint(parts ...string) string {
	hash := sha256.New()
	for _, part := range parts {
		value := strings.TrimSpace(part)
		_, _ = fmt.Fprintf(hash, "%d:%s|", len(value), value)
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func normalizeBridgeOperationID(operationID *string) error {
	*operationID = strings.TrimSpace(*operationID)
	if *operationID == "" {
		return fmt.Errorf("operation_id is required")
	}
	if len(*operationID) > 384 {
		return fmt.Errorf("operation_id is too long")
	}
	return nil
}

func normalizeBridgeMemoryIDs(memoryIDs []string) ([]string, error) {
	if len(memoryIDs) == 0 {
		return nil, fmt.Errorf("memory_ids is required")
	}
	if len(memoryIDs) > 50 {
		return nil, fmt.Errorf("memory_ids has too many items")
	}
	seen := make(map[string]struct{}, len(memoryIDs))
	normalized := make([]string, 0, len(memoryIDs))
	for _, memoryID := range memoryIDs {
		memoryID = strings.TrimSpace(memoryID)
		if memoryID == "" {
			return nil, fmt.Errorf("memory_ids contains an empty item")
		}
		if _, exists := seen[memoryID]; exists {
			return nil, fmt.Errorf("memory_ids contains a duplicate item")
		}
		seen[memoryID] = struct{}{}
		normalized = append(normalized, memoryID)
	}
	return normalized, nil
}
