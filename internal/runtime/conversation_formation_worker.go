package runtime

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

type ConversationFormationWorker struct {
	store     *Store
	formation *SourceFormationService
	options   ConversationFormationWorkerOptions
}

func NewConversationFormationWorker(store *Store, formation *SourceFormationService, options ConversationFormationWorkerOptions) (*ConversationFormationWorker, error) {
	options.TenantID = strings.TrimSpace(options.TenantID)
	if store == nil {
		return nil, fmt.Errorf("conversation formation worker store is required")
	}
	if formation == nil {
		return nil, fmt.Errorf("conversation formation worker service is required")
	}
	if options.TenantID == "" {
		return nil, fmt.Errorf("conversation formation worker tenant_id is required")
	}
	if options.PollInterval <= 0 {
		options.PollInterval = time.Second
	}
	if options.LeaseDuration <= 0 {
		options.LeaseDuration = 2 * time.Minute
	}
	if options.RetryDelay <= 0 {
		options.RetryDelay = 30 * time.Second
	}
	if options.MaxAttempts <= 0 {
		options.MaxAttempts = 5
	}
	return &ConversationFormationWorker{store: store, formation: formation, options: options}, nil
}

func (worker *ConversationFormationWorker) Run(ctx context.Context) error {
	ticker := time.NewTicker(worker.options.PollInterval)
	defer ticker.Stop()
	for {
		if _, err := worker.RunOnce(ctx); err != nil && !errors.Is(err, context.Canceled) {
			// Retryable provider and lease failures are durable schedule state.
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (worker *ConversationFormationWorker) RunOnce(ctx context.Context) (ConversationFormationWorkerResult, error) {
	claim, found, err := worker.store.ClaimConversationFormationWithLimit(
		ctx, worker.options.TenantID, worker.options.LeaseDuration, worker.options.RetryDelay, worker.options.MaxAttempts,
	)
	if err != nil {
		return ConversationFormationWorkerResult{TenantID: worker.options.TenantID}, err
	}
	if !found {
		return ConversationFormationWorkerResult{TenantID: worker.options.TenantID}, nil
	}
	result := ConversationFormationWorkerResult{
		TenantID:     worker.options.TenantID,
		ContinuityID: claim.Schedule.ContinuityID,
		OperationID:  claim.Schedule.ActiveOperationID,
		Found:        true,
	}
	observationIDs := make([]string, len(claim.Observations))
	for index, observation := range claim.Observations {
		observationIDs[index] = observation.ID
	}
	receipt, formationErr := worker.formation.FormConversationContinuity(ctx, claim.Schedule.ContinuityID, claim.Schedule.ActiveOperationID, observationIDs)
	result.FormationRunID = receipt.ID
	result.FormationStatus = receipt.Status
	result.FailureCode = receipt.FailureCode
	result.Replayed = receipt.Replayed
	if formationErr != nil {
		_, retryErr := worker.store.RetryConversationFormationClaim(
			context.WithoutCancel(ctx), worker.options.TenantID, claim, receipt, "worker_error", worker.options.RetryDelay,
		)
		if retryErr != nil {
			return result, errors.Join(formationErr, retryErr)
		}
		return result, formationErr
	}
	if receipt.Status == SourceFormationFailed {
		_, retryErr := worker.store.RetryConversationFormationClaim(
			context.WithoutCancel(ctx), worker.options.TenantID, claim, receipt, receipt.FailureCode, worker.options.RetryDelay,
		)
		if retryErr != nil {
			return result, retryErr
		}
		return result, fmt.Errorf("automatic conversation formation failed: %s", receipt.FailureCode)
	}
	if receipt.Status != SourceFormationCompleted && receipt.Status != SourceFormationAbstained {
		_, retryErr := worker.store.RetryConversationFormationClaim(
			context.WithoutCancel(ctx), worker.options.TenantID, claim, receipt, "unexpected_status", worker.options.RetryDelay,
		)
		if retryErr != nil {
			return result, retryErr
		}
		return result, fmt.Errorf("automatic conversation formation returned unexpected status %q", receipt.Status)
	}
	schedule, err := worker.store.CompleteConversationFormationClaim(
		context.WithoutCancel(ctx), worker.options.TenantID, claim, receipt,
	)
	if err != nil {
		return result, err
	}
	result.ProcessedThroughSequence = schedule.ProcessedThroughSequence
	return result, nil
}
