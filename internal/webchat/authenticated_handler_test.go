package webchat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"vermory/internal/authn"
	"vermory/internal/provider"
	"vermory/internal/runtime"
)

func TestAuthenticatedHandlerRejectsMissingMalformedUnknownExpiredAndRevokedTokens(t *testing.T) {
	_, store := testHandler(t, provider.Mock{Output: "unused"})
	authenticator := staticAuthenticator{
		errors: map[string]error{
			"unknown-token": ErrSyntheticUnknownToken,
			"expired-token": authn.ErrAuthenticationFailed,
			"revoked-token": authn.ErrAuthenticationFailed,
		},
	}
	handler := NewAuthenticatedHandler(store, provider.Mock{Output: "unused"}, "test-model", authenticator)

	tests := []struct {
		name   string
		header []string
	}{
		{name: "missing"},
		{name: "basic", header: []string{"Basic abc"}},
		{name: "empty bearer", header: []string{"Bearer "}},
		{name: "multiple", header: []string{"Bearer unknown-token", "Bearer other-token"}},
		{name: "unknown", header: []string{"Bearer unknown-token"}},
		{name: "expired", header: []string{"Bearer expired-token"}},
		{name: "revoked", header: []string{"Bearer revoked-token"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/v1/chat/turn", strings.NewReader(`{"operation_id":"auth-fail","channel":"web_chat","thread_id":"auth","message":"hello"}`))
			request.Header.Set("Content-Type", "application/json")
			for _, value := range test.header {
				request.Header.Add("Authorization", value)
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != http.StatusUnauthorized {
				t.Fatalf("unexpected status %d: %s", response.Code, response.Body.String())
			}
			for _, secret := range []string{"unknown-token", "expired-token", "revoked-token", "identity-a"} {
				if strings.Contains(response.Body.String(), secret) {
					t.Fatalf("authentication error leaked %q: %s", secret, response.Body.String())
				}
			}
		})
	}
}

func TestAuthenticatedBrowserAssetsAndSessionBoundary(t *testing.T) {
	store := &runtime.Store{}
	authenticator := staticAuthenticator{principals: map[string]authn.Principal{
		"client-a":   principal("identity-a", authn.RoleClient),
		"operator-a": principal("identity-a", authn.RoleOperator),
		"client-b":   principal("identity-b", authn.RoleClient),
	}}
	handler := NewAuthenticatedHandler(store, provider.Mock{Output: "unused"}, "test-model", authenticator)

	for _, path := range []string{"/", "/assets/app.css", "/assets/app.js", "/v1/browser/runtime"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusOK {
			t.Fatalf("anonymous browser asset %s returned %d: %s", path, response.Code, response.Body.String())
		}
		if response.Header().Get("Content-Security-Policy") == "" {
			t.Fatalf("anonymous browser asset %s omitted browser protections", path)
		}
	}
	runtimeResponse := httptest.NewRecorder()
	handler.ServeHTTP(runtimeResponse, httptest.NewRequest(http.MethodGet, "/v1/browser/runtime", nil))
	if runtimeResponse.Code != http.StatusOK || runtimeResponse.Body.String() != "{\"mode\":\"authenticated\"}\n" {
		t.Fatalf("anonymous browser runtime returned %d: %s", runtimeResponse.Code, runtimeResponse.Body.String())
	}

	missing := httptest.NewRecorder()
	handler.ServeHTTP(missing, httptest.NewRequest(http.MethodGet, "/v1/session", nil))
	if missing.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous session returned %d: %s", missing.Code, missing.Body.String())
	}

	sessions := make(map[string]map[string]any)
	for _, token := range []string{"client-a", "operator-a", "client-b"} {
		response := performAuthenticatedJSON(t, handler, token, http.MethodGet, "/v1/session", "")
		if response.Code != http.StatusOK {
			t.Fatalf("session %s returned %d: %s", token, response.Code, response.Body.String())
		}
		if response.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("session %s may be cached: %q", token, response.Header().Get("Cache-Control"))
		}
		var payload map[string]any
		decodeResponse(t, response, &payload)
		if len(payload) != 2 || payload["role"] == "" || payload["storage_scope"] == "" {
			t.Fatalf("session %s exposed the wrong capability shape: %#v", token, payload)
		}
		for _, forbidden := range []string{"tenant_id", "subject_id", "token_id", "expires_at", "token", "credential"} {
			if _, ok := payload[forbidden]; ok {
				t.Fatalf("session %s exposed %s: %#v", token, forbidden, payload)
			}
		}
		sessions[token] = payload
	}
	if sessions["client-a"]["storage_scope"] != sessions["operator-a"]["storage_scope"] {
		t.Fatal("same-tenant browser identities received different storage scopes")
	}
	if sessions["client-a"]["storage_scope"] == sessions["client-b"]["storage_scope"] {
		t.Fatal("different tenants received the same browser storage scope")
	}
}

