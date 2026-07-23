package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"vermory/internal/provider"
)

func TestConversationReviewInboxExposesOnlyScopedExactEvidence(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	tenantID := "conversation-review"
	conversation := NewConversationService(store, tenantID, nil, "", ConversationServiceConfig{})
	anchorA := ConversationAnchor{Channel: "openclaw", ThreadID: "review-a"}
	anchorB := ConversationAnchor{Channel: "hermes", ThreadID: "review-b"}
	turnA := persistFormationConversationTurn(t, conversation, anchorA, "review-turn-a", "The submission bundle is thesis-defense-v7.zip.", "Acknowledged.")
	turnB := persistFormationConversationTurn(t, conversation, anchorB, "review-turn-b", "The printer room is C-204.", "Acknowledged.")

	outputA := fmt.Sprintf(`{"candidates":[{"decision":"new","memory_key":"submission.bundle.current","source_observation_id":%q,"quote":"The submission bundle is thesis-defense-v7.zip.","occurrence":1,"content":"The submission bundle is thesis-defense-v7.zip.","reason":"Explicit current bundle."}],"reason":"One candidate."}`, turnA.UserObservationID)
	formationA := NewSourceFormationService(store, tenantID, provider.Mock{Output: outputA}, "fixture-provider", "fixture-model")
	receiptA, err := formationA.FormConversation(ctx, ConversationFormationRequest{
		OperationID: "review-formation-a", Anchor: anchorA, ObservationIDs: []string{turnA.UserObservationID},
	})
	if err != nil || len(receiptA.Items) != 1 {
		t.Fatalf("formation A receipt=%#v err=%v", receiptA, err)
	}
	outputB := fmt.Sprintf(`{"candidates":[{"decision":"new","memory_key":"printer.room.current","source_observation_id":%q,"quote":"The printer room is C-204.","occurrence":1,"content":"The printer room is C-204.","reason":"Explicit room."}],"reason":"One candidate."}`, turnB.UserObservationID)
	formationB := NewSourceFormationService(store, tenantID, provider.Mock{Output: outputB}, "fixture-provider", "fixture-model")
	if _, err := formationB.FormConversation(ctx, ConversationFormationRequest{
		OperationID: "review-formation-b", Anchor: anchorB, ObservationIDs: []string{turnB.UserObservationID},
	}); err != nil {
		t.Fatal(err)
	}

	inbox, err := conversation.ReviewCandidates(ctx, anchorA)
	if err != nil {
		t.Fatal(err)
	}
	if inbox.Resolution.ContinuityID != turnA.ContinuityID || len(inbox.Candidates) != 1 {
		t.Fatalf("unexpected scoped review inbox: %#v", inbox)
	}
	candidate := inbox.Candidates[0]
	if candidate.CandidateMemoryID != receiptA.Items[0].CandidateMemoryID ||
		candidate.MemoryKey != "submission.bundle.current" ||
		candidate.Decision != SourceFormationNew ||
		candidate.SourceObservationID != turnA.UserObservationID ||
		candidate.SourceQuote != "The submission bundle is thesis-defense-v7.zip." ||
		candidate.Content != "The submission bundle is thesis-defense-v7.zip." ||
		candidate.TargetMemoryID != "" || candidate.CreatedAt.IsZero() {
		t.Fatalf("review candidate omitted safe evidence: %#v", candidate)
	}
	encoded, err := json.Marshal(inbox)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{
		"printer room", "C-204", "provider_output", "provider_artifact", "request_fingerprint",
		"active_snapshot", "prompt", "Explicit current bundle", "fixture-provider",
	} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("review inbox exposed %q: %s", forbidden, encoded)
		}
	}

	if _, err := conversation.AcceptCandidate(ctx, ReviewConversationCandidateRequest{
		OperationID: "review-accept-a", Anchor: anchorA, MemoryID: candidate.CandidateMemoryID,
	}); err != nil {
		t.Fatal(err)
	}
	after, err := conversation.ReviewCandidates(ctx, anchorA)
	if err != nil {
		t.Fatal(err)
	}
	if len(after.Candidates) != 0 {
		t.Fatalf("accepted candidate remained in review inbox: %#v", after)
	}
}
