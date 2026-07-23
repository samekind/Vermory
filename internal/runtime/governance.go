package runtime

import (
	"context"
	"fmt"
	"strings"
)

const localOperatorSourceRef = "operator:local"

type GovernanceWriteRequest struct {
	OperationID string
	MemoryKey   string
	Content     string
	SourceRef   string
}

type GovernanceService struct {
	store               *Store
	tenantID            string
	filesystemNamespace string
}

func NewGovernanceService(store *Store, tenantID string) *GovernanceService {
	return NewGovernanceServiceWithNamespace(store, tenantID, "")
}

func NewGovernanceServiceWithNamespace(store *Store, tenantID, filesystemNamespace string) *GovernanceService {
	return &GovernanceService{
		store: store, tenantID: strings.TrimSpace(tenantID), filesystemNamespace: strings.TrimSpace(filesystemNamespace),
	}
}

func (s *GovernanceService) ConfirmWorkspace(ctx context.Context, repoRoot string) (WorkspaceResolution, error) {
	return s.ConfirmWorkspaceAnchor(ctx, s.workspaceAnchor(repoRoot))
}

func (s *GovernanceService) ConfirmWorkspaceAnchor(ctx context.Context, anchor WorkspaceAnchor) (WorkspaceResolution, error) {
	if err := s.configured(); err != nil {
		return WorkspaceResolution{}, err
	}
	continuityID, err := s.store.ConfirmWorkspaceAnchorBinding(ctx, s.tenantID, anchor)
	if err != nil {
		return WorkspaceResolution{}, err
	}
	anchor, err = anchor.Normalized()
	if err != nil {
		return WorkspaceResolution{}, err
	}
	return WorkspaceResolution{
		Status:              ResolutionResolved,
		ContinuityID:        continuityID,
		RepoRoot:            anchor.RepoRoot,
		FilesystemNamespace: anchor.FilesystemNamespace,
	}, nil
}

func (s *GovernanceService) InspectWorkspace(ctx context.Context, repoRoot string) (WorkspaceResolution, error) {
	return s.InspectWorkspaceAnchor(ctx, s.workspaceAnchor(repoRoot))
}

func (s *GovernanceService) InspectWorkspaceAnchor(ctx context.Context, anchor WorkspaceAnchor) (WorkspaceResolution, error) {
	if err := s.configured(); err != nil {
		return WorkspaceResolution{}, err
	}
	return s.store.ResolveWorkspace(ctx, s.tenantID, anchor)
}

func (s *GovernanceService) ListWorkspaceMemories(ctx context.Context, repoRoot string) (WorkspaceResolution, []GovernedMemory, error) {
	resolution, err := s.confirmedWorkspace(ctx, repoRoot)
	if err != nil {
		return WorkspaceResolution{}, nil, err
	}
	memories, err := s.store.ListGovernedMemories(ctx, s.tenantID, resolution.ContinuityID)
	if err != nil {
		return WorkspaceResolution{}, nil, err
	}
	return resolution, memories, nil
}

func (s *GovernanceService) AddSource(ctx context.Context, repoRoot string, write GovernanceWriteRequest) (GovernedObservationReceipt, error) {
	if strings.TrimSpace(write.SourceRef) == "" {
		return GovernedObservationReceipt{}, fmt.Errorf("source_ref is required for source facts")
	}
	return s.commit(ctx, repoRoot, CommitObservationRequest{
		OperationID: write.OperationID,
		Kind:        ObservationKindSourceUpdate,
		MemoryKey:   write.MemoryKey,
		Content:     write.Content,
		SourceRef:   write.SourceRef,
	})
}

func (s *GovernanceService) ReviseSource(ctx context.Context, repoRoot, memoryID string, write GovernanceWriteRequest) (GovernedObservationReceipt, error) {
	if strings.TrimSpace(memoryID) == "" {
		return GovernedObservationReceipt{}, fmt.Errorf("memory_id is required for source revision")
	}
	if strings.TrimSpace(write.SourceRef) == "" {
		return GovernedObservationReceipt{}, fmt.Errorf("source_ref is required for source revision")
	}
	return s.commit(ctx, repoRoot, CommitObservationRequest{
		OperationID:        write.OperationID,
		Kind:               ObservationKindSourceUpdate,
		Content:            write.Content,
		SourceRef:          write.SourceRef,
		SupersedesMemoryID: memoryID,
	})
}

func (s *GovernanceService) Correct(ctx context.Context, repoRoot, memoryID string, write GovernanceWriteRequest) (GovernedObservationReceipt, error) {
	if strings.TrimSpace(memoryID) == "" {
		return GovernedObservationReceipt{}, fmt.Errorf("memory_id is required for correction")
	}
	return s.commit(ctx, repoRoot, CommitObservationRequest{
		OperationID:        write.OperationID,
		Kind:               ObservationKindUserCorrection,
		Content:            write.Content,
		SourceRef:          localOperatorSourceRef,
		SupersedesMemoryID: memoryID,
	})
}

func (s *GovernanceService) Forget(ctx context.Context, repoRoot, memoryID, operationID string) (GovernedObservationReceipt, error) {
	if strings.TrimSpace(memoryID) == "" {
		return GovernedObservationReceipt{}, fmt.Errorf("memory_id is required for forget")
	}
	return s.commit(ctx, repoRoot, CommitObservationRequest{
		OperationID:    operationID,
		Kind:           ObservationKindForgetRequest,
		Content:        "Operator requested deletion.",
		SourceRef:      localOperatorSourceRef,
		TargetMemoryID: memoryID,
	})
}

func (s *GovernanceService) commit(ctx context.Context, repoRoot string, request CommitObservationRequest) (GovernedObservationReceipt, error) {
	resolution, err := s.confirmedWorkspace(ctx, repoRoot)
	if err != nil {
		return GovernedObservationReceipt{}, err
	}
	return s.store.CommitGovernedObservation(ctx, s.tenantID, resolution.ContinuityID, request)
}

func (s *GovernanceService) confirmedWorkspace(ctx context.Context, repoRoot string) (WorkspaceResolution, error) {
	resolution, err := s.InspectWorkspace(ctx, repoRoot)
	if err != nil {
		return WorkspaceResolution{}, err
	}
	if resolution.Status != ResolutionResolved {
		return WorkspaceResolution{}, fmt.Errorf("workspace requires confirmation: %s", resolution.RepoRoot)
	}
	return resolution, nil
}

func (s *GovernanceService) configured() error {
	if s.store == nil || s.tenantID == "" {
		return fmt.Errorf("governance service is not configured")
	}
	return nil
}

func (s *GovernanceService) workspaceAnchor(repoRoot string) WorkspaceAnchor {
	return WorkspaceAnchor{RepoRoot: repoRoot, FilesystemNamespace: s.filesystemNamespace}
}
