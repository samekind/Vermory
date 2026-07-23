package runtime

import (
	"context"
	"fmt"
	"strings"
)

type BridgeService struct {
	store    *Store
	tenantID string
}

func NewBridgeService(store *Store, tenantID string) *BridgeService {
	return &BridgeService{store: store, tenantID: strings.TrimSpace(tenantID)}
}

func (s *BridgeService) PromoteConversationToWorkspace(ctx context.Context, request PromoteConversationToWorkspaceRequest) (BridgeReceipt, error) {
	if err := s.configured(); err != nil {
		return BridgeReceipt{}, err
	}
	if err := request.Validate(); err != nil {
		return BridgeReceipt{}, err
	}
	sourceAnchor := request.Source.Channel + "/" + request.Source.ThreadID
	fingerprint := bridgeRequestFingerprint(sourceAnchor, request.TargetRepoRoot, strings.Join(request.MemoryIDs, ","))
	if replay, found, err := s.store.ReplayBridgeOperation(ctx, s.tenantID, request.OperationID, BridgeActionPromote, fingerprint); err != nil || found {
		return replay, err
	}
	source, err := s.store.ResolveConversation(ctx, s.tenantID, request.Source)
	if err != nil {
		return BridgeReceipt{}, err
	}
	if source.Status != ResolutionResolved {
		return BridgeReceipt{}, fmt.Errorf("source conversation does not exist")
	}
	target, err := s.store.ResolveWorkspace(ctx, s.tenantID, WorkspaceAnchor{RepoRoot: request.TargetRepoRoot})
	if err != nil {
		return BridgeReceipt{}, err
	}
	if target.Status != ResolutionResolved {
		return BridgeReceipt{}, fmt.Errorf("target workspace requires confirmation")
	}
	return s.store.PromoteConversationMemory(
		ctx,
		s.tenantID,
		request.OperationID,
		source.ContinuityID,
		target.ContinuityID,
		sourceAnchor,
		request.TargetRepoRoot,
		request.MemoryIDs,
	)
}

func (s *BridgeService) ExportWorkspace(ctx context.Context, request ExportWorkspaceRequest) (BridgeReceipt, error) {
	if err := s.configured(); err != nil {
		return BridgeReceipt{}, err
	}
	if err := request.Validate(); err != nil {
		return BridgeReceipt{}, err
	}
	fingerprint := bridgeRequestFingerprint(request.RepoRoot, strings.Join(request.MemoryIDs, ","), request.Title, request.TargetProfile)
	if replay, found, err := s.store.ReplayBridgeOperation(ctx, s.tenantID, request.OperationID, BridgeActionExport, fingerprint); err != nil || found {
		return replay, err
	}
	resolution, err := s.store.ResolveWorkspace(ctx, s.tenantID, WorkspaceAnchor{RepoRoot: request.RepoRoot})
	if err != nil {
		return BridgeReceipt{}, err
	}
	if resolution.Status != ResolutionResolved {
		return BridgeReceipt{}, fmt.Errorf("workspace requires confirmation")
	}
	return s.store.ExportWorkspaceMemory(
		ctx,
		s.tenantID,
		request.OperationID,
		resolution.ContinuityID,
		request.RepoRoot,
		request.MemoryIDs,
		request.Title,
		request.TargetProfile,
	)
}

func (s *BridgeService) LinkConversations(ctx context.Context, request LinkConversationsRequest) (BridgeReceipt, error) {
	if err := s.configured(); err != nil {
		return BridgeReceipt{}, err
	}
	if err := request.Validate(); err != nil {
		return BridgeReceipt{}, err
	}
	primaryAnchor := request.Primary.Channel + "/" + request.Primary.ThreadID
	linkedAnchor := request.Linked.Channel + "/" + request.Linked.ThreadID
	fingerprint := bridgeRequestFingerprint(primaryAnchor, linkedAnchor)
	if replay, found, err := s.store.ReplayBridgeOperation(ctx, s.tenantID, request.OperationID, BridgeActionLink, fingerprint); err != nil || found {
		return replay, err
	}
	primary, err := s.store.ResolveConversation(ctx, s.tenantID, request.Primary)
	if err != nil {
		return BridgeReceipt{}, err
	}
	if primary.Status != ResolutionResolved {
		return BridgeReceipt{}, fmt.Errorf("primary conversation does not exist")
	}
	linked, err := s.store.ResolveConversation(ctx, s.tenantID, request.Linked)
	if err != nil {
		return BridgeReceipt{}, err
	}
	if linked.Status != ResolutionResolved {
		return BridgeReceipt{}, fmt.Errorf("linked conversation does not exist")
	}
	return s.store.LinkConversationContinuities(
		ctx,
		s.tenantID,
		request.OperationID,
		primary.ContinuityID,
		linked.ContinuityID,
		primaryAnchor,
		linkedAnchor,
	)
}

