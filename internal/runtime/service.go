package runtime

import (
	"context"
	"fmt"
	"strings"
)

type PrepareContextResponse struct {
	Status     ResolutionStatus `json:"status"`
	DeliveryID string           `json:"delivery_id,omitempty"`
	Context    string           `json:"context"`
}

type CommitObservationResponse struct {
	ObservationID string `json:"observation_id"`
	MemoryID      string `json:"memory_id,omitempty"`
	MemoryStatus  string `json:"memory_status,omitempty"`
	Replayed      bool   `json:"replayed"`
}

type Service struct {
	store     *Store
	tenantID  string
	retriever MemoryRetriever
}

func NewService(store *Store, tenantID string) *Service {
	return &Service{store: store, tenantID: strings.TrimSpace(tenantID)}
}

func NewServiceWithRetriever(store *Store, tenantID string, retriever MemoryRetriever) *Service {
	return &Service{store: store, tenantID: strings.TrimSpace(tenantID), retriever: retriever}
}

func (s *Service) PrepareContext(ctx context.Context, request PrepareContextRequest) (PrepareContextResponse, error) {
	if s.store == nil || s.tenantID == "" {
		return PrepareContextResponse{}, fmt.Errorf("runtime service is not configured")
	}
	if err := request.Validate(); err != nil {
		return PrepareContextResponse{}, err
	}
	resolution, err := s.store.ResolveWorkspace(ctx, s.tenantID, request.Workspace)
	if err != nil {
		return PrepareContextResponse{}, err
	}
	if resolution.Status == ResolutionNeedsConfirmation {
		return PrepareContextResponse{Status: resolution.Status}, nil
	}
	snapshot, err := s.store.CurrentEligibilitySnapshot(ctx, s.tenantID)
	if err != nil {
		return PrepareContextResponse{}, err
	}
	defaultsContinuityID, err := s.store.EnsureGlobalDefaultsContinuity(ctx, s.tenantID)
	if err != nil {
		return PrepareContextResponse{}, err
	}
	defaults, err := s.store.ListEligibleGlobalDefaultsAt(ctx, s.tenantID, defaultsContinuityID, snapshot.AsOf)
	if err != nil {
		return PrepareContextResponse{}, err
	}
	var memories []Memory
	if s.retriever == nil {
		memories, err = s.store.SearchEligibleMemoryAt(
			ctx, s.tenantID, resolution.ContinuityID, request.Task, request.MaxItems, snapshot.AsOf,
		)
	} else {
		var result RetrievalResult
		result, err = s.retriever.Retrieve(ctx, RetrievalRequest{
			OperationID:     "workspace-retrieval:" + request.OperationID,
			TenantID:        s.tenantID,
			ContinuityIDs:   []string{resolution.ContinuityID},
			Query:           request.Task,
			Limit:           request.MaxItems,
			EligibilityAsOf: snapshot.AsOf,
		})
		memories = result.Memories
	}
	if err != nil {
		return PrepareContextResponse{}, err
	}
	delivery, err := s.store.RecordDeliveryAt(
		ctx, s.tenantID, resolution.ContinuityID, request.OperationID,
		request.Task, BuildWorkspaceContext(defaults, memories), snapshot.AsOf,
	)
	if err != nil {
		return PrepareContextResponse{}, err
	}
	return PrepareContextResponse{
		Status:     ResolutionResolved,
		DeliveryID: delivery.DeliveryID,
		Context:    delivery.Context,
	}, nil
}

func BuildWorkspaceContext(defaults, memories []Memory) string {
	sections := make([]string, 0, 2)
	if lines := semanticMemoryLines(defaults); len(lines) > 0 {
		sections = append(sections, "Global defaults:\n"+strings.Join(lines, "\n"))
	}
	if lines := semanticMemoryLines(memories); len(lines) > 0 {
		sections = append(sections, "Governed memory:\n"+strings.Join(lines, "\n"))
	}
	return strings.Join(sections, "\n\n")
}

func semanticMemoryLines(memories []Memory) []string {
	lines := make([]string, 0, len(memories))
	for _, memory := range memories {
		if content := strings.TrimSpace(memory.Content); content != "" && content != "[redacted]" {
			lines = append(lines, content)
		}
	}
	return lines
}

func (s *Service) CommitObservation(ctx context.Context, request CommitObservationRequest) (CommitObservationResponse, error) {
	if s.store == nil || s.tenantID == "" {
		return CommitObservationResponse{}, fmt.Errorf("runtime service is not configured")
	}
	if err := request.Validate(); err != nil {
		return CommitObservationResponse{}, err
	}
	if request.DeliveryID == "" {
		return CommitObservationResponse{}, fmt.Errorf("delivery_id is required")
	}
	continuityID, err := s.store.DeliveryContinuity(ctx, s.tenantID, request.DeliveryID)
	if err != nil {
		return CommitObservationResponse{}, err
	}
	result, err := s.store.CommitGovernedObservation(ctx, s.tenantID, continuityID, request)
	if err != nil {
		return CommitObservationResponse{}, err
	}
	return CommitObservationResponse{
		ObservationID: result.Observation.ObservationID,
		MemoryID:      result.Memory.MemoryID,
		MemoryStatus:  result.Memory.Status,
		Replayed:      result.Observation.Replayed || result.Memory.Replayed,
	}, nil
}
