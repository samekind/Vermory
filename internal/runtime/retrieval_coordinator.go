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

type RetrievalCoordinator struct {
	store    *Store
	embedder Embedder
	profile  RetrievalProfile
}

func NewRetrievalCoordinator(store *Store, embedder Embedder, profile RetrievalProfile) (*RetrievalCoordinator, error) {
	if store == nil {
		return nil, fmt.Errorf("retrieval coordinator store is required")
	}
	if embedder == nil {
		if profile != (RetrievalProfile{}) {
			return nil, fmt.Errorf("retrieval embedder is required for a configured profile")
		}
		return &RetrievalCoordinator{store: store}, nil
	}
	if err := profile.Validate(); err != nil {
		return nil, err
	}
	return &RetrievalCoordinator{store: store, embedder: embedder, profile: profile}, nil
}

func (c *RetrievalCoordinator) Retrieve(ctx context.Context, request RetrievalRequest) (RetrievalResult, error) {
	if c == nil || c.store == nil {
		return RetrievalResult{}, fmt.Errorf("retrieval coordinator is not configured")
	}
	primaryContinuityID := ""
	if len(request.ContinuityIDs) > 0 {
		primaryContinuityID = strings.TrimSpace(request.ContinuityIDs[0])
	}
	normalized, err := request.normalized()
	if err != nil {
		return RetrievalResult{}, err
	}
	if normalized.EligibilityAsOf.IsZero() {
		existingAsOf, found, lookupErr := c.store.retrievalAuditEligibilityAsOf(
			ctx, normalized.TenantID, normalized.OperationID,
		)
		if lookupErr != nil {
			return RetrievalResult{}, lookupErr
		}
		if found {
			normalized.EligibilityAsOf = existingAsOf
		} else {
			snapshot, snapshotErr := c.store.CurrentEligibilitySnapshot(ctx, normalized.TenantID)
			if snapshotErr != nil {
				return RetrievalResult{}, snapshotErr
			}
			normalized.EligibilityAsOf = snapshot.AsOf
		}
	}
	scope, err := c.store.authorizeRetrievalScope(ctx, normalized.TenantID, primaryContinuityID, normalized.ContinuityIDs)
	if err != nil {
		return RetrievalResult{}, err
	}

	lexicalStarted := time.Now()
	lexical, err := c.lexical(ctx, normalized, scope)
	if err != nil {
		return RetrievalResult{}, err
	}
	lexicalLatency := time.Since(lexicalStarted)
	if normalized.Mode == RetrievalLexical {
		return RetrievalResult{
			Memories: lexical, Effective: RetrievalLexical,
			EligibilityAsOf: normalized.EligibilityAsOf,
		}, nil
	}
	if c.embedder == nil {
		return RetrievalResult{}, fmt.Errorf("semantic retrieval is not configured")
	}

	fingerprint, querySHA256, err := retrievalRequestFingerprint(normalized, c.profile.ID)
	if err != nil {
		return RetrievalResult{}, err
	}
	if _, err := c.store.checkRetrievalAuditReplay(ctx, normalized.TenantID, normalized.OperationID, fingerprint); err != nil {
		return RetrievalResult{}, err
	}

	projectionCurrent := false
	vectorMemories := []Memory{}
	vectorLatency := time.Duration(0)
	failureCode := ""
	status, statusErr := c.store.RetrievalProjectionStatus(ctx, normalized.TenantID, c.profile.ID)
	if statusErr != nil {
		failureCode = "projection_unavailable"
	} else if status.RebuildRequired {
		failureCode = ProjectionFailureRebuildRequired
	} else if status.Status != "idle" {
		failureCode = "projection_not_current"
	} else if status.Lag != 0 {
		failureCode = "projection_lag"
	} else {
		projectionCurrent = true
		vectorStarted := time.Now()
		queryVector, embedErr := c.embedder.Embed(ctx, normalized.Query)
		if embedErr != nil {
			failureCode = "embedding_unavailable"
		} else if len(queryVector) != c.profile.Dimensions {
			failureCode = "embedding_dimension_mismatch"
		} else {
			vectorMemories, err = c.store.searchEligibleVectorMemoryAt(
				ctx, normalized.TenantID, normalized.ContinuityIDs, queryVector,
				normalized.Limit, c.profile.ID, normalized.EligibilityAsOf,
			)
			if err != nil {
				failureCode = "vector_query_error"
				vectorMemories = []Memory{}
			} else if len(vectorMemories) == 0 && len(lexical) > 0 {
				failureCode = "vector_empty"
			}
		}
		vectorLatency = time.Since(vectorStarted)
	}

	effective := normalized.Mode
	delivered := vectorMemories
	degraded := failureCode != ""
	if normalized.Mode == RetrievalShadow || degraded {
		delivered = lexical
		if degraded {
			effective = RetrievalLexical
		}
	}
	auditID, err := c.store.recordRetrievalAudit(ctx, retrievalAuditInput{
		TenantID:            normalized.TenantID,
		PrimaryContinuityID: scope.PrimaryContinuityID,
		ContinuityIDs:       normalized.ContinuityIDs,
		OperationID:         normalized.OperationID,
		RequestFingerprint:  fingerprint,
		RequestedMode:       normalized.Mode,
		EffectiveMode:       effective,
		ProfileID:           c.profile.ID,
		QuerySHA256:         querySHA256,
		LexicalMemoryIDs:    retrievalMemoryIDs(lexical),
		VectorMemoryIDs:     retrievalMemoryIDs(vectorMemories),
		DeliveredMemoryIDs:  retrievalMemoryIDs(delivered),
		ProjectionCurrent:   projectionCurrent,
		Degraded:            degraded,
		FailureCode:         failureCode,
		LexicalLatency:      lexicalLatency,
		VectorLatency:       vectorLatency,
		EligibilityAsOf:     normalized.EligibilityAsOf,
	})
	if err != nil {
		return RetrievalResult{}, err
	}
	return RetrievalResult{
		Memories:        delivered,
		Effective:       effective,
		Degraded:        degraded,
		FailureCode:     failureCode,
		AuditID:         auditID,
		EligibilityAsOf: normalized.EligibilityAsOf,
	}, nil
}

