package runtime

import (
	"context"
	"fmt"
)

const maxConversationReviewCandidates = 100

func (s *Store) ListConversationReviewCandidates(ctx context.Context, tenantID, continuityID string) ([]ConversationReviewCandidate, error) {
	ctx, err := withTenantContext(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `
SELECT candidate.id::text, candidate.memory_key, candidate.content,
       item.quote, item.evidence_observation_id::text, item.decision,
       COALESCE(item.target_memory_id::text, ''), candidate.created_at,
       evidence.observation_kind, COALESCE(tool.tool_name, '')
FROM governed_memories candidate
JOIN observations origin
  ON origin.tenant_id = candidate.tenant_id
 AND origin.continuity_id = candidate.continuity_id
 AND origin.id = candidate.origin_observation_id
JOIN source_formation_items item
  ON item.tenant_id = candidate.tenant_id
 AND item.continuity_id = candidate.continuity_id
 AND item.candidate_memory_id = candidate.id
JOIN source_formation_runs run
  ON run.tenant_id = item.tenant_id
 AND run.continuity_id = item.continuity_id
 AND run.id = item.run_id
JOIN observations evidence
  ON evidence.tenant_id = item.tenant_id
 AND evidence.continuity_id = item.continuity_id
 AND evidence.id = item.evidence_observation_id
LEFT JOIN conversation_tool_results tool
  ON tool.tenant_id = evidence.tenant_id
 AND tool.continuity_id = evidence.continuity_id
 AND tool.observation_id = evidence.id
WHERE candidate.tenant_id = $1
  AND candidate.continuity_id = $2::uuid
  AND candidate.lifecycle_status = 'proposed'
  AND origin.observation_kind = 'source_candidate'
  AND run.input_kind = 'conversation'
  AND run.status = 'completed'
  AND item.evidence_observation_id IS NOT NULL
  AND item.decision IN ('new', 'update')
ORDER BY candidate.created_at, candidate.id
LIMIT $3`, tenantID, continuityID, maxConversationReviewCandidates)
	if err != nil {
		return nil, fmt.Errorf("list conversation review candidates: %w", err)
	}
	defer rows.Close()
	candidates := make([]ConversationReviewCandidate, 0)
	for rows.Next() {
		var candidate ConversationReviewCandidate
		if err := rows.Scan(
			&candidate.CandidateMemoryID,
			&candidate.MemoryKey,
			&candidate.Content,
			&candidate.SourceQuote,
			&candidate.SourceObservationID,
			&candidate.Decision,
			&candidate.TargetMemoryID,
			&candidate.CreatedAt,
			&candidate.SourceKind,
			&candidate.SourceLabel,
		); err != nil {
			return nil, fmt.Errorf("scan conversation review candidate: %w", err)
		}
		candidates = append(candidates, candidate)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate conversation review candidates: %w", err)
	}
	return candidates, nil
}