func TestAuthenticatedHandlerUsesPrincipalTenantAndRolePolicy(t *testing.T) {
	_, store := testHandler(t, provider.Mock{Output: "unused"})
	authenticator := staticAuthenticator{principals: map[string]authn.Principal{
		"client-a":   principal("identity-a", authn.RoleClient),
		"operator-a": principal("identity-a", authn.RoleOperator),
	}}
	handler := NewAuthenticatedHandler(store, provider.Mock{Output: "authenticated answer"}, "test-model", authenticator)

	chat := performAuthenticatedJSON(t, handler, "client-a", http.MethodPost, "/v1/chat/turn", `{
  "operation_id":"authenticated-chat-a",
  "channel":"web_chat",
  "thread_id":"shared-anchor",
  "message":"hello"
}`)
	if chat.Code != http.StatusOK {
		t.Fatalf("client chat failed: %d %s", chat.Code, chat.Body.String())
	}
	clientInspection := performAuthenticatedJSON(t, handler, "client-a", http.MethodGet, "/v1/conversations/inspect?channel=web_chat&thread_id=shared-anchor", "")
	if clientInspection.Code != http.StatusOK {
		t.Fatalf("client conversation inspection failed: %d %s", clientInspection.Code, clientInspection.Body.String())
	}
	resolution, err := store.ResolveConversation(context.Background(), "identity-a", runtime.ConversationAnchor{Channel: "web_chat", ThreadID: "shared-anchor"})
	if err != nil || resolution.Status != runtime.ResolutionResolved {
		t.Fatalf("principal tenant was not persisted: resolution=%#v err=%v", resolution, err)
	}

	hermes := performAuthenticatedJSON(t, handler, "client-a", http.MethodPost, "/v1/integrations/hermes/turns/prepare", `{
  "operation_id":"authenticated-hermes-a",
  "session_key":"profile:personal:thread-a",
  "message":"continue"
}`)
	if hermes.Code != http.StatusOK {
		t.Fatalf("client Hermes prepare failed: %d %s", hermes.Code, hermes.Body.String())
	}
	hermesResolution, err := store.ResolveConversation(context.Background(), "identity-a", runtime.ConversationAnchor{Channel: "hermes", ThreadID: "profile:personal:thread-a"})
	if err != nil || hermesResolution.Status != runtime.ResolutionResolved {
		t.Fatalf("authenticated Hermes anchor was not tenant-scoped: resolution=%#v err=%v", hermesResolution, err)
	}

	clientGovernance := performAuthenticatedJSON(t, handler, "client-a", http.MethodPost, "/v1/defaults/set", `{
  "operation_id":"client-default-denied",
  "key":"reply_language",
  "content":"Chinese"
}`)
	if clientGovernance.Code != http.StatusForbidden {
		t.Fatalf("client governance returned %d: %s", clientGovernance.Code, clientGovernance.Body.String())
	}

	operatorGovernance := performAuthenticatedJSON(t, handler, "operator-a", http.MethodPost, "/v1/defaults/set", `{
  "operation_id":"operator-default-a",
  "key":"reply_language",
  "content":"Default replies to Chinese."
}`)
	if operatorGovernance.Code != http.StatusOK {
		t.Fatalf("operator governance failed: %d %s", operatorGovernance.Code, operatorGovernance.Body.String())
	}
	inspection, err := runtime.NewGlobalDefaultsService(store, "identity-a").Inspect(context.Background())
	if err != nil || len(inspection.Defaults) != 1 {
		t.Fatalf("operator mutation was not tenant-scoped: inspection=%#v err=%v", inspection, err)
	}
}

