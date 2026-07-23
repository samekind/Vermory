package runtime

import (
	"context"
	"testing"
)

func TestConversationAnchorRejectsMissingThread(t *testing.T) {
	_, err := (ConversationAnchor{Channel: "web_chat"}).Normalized()
	if err == nil {
		t.Fatal("missing thread_id must be rejected")
	}
}

func TestResolveOrCreateConversationUsesExactChannelAndThread(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()

	first, err := store.ResolveOrCreateConversation(ctx, "local", ConversationAnchor{
		Channel:  "web_chat",
		ThreadID: "matter-1",
	})
	requireNoError(t, err)
	again, err := store.ResolveOrCreateConversation(ctx, "local", ConversationAnchor{
		Channel:  "web_chat",
		ThreadID: "matter-1",
	})
	requireNoError(t, err)
	other, err := store.ResolveOrCreateConversation(ctx, "local", ConversationAnchor{
		Channel:  "other",
		ThreadID: "matter-1",
	})
	requireNoError(t, err)

	if first.ContinuityID != again.ContinuityID {
		t.Fatalf("same exact anchor did not resolve consistently: first=%#v again=%#v", first, again)
	}
	if first.ContinuityID == other.ContinuityID {
		t.Fatalf("different channels shared a continuity: first=%#v other=%#v", first, other)
	}
}

func TestListRecentConversationObservationsIsChronologicalAndStopsBeforeCurrent(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	resolution, err := store.ResolveOrCreateConversation(ctx, "local", ConversationAnchor{
		Channel:  "web_chat",
		ThreadID: "matter-history",
	})
	requireNoError(t, err)

	for _, request := range []CommitObservationRequest{
		{OperationID: "history-1", Kind: ObservationKindUserMessage, Content: "first user turn"},
		{OperationID: "history-2", Kind: ObservationKindAssistantMessage, Content: "first assistant turn"},
	} {
		_, err := store.CommitObservation(ctx, "local", resolution.ContinuityID, request)
		requireNoError(t, err)
	}
	current, err := store.CommitObservation(ctx, "local", resolution.ContinuityID, CommitObservationRequest{
		OperationID: "history-3",
		Kind:        ObservationKindUserMessage,
		Content:     "current user turn",
	})
	requireNoError(t, err)

	recent, err := store.ListRecentConversationObservations(ctx, "local", resolution.ContinuityID, current.ObservationID, 12)
	requireNoError(t, err)
	if len(recent) != 2 {
		t.Fatalf("expected two prior observations, got %#v", recent)
	}
	if recent[0].Content != "first user turn" || recent[1].Content != "first assistant turn" {
		t.Fatalf("recent observations are not chronological: %#v", recent)
	}
}

func TestCommitObservationReplayRejectsChangedLogicalRequest(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	resolution, err := store.ResolveOrCreateConversation(ctx, "local", ConversationAnchor{
		Channel:  "web_chat",
		ThreadID: "matter-idempotency",
	})
	requireNoError(t, err)

	_, err = store.CommitObservation(ctx, "local", resolution.ContinuityID, CommitObservationRequest{
		OperationID: "same-operation",
		Kind:        ObservationKindUserMessage,
		Content:     "original message",
	})
	requireNoError(t, err)
	_, err = store.CommitObservation(ctx, "local", resolution.ContinuityID, CommitObservationRequest{
		OperationID: "same-operation",
		Kind:        ObservationKindUserMessage,
		Content:     "changed message",
	})
	if err == nil {
		t.Fatal("changed logical request must not replay an existing observation")
	}
}
