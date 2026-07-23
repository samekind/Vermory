package runtime

import (
	"context"
	"fmt"
	"strings"
)

type GlobalDefaultsService struct {
	store    *Store
	tenantID string
}

func NewGlobalDefaultsService(store *Store, tenantID string) *GlobalDefaultsService {
	return &GlobalDefaultsService{store: store, tenantID: strings.TrimSpace(tenantID)}
}

func (s *GlobalDefaultsService) Inspect(ctx context.Context) (GlobalDefaultsInspection, error) {
	if err := s.configured(); err != nil {
		return GlobalDefaultsInspection{}, err
	}
	continuityID, defaults, err := s.store.ListGlobalDefaults(ctx, s.tenantID)
	if err != nil {
		return GlobalDefaultsInspection{}, err
	}
	return GlobalDefaultsInspection{ContinuityID: continuityID, Defaults: defaults}, nil
}

func (s *GlobalDefaultsService) Set(ctx context.Context, request SetGlobalDefaultRequest) (GlobalDefaultMutationReceipt, error) {
	if err := s.configured(); err != nil {
		return GlobalDefaultMutationReceipt{}, err
	}
	if err := request.Validate(); err != nil {
		return GlobalDefaultMutationReceipt{}, err
	}
	continuityID, err := s.store.EnsureGlobalDefaultsContinuity(ctx, s.tenantID)
	if err != nil {
		return GlobalDefaultMutationReceipt{}, err
	}
	receipt, err := s.store.SetGlobalDefault(ctx, s.tenantID, request.OperationID, request.Key, request.Content)
	if err != nil {
		return GlobalDefaultMutationReceipt{}, err
	}
	return globalDefaultReceipt(continuityID, receipt), nil
}

func (s *GlobalDefaultsService) Correct(ctx context.Context, request CorrectGlobalDefaultRequest) (GlobalDefaultMutationReceipt, error) {
	if err := s.configured(); err != nil {
		return GlobalDefaultMutationReceipt{}, err
	}
	if err := request.Validate(); err != nil {
		return GlobalDefaultMutationReceipt{}, err
	}
	continuityID, err := s.store.EnsureGlobalDefaultsContinuity(ctx, s.tenantID)
	if err != nil {
		return GlobalDefaultMutationReceipt{}, err
	}
	receipt, err := s.store.CorrectGlobalDefault(ctx, s.tenantID, request.OperationID, request.MemoryID, request.Content)
	if err != nil {
		return GlobalDefaultMutationReceipt{}, err
	}
	return globalDefaultReceipt(continuityID, receipt), nil
}

func (s *GlobalDefaultsService) Forget(ctx context.Context, request ForgetGlobalDefaultRequest) (GlobalDefaultMutationReceipt, error) {
	if err := s.configured(); err != nil {
		return GlobalDefaultMutationReceipt{}, err
	}
	if err := request.Validate(); err != nil {
		return GlobalDefaultMutationReceipt{}, err
	}
	continuityID, err := s.store.EnsureGlobalDefaultsContinuity(ctx, s.tenantID)
	if err != nil {
		return GlobalDefaultMutationReceipt{}, err
	}
	receipt, err := s.store.ForgetGlobalDefault(ctx, s.tenantID, request.OperationID, request.MemoryID)
	if err != nil {
		return GlobalDefaultMutationReceipt{}, err
	}
	return globalDefaultReceipt(continuityID, receipt), nil
}

func (s *GlobalDefaultsService) configured() error {
	if s.store == nil || s.tenantID == "" {
		return fmt.Errorf("global defaults service is not configured")
	}
	return nil
}

func globalDefaultReceipt(continuityID string, receipt GovernedObservationReceipt) GlobalDefaultMutationReceipt {
	return GlobalDefaultMutationReceipt{
		ContinuityID:  continuityID,
		ObservationID: receipt.Observation.ObservationID,
		MemoryID:      receipt.Memory.MemoryID,
		MemoryStatus:  receipt.Memory.Status,
		Replayed:      receipt.Observation.Replayed || receipt.Memory.Replayed,
	}
}