func TestAuthenticatedOpenClawToolResultUsesClientTenantAndHidesRejectedContent(t *testing.T) {
	_, store := testHandler(t, provider.Mock{Output: "unused"})
	authenticator := staticAuthenticator{principals: map[string]authn.Principal{
		"client-tool": principal("identity-tool", authn.RoleClient),
	}}
	handler := NewAuthenticatedHandler(store, provider.Mock{Output: "unused"}, "test-model", authenticator)

	prepared := performAuthenticatedJSON(t, handler, "client-tool", http.MethodPost, "/v1/integrations/openclaw/turns/prepare", `{
  "operation_id":"openclaw:run-tool-http",
  "session_key":"agent:main:tool-http",
  "message":"Inspect the keyboard state."
}`)
	if prepared.Code != http.StatusOK {
		t.Fatalf("tool turn prepare failed: %d %s", prepared.Code, prepared.Body.String())
	}

	accepted := performAuthenticatedJSON(t, handler, "client-tool", http.MethodPost, "/v1/integrations/openclaw/turns/tool-results", `{
  "operation_id":"openclaw:run-tool-http",
  "session_key":"agent:main:tool-http",
  "run_id":"run-tool-http",
  "tool_name":"device.keyboard_diagnostic",
  "tool_call_id":"call-tool-http",
  "content":"Keyboard diagnostic processed 1,333,470 events."
}`)
	if accepted.Code != http.StatusOK {
		t.Fatalf("client tool result failed: %d %s", accepted.Code, accepted.Body.String())
	}
	var receipt runtime.ConversationToolResultReceipt
	decodeResponse(t, accepted, &receipt)
	if receipt.ObservationID == "" || receipt.ToolName != "device.keyboard_diagnostic" || receipt.Replayed {
		t.Fatalf("unexpected tool result receipt: %#v", receipt)
	}

	requestAuthority := performAuthenticatedJSON(t, handler, "client-tool", http.MethodPost, "/v1/integrations/openclaw/turns/tool-results", `{
  "operation_id":"openclaw:run-tool-http",
  "session_key":"agent:main:tool-http",
  "run_id":"run-tool-http",
  "tool_name":"device.keyboard_diagnostic",
  "tool_call_id":"call-request-authority",
  "content":"ok",
  "tenant_id":"attacker"
}`)
	if requestAuthority.Code != http.StatusBadRequest || strings.Contains(requestAuthority.Body.String(), "attacker") {
		t.Fatalf("request-owned authority was not rejected safely: %d %s", requestAuthority.Code, requestAuthority.Body.String())
	}

	const sensitive = "api_token=W23_SYNTHETIC_SECRET_MUST_NOT_PERSIST"
	rejected := performAuthenticatedJSON(t, handler, "client-tool", http.MethodPost, "/v1/integrations/openclaw/turns/tool-results", `{
  "operation_id":"openclaw:run-tool-http",
  "session_key":"agent:main:tool-http",
  "run_id":"run-tool-http",
  "tool_name":"device.debug",
  "tool_call_id":"call-sensitive-http",
  "content":"`+sensitive+`"
}`)
	if rejected.Code != http.StatusBadRequest {
		t.Fatalf("sensitive tool result returned %d: %s", rejected.Code, rejected.Body.String())
	}
	if strings.Contains(rejected.Body.String(), sensitive) || strings.Contains(rejected.Body.String(), "device.debug") {
		t.Fatalf("rejected tool result leaked input: %s", rejected.Body.String())
	}

	resolution, err := store.ResolveConversation(context.Background(), "identity-tool", runtime.ConversationAnchor{
		Channel: "openclaw", ThreadID: "agent:main:tool-http",
	})
	if err != nil || resolution.Status != runtime.ResolutionResolved {
		t.Fatalf("authenticated tool result did not use principal tenant: resolution=%#v err=%v", resolution, err)
	}
	other, err := store.ResolveConversation(context.Background(), "attacker", runtime.ConversationAnchor{
		Channel: "openclaw", ThreadID: "agent:main:tool-http",
	})
	if err != nil || other.Status != runtime.ResolutionUnresolved {
		t.Fatalf("request-owned tenant gained a binding: resolution=%#v err=%v", other, err)
	}
}