func (s *BridgeService) AdoptWorkspaceAnchor(ctx context.Context, request AdoptWorkspaceAnchorRequest) (BridgeReceipt, error) {
	if err := s.configured(); err != nil {
		return BridgeReceipt{}, err
	}
	if err := request.Validate(); err != nil {
		return BridgeReceipt{}, err
	}
	fingerprint := bridgeRequestFingerprint(request.ExistingNamespaceID, request.ExistingRepoRoot, request.NewNamespaceID, request.NewRepoRoot)
	if replay, found, err := s.store.ReplayBridgeOperation(ctx, s.tenantID, request.OperationID, BridgeActionAdopt, fingerprint); err != nil || found {
		return replay, err
	}
	existing, err := s.store.ResolveWorkspace(ctx, s.tenantID, WorkspaceAnchor{RepoRoot: request.ExistingRepoRoot, FilesystemNamespace: request.ExistingNamespaceID})
	if err != nil {
		return BridgeReceipt{}, err
	}
	if existing.Status != ResolutionResolved {
		return BridgeReceipt{}, fmt.Errorf("existing workspace requires confirmation")
	}
	return s.store.AdoptWorkspaceBinding(ctx, s.tenantID, request.OperationID, existing.ContinuityID,
		WorkspaceAnchor{RepoRoot: request.ExistingRepoRoot, FilesystemNamespace: request.ExistingNamespaceID},
		WorkspaceAnchor{RepoRoot: request.NewRepoRoot, FilesystemNamespace: request.NewNamespaceID})
}

func (s *BridgeService) RebindWorkspace(ctx context.Context, request RebindWorkspaceRequest) (BridgeReceipt, error) {
	if err := s.configured(); err != nil {
		return BridgeReceipt{}, err
	}
	if err := request.Validate(); err != nil {
		return BridgeReceipt{}, err
	}
	fingerprint := bridgeRequestFingerprint(request.OldNamespaceID, request.OldRepoRoot, request.NewNamespaceID, request.NewRepoRoot)
	if replay, found, err := s.store.ReplayBridgeOperation(ctx, s.tenantID, request.OperationID, BridgeActionRebind, fingerprint); err != nil || found {
		return replay, err
	}
	existing, err := s.store.ResolveWorkspace(ctx, s.tenantID, WorkspaceAnchor{RepoRoot: request.OldRepoRoot, FilesystemNamespace: request.OldNamespaceID})
	if err != nil {
		return BridgeReceipt{}, err
	}
	if existing.Status != ResolutionResolved {
		return BridgeReceipt{}, fmt.Errorf("old workspace requires confirmation")
	}
	return s.store.RebindWorkspaceBinding(ctx, s.tenantID, request.OperationID, existing.ContinuityID,
		WorkspaceAnchor{RepoRoot: request.OldRepoRoot, FilesystemNamespace: request.OldNamespaceID},
		WorkspaceAnchor{RepoRoot: request.NewRepoRoot, FilesystemNamespace: request.NewNamespaceID})
}

func (s *BridgeService) Reverse(ctx context.Context, request ReverseBridgeRequest) (BridgeReceipt, error) {
	if err := s.configured(); err != nil {
		return BridgeReceipt{}, err
	}
	if err := request.Validate(); err != nil {
		return BridgeReceipt{}, err
	}
	return s.store.ReverseBridge(ctx, s.tenantID, request.OperationID, request.BridgeID)
}

func (s *BridgeService) Inspect(ctx context.Context, bridgeID string) (BridgeReceipt, error) {
	if err := s.configured(); err != nil {
		return BridgeReceipt{}, err
	}
	bridgeID = strings.TrimSpace(bridgeID)
	if bridgeID == "" {
		return BridgeReceipt{}, fmt.Errorf("bridge_id is required")
	}
	return s.store.InspectBridge(ctx, s.tenantID, bridgeID)
}

func (s *BridgeService) configured() error {
	if s.store == nil || s.tenantID == "" {
		return fmt.Errorf("bridge service is not configured")
	}
	return nil
}
