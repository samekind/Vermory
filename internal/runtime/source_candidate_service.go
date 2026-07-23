package runtime

import (
	"context"
	"fmt"
	"strings"
)

func (s *GovernanceService) ProposeSourceCandidate(ctx context.Context, repoRoot string, write GovernanceWriteRequest) (SourceCandidateReceipt, error) {
	if err := s.configured(); err != nil {
		return SourceCandidateReceipt{}, err
	}
	resolution, err := s.confirmedWorkspace(ctx, repoRoot)
	if err != nil {
		return SourceCandidateReceipt{}, err
	}
	request := CommitObservationRequest{
		OperationID: write.OperationID,
		Kind:        ObservationKindSourceCandidate,
		MemoryKey:   write.MemoryKey,
		Content:     write.Content,
		SourceRef:   write.SourceRef,
	}
	if err := request.Validate(); err != nil {
		return SourceCandidateReceipt{}, err
	}
	if replay, found, err := s.store.LookupSourceCandidateOperation(ctx, s.tenantID, resolution.ContinuityID, request); err != nil {
		return SourceCandidateReceipt{}, err
	} else if found {
		return replay, nil
	}

	matches, err := s.store.ListActiveMemoriesByKey(ctx, s.tenantID, resolution.ContinuityID, request.MemoryKey, 2)
	if err != nil {
		return SourceCandidateReceipt{}, err
	}
	if len(matches) > 1 {
		return SourceCandidateReceipt{}, fmt.Errorf("source candidate memory_key %q is ambiguous", request.MemoryKey)
	}
	if len(matches) == 1 {
		request.SupersedesMemoryID = matches[0].ID
		if strings.TrimSpace(matches[0].Content) == request.Content {
			observation, err := s.store.CommitObservation(ctx, s.tenantID, resolution.ContinuityID, request)
			if err != nil {
				return SourceCandidateReceipt{}, err
			}
			return SourceCandidateReceipt{
				Disposition:    SourceCandidateUnchanged,
				MemoryKey:      request.MemoryKey,
				TargetMemoryID: matches[0].ID,
				Observation:    observation,
				Replayed:       observation.Replayed,
			}, nil
		}
	}

	receipt, err := s.store.CommitGovernedObservation(ctx, s.tenantID, resolution.ContinuityID, request)
	if err != nil {
		return SourceCandidateReceipt{}, err
	}
	disposition := SourceCandidateNew
	if request.SupersedesMemoryID != "" {
		disposition = SourceCandidateReplacement
	}
	return SourceCandidateReceipt{
		Disposition:    disposition,
		MemoryKey:      request.MemoryKey,
		TargetMemoryID: request.SupersedesMemoryID,
		Observation:    receipt.Observation,
		Candidate:      receipt.Memory,
		Replayed:       receipt.Observation.Replayed || receipt.Memory.Replayed,
	}, nil
}

func (s *GovernanceService) AcceptCandidate(ctx context.Context, repoRoot, candidateMemoryID, operationID string) (GovernedObservationReceipt, error) {
	resolution, err := s.confirmedWorkspace(ctx, repoRoot)
	if err != nil {
		return GovernedObservationReceipt{}, err
	}
	return s.store.AcceptSourceCandidate(ctx, s.tenantID, resolution.ContinuityID, strings.TrimSpace(candidateMemoryID), strings.TrimSpace(operationID))
}

func (s *GovernanceService) RejectCandidate(ctx context.Context, repoRoot, candidateMemoryID, operationID string) (GovernedObservationReceipt, error) {
	resolution, err := s.confirmedWorkspace(ctx, repoRoot)
	if err != nil {
		return GovernedObservationReceipt{}, err
	}
	return s.store.RejectSourceCandidate(ctx, s.tenantID, resolution.ContinuityID, strings.TrimSpace(candidateMemoryID), strings.TrimSpace(operationID))
}