func TestAuthenticatedConversationCandidateReviewIsOperatorOnlyAndSessionScoped(t *testing.T) {
	_, store := testHandler(t, provider.Mock{Output: "unused"})
	tenantID := "review-http"
	authenticator := staticAuthenticator{principals: map[string]authn.Principal{
		"client-review":   principal(tenantID, authn.RoleClient),
		"operator-review": principal(tenantID, authn.RoleOperator),
	}}
	handler := NewAuthenticatedHandler(store, nil, "", authenticator)
	openClawAnchor := runtime.ConversationAnchor{Channel: "openclaw", ThreadID: "agent:main:review"}
	hermesAnchor := runtime.ConversationAnchor{Channel: "hermes", ThreadID: "profile:personal:review"}
	openClawItems := seedAuthenticatedReviewCandidates(t, store, tenantID, openClawAnchor, "review-http-openclaw", []reviewSeed{
		{key: "submission.bundle.current", text: "The submission bundle is thesis-defense-v7.zip."},
		{key: "submission.deadline.current", text: "The deadline is Tuesday at 18:00."},
	})
	hermesItems := seedAuthenticatedReviewCandidates(t, store, tenantID, hermesAnchor, "review-http-hermes", []reviewSeed{
		{key: "printer.room.current", text: "The printer room is C-204."},
	})

	listPath := "/v1/memories/candidates?channel=openclaw&thread_id=agent%3Amain%3Areview"
	clientList := performAuthenticatedJSON(t, handler, "client-review", http.MethodGet, listPath, "")
	if clientList.Code != http.StatusForbidden {
		t.Fatalf("client candidate list returned %d: %s", clientList.Code, clientList.Body.String())
	}
	clientAccept := performAuthenticatedJSON(t, handler, "client-review", http.MethodPost, "/v1/memories/candidates/accept", fmt.Sprintf(`{
  "operation_id":"client-review-accept",
  "channel":"openclaw",
  "thread_id":"agent:main:review",
  "candidate_memory_id":%q
}`, openClawItems[0].CandidateMemoryID))
	if clientAccept.Code != http.StatusForbidden {
		t.Fatalf("client candidate acceptance returned %d: %s", clientAccept.Code, clientAccept.Body.String())
	}

	operatorList := performAuthenticatedJSON(t, handler, "operator-review", http.MethodGet, listPath, "")
	if operatorList.Code != http.StatusOK {
		t.Fatalf("operator candidate list returned %d: %s", operatorList.Code, operatorList.Body.String())
	}
	var inbox runtime.ConversationReviewInbox
	decodeResponse(t, operatorList, &inbox)
	if len(inbox.Candidates) != 2 {
		t.Fatalf("operator inbox candidates=%d: %#v", len(inbox.Candidates), inbox)
	}
	for _, forbidden := range []string{"C-204", "printer.room.current", "fixture-provider", "provider_output", "request_fingerprint", "Explicit current fact"} {
		if strings.Contains(operatorList.Body.String(), forbidden) {
			t.Fatalf("operator inbox exposed %q: %s", forbidden, operatorList.Body.String())
		}
	}

	crossSession := performAuthenticatedJSON(t, handler, "operator-review", http.MethodPost, "/v1/memories/candidates/accept", fmt.Sprintf(`{
  "operation_id":"operator-review-cross-session",
  "channel":"openclaw",
  "thread_id":"agent:main:review",
  "candidate_memory_id":%q
}`, hermesItems[0].CandidateMemoryID))
	if crossSession.Code != http.StatusBadRequest && crossSession.Code != http.StatusNotFound {
		t.Fatalf("cross-session acceptance returned %d: %s", crossSession.Code, crossSession.Body.String())
	}
	if strings.Contains(crossSession.Body.String(), hermesItems[0].CandidateMemoryID) || strings.Contains(crossSession.Body.String(), "C-204") {
		t.Fatalf("cross-session rejection leaked resource details: %s", crossSession.Body.String())
	}

	accepted := performAuthenticatedJSON(t, handler, "operator-review", http.MethodPost, "/v1/memories/candidates/accept", fmt.Sprintf(`{
  "operation_id":"operator-review-accept",
  "channel":"openclaw",
  "thread_id":"agent:main:review",
  "candidate_memory_id":%q
}`, openClawItems[0].CandidateMemoryID))
	if accepted.Code != http.StatusOK {
		t.Fatalf("operator acceptance returned %d: %s", accepted.Code, accepted.Body.String())
	}
	rejected := performAuthenticatedJSON(t, handler, "operator-review", http.MethodPost, "/v1/memories/candidates/reject", fmt.Sprintf(`{
  "operation_id":"operator-review-reject",
  "channel":"openclaw",
  "thread_id":"agent:main:review",
  "candidate_memory_id":%q
}`, openClawItems[1].CandidateMemoryID))
	if rejected.Code != http.StatusOK {
		t.Fatalf("operator rejection returned %d: %s", rejected.Code, rejected.Body.String())
	}
	after := performAuthenticatedJSON(t, handler, "operator-review", http.MethodGet, listPath, "")
	if after.Code != http.StatusOK {
		t.Fatalf("post-review list returned %d: %s", after.Code, after.Body.String())
	}
	decodeResponse(t, after, &inbox)
	if len(inbox.Candidates) != 0 {
		t.Fatalf("reviewed candidates remained pending: %#v", inbox)
	}
}