func (c *RetrievalCoordinator) lexical(ctx context.Context, request RetrievalRequest, scope retrievalScope) ([]Memory, error) {
	if scope.Line == "workspace" {
		return c.store.SearchEligibleMemoryAt(
			ctx, request.TenantID, scope.PrimaryContinuityID,
			request.Query, request.Limit, request.EligibilityAsOf,
		)
	}
	return c.store.SearchEligibleConversationMemoryAt(
		ctx, request.TenantID, scope.PrimaryContinuityID,
		request.Query, request.Limit, request.EligibilityAsOf,
	)
}

type retrievalAuditInput struct {
	TenantID            string
	PrimaryContinuityID string
	ContinuityIDs       []string
	OperationID         string
	RequestFingerprint  string
	RequestedMode       RetrievalMode
	EffectiveMode       RetrievalMode
	ProfileID           string
	QuerySHA256         string
	LexicalMemoryIDs    []string
	VectorMemoryIDs     []string
	DeliveredMemoryIDs  []string
	ProjectionCurrent   bool
	Degraded            bool
	FailureCode         string
	LexicalLatency      time.Duration
	VectorLatency       time.Duration
	EligibilityAsOf     time.Time
}

func retrievalRequestFingerprint(request RetrievalRequest, profileID string) (string, string, error) {
	queryDigest := sha256.Sum256([]byte(request.Query))
	querySHA256 := hex.EncodeToString(queryDigest[:])
	payload := struct {
		TenantID        string        `json:"tenant_id"`
		ContinuityIDs   []string      `json:"continuity_ids"`
		QuerySHA256     string        `json:"query_sha256"`
		Limit           int           `json:"limit"`
		Mode            RetrievalMode `json:"mode"`
		ProfileID       string        `json:"profile_id"`
		EligibilityAsOf string        `json:"eligibility_as_of"`
	}{
		TenantID:        request.TenantID,
		ContinuityIDs:   request.ContinuityIDs,
		QuerySHA256:     querySHA256,
		Limit:           request.Limit,
		Mode:            request.Mode,
		ProfileID:       profileID,
		EligibilityAsOf: request.EligibilityAsOf.UTC().Format(time.RFC3339Nano),
	}
	canonical, err := json.Marshal(payload)
	if err != nil {
		return "", "", fmt.Errorf("encode retrieval request fingerprint: %w", err)
	}
	digest := sha256.Sum256(canonical)
	return hex.EncodeToString(digest[:]), querySHA256, nil
}

func retrievalMemoryIDs(memories []Memory) []string {
	ids := make([]string, len(memories))
	for index, memory := range memories {
		ids[index] = memory.ID
	}
	return ids
}

type retrievalScope struct {
	PrimaryContinuityID string
	ContinuityIDs       []string
	Line                string
}

func (s *Store) authorizeRetrievalScope(ctx context.Context, tenantID, primaryContinuityID string, requestedIDs []string) (retrievalScope, error) {
	tenantCtx, err := withTenantContext(ctx, tenantID)
	if err != nil {
		return retrievalScope{}, err
	}
	var line string
	if err := s.pool.QueryRow(tenantCtx, `
SELECT continuity_line
FROM continuity_spaces
WHERE tenant_id = $1 AND id = $2::uuid AND state = 'active'`, tenantID, primaryContinuityID).Scan(&line); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return retrievalScope{}, fmt.Errorf("active retrieval continuity was not found")
		}
		return retrievalScope{}, fmt.Errorf("authorize retrieval continuity: %w", err)
	}
	if line == "workspace" {
		if len(requestedIDs) != 1 || requestedIDs[0] != primaryContinuityID {
			return retrievalScope{}, fmt.Errorf("workspace retrieval requires exactly one continuity")
		}
		return retrievalScope{PrimaryContinuityID: primaryContinuityID, ContinuityIDs: requestedIDs, Line: line}, nil
	}
	if line != "conversation" {
		return retrievalScope{}, fmt.Errorf("unsupported retrieval continuity line")
	}
	resolved, err := s.ResolveLinkedConversationContinuityIDs(ctx, tenantID, primaryContinuityID)
	if err != nil {
		return retrievalScope{}, err
	}
	if !sameRetrievalContinuityIDs(resolved, requestedIDs) {
		return retrievalScope{}, fmt.Errorf("conversation retrieval continuity set is stale or unauthorized")
	}
	return retrievalScope{PrimaryContinuityID: primaryContinuityID, ContinuityIDs: resolved, Line: line}, nil
}

func sameRetrievalContinuityIDs(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