type reviewSeed struct {
	key  string
	text string
}

func seedAuthenticatedReviewCandidates(t *testing.T, store *runtime.Store, tenantID string, anchor runtime.ConversationAnchor, operationPrefix string, seeds []reviewSeed) []runtime.SourceFormationItemReceipt {
	t.Helper()
	conversation := runtime.NewConversationService(store, tenantID, nil, "", runtime.ConversationServiceConfig{})
	observationIDs := make([]string, len(seeds))
	candidates := make([]map[string]any, len(seeds))
	for index, seed := range seeds {
		operationID := fmt.Sprintf("%s-turn-%d", operationPrefix, index)
		_, err := conversation.PrepareExternalTurn(context.Background(), runtime.ExternalConversationTurnRequest{
			OperationID: operationID, Anchor: anchor, Message: seed.text,
		})
		if err != nil {
			t.Fatal(err)
		}
		completed, err := conversation.CompleteExternalTurn(context.Background(), runtime.CompleteExternalConversationTurnRequest{
			OperationID: operationID, Anchor: anchor, Answer: "Acknowledged.", Model: "fixture-client",
		})
		if err != nil {
			t.Fatal(err)
		}
		observationIDs[index] = completed.UserObservationID
		candidates[index] = map[string]any{
			"decision": "new", "memory_key": seed.key,
			"source_observation_id": completed.UserObservationID,
			"quote":                 seed.text, "occurrence": 1, "content": seed.text,
			"reason": "Explicit current fact.",
		}
	}
	payload, err := json.Marshal(map[string]any{"candidates": candidates, "reason": "Review candidates."})
	if err != nil {
		t.Fatal(err)
	}
	formation := runtime.NewSourceFormationService(store, tenantID, provider.Mock{Output: string(payload)}, "fixture-provider", "fixture-model")
	receipt, err := formation.FormConversation(context.Background(), runtime.ConversationFormationRequest{
		OperationID: operationPrefix + "-formation", Anchor: anchor, ObservationIDs: observationIDs,
	})
	if err != nil {
		t.Fatal(err)
	}
	return receipt.Items
}

func TestAuthenticatedHandlerPassesPrincipalTenantToRetrieverWithoutExposingMetadata(t *testing.T) {
	_, store := testHandler(t, provider.Mock{Output: "unused"})
	authenticator := staticAuthenticator{principals: map[string]authn.Principal{
		"client-semantic": principal("identity-semantic", authn.RoleClient),
	}}
	retriever := &authenticatedRecordingRetriever{}
	handler := NewAuthenticatedHandlerWithRetriever(store, provider.Mock{Output: "semantic answer"}, "test-model", authenticator, retriever)
	response := performAuthenticatedJSON(t, handler, "client-semantic", http.MethodPost, "/v1/chat/turn", `{
  "operation_id":"authenticated-semantic-turn",
  "channel":"web_chat",
  "thread_id":"semantic-thread",
  "message":"What is the rollback approval rule?"
}`)
	if response.Code != http.StatusOK {
		t.Fatalf("semantic chat failed: %d %s", response.Code, response.Body.String())
	}
	if len(retriever.requests) != 1 {
		t.Fatalf("retriever calls=%d", len(retriever.requests))
	}
	request := retriever.requests[0]
	if request.TenantID != "identity-semantic" || request.OperationID != "conversation-retrieval:authenticated-semantic-turn" || len(request.ContinuityIDs) != 1 {
		t.Fatalf("authenticated retrieval request used the wrong authority: %#v", request)
	}
	for _, internal := range []string{"vector", runtime.ProductionRetrievalProfileID, "55555555-5555-5555-5555-555555555555", "retrieval_mode", "audit_id"} {
		if strings.Contains(response.Body.String(), internal) {
			t.Fatalf("authenticated response exposed retrieval metadata %q: %s", internal, response.Body.String())
		}
	}
}

type authenticatedRecordingRetriever struct {
	requests []runtime.RetrievalRequest
}

func (retriever *authenticatedRecordingRetriever) Retrieve(_ context.Context, request runtime.RetrievalRequest) (runtime.RetrievalResult, error) {
	retriever.requests = append(retriever.requests, request)
	return runtime.RetrievalResult{
		Memories:  []runtime.Memory{{ID: "66666666-6666-6666-6666-666666666666", Content: "Rollback requires two maintainers."}},
		Effective: runtime.RetrievalVector,
		AuditID:   "55555555-5555-5555-5555-555555555555",
	}, nil
}

func TestAuthenticatedHandlerHidesCrossTenantResourcesAndRejectsRequestAuthority(t *testing.T) {
	_, store := testHandler(t, provider.Mock{Output: "unused"})
	authenticator := staticAuthenticator{principals: map[string]authn.Principal{
		"operator-a": principal("identity-a", authn.RoleOperator),
		"operator-b": principal("identity-b", authn.RoleOperator),
	}}
	handler := NewAuthenticatedHandler(store, provider.Mock{Output: "unused"}, "test-model", authenticator)

	createdResponse := performAuthenticatedJSON(t, handler, "operator-a", http.MethodPost, "/v1/defaults/set", `{
  "operation_id":"cross-tenant-default-a",
  "key":"private_fact",
  "content":"TENANT-A-ONLY"
}`)
	if createdResponse.Code != http.StatusOK {
		t.Fatalf("seed default failed: %d %s", createdResponse.Code, createdResponse.Body.String())
	}
	var created runtime.GlobalDefaultMutationReceipt
	decodeResponse(t, createdResponse, &created)

	crossTenant := performAuthenticatedJSON(t, handler, "operator-b", http.MethodPost, "/v1/defaults/correct", `{
  "operation_id":"cross-tenant-attack-b",
  "memory_id":"`+created.MemoryID+`",
  "content":"attacker replacement"
}`)
	if crossTenant.Code != http.StatusBadRequest && crossTenant.Code != http.StatusNotFound {
		t.Fatalf("unexpected cross-tenant status %d: %s", crossTenant.Code, crossTenant.Body.String())
	}
	for _, forbidden := range []string{created.MemoryID, "identity-a", "identity-b", "operator-b", "TENANT-A-ONLY"} {
		if strings.Contains(crossTenant.Body.String(), forbidden) {
			t.Fatalf("cross-tenant error leaked %q: %s", forbidden, crossTenant.Body.String())
		}
	}

	authority := performAuthenticatedJSON(t, handler, "operator-b", http.MethodPost, "/v1/chat/turn", `{
  "operation_id":"authority-attack",
  "channel":"web_chat",
  "thread_id":"shared-anchor",
  "message":"hello",
  "tenant_id":"identity-a"
}`)
	if authority.Code != http.StatusBadRequest {
		t.Fatalf("request-owned tenant was accepted: %d %s", authority.Code, authority.Body.String())
	}
	if strings.Contains(authority.Body.String(), "identity-a") {
		t.Fatalf("authority rejection echoed tenant: %s", authority.Body.String())
	}
}

type staticAuthenticator struct {
	principals map[string]authn.Principal
	errors     map[string]error
}

func (authenticator staticAuthenticator) Authenticate(_ context.Context, raw string) (authn.Principal, error) {
	if err := authenticator.errors[raw]; err != nil {
		return authn.Principal{}, err
	}
	if principal, ok := authenticator.principals[raw]; ok {
		return principal, nil
	}
	return authn.Principal{}, authn.ErrAuthenticationFailed
}

func principal(tenantID string, role authn.Role) authn.Principal {
	return authn.Principal{
		TokenID:   "synthetic-token-id",
		TenantID:  tenantID,
		SubjectID: "synthetic-subject",
		Role:      role,
		ExpiresAt: time.Now().UTC().Add(time.Hour),
	}
}

func performAuthenticatedJSON(t *testing.T, handler http.Handler, token, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+token)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

var ErrSyntheticUnknownToken = errors.Join(authn.ErrAuthenticationFailed, errors.New("synthetic lookup detail that must stay private"))
