package webchat

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"vermory/internal/runtime"
)

const maxRequestBodyBytes int64 = 1 << 20

type Handler struct {
	service  *runtime.ConversationService
	defaults *runtime.GlobalDefaultsService
	bridges  *runtime.BridgeService
	mux      *http.ServeMux
}

func NewHandler(service *runtime.ConversationService, defaultServices ...*runtime.GlobalDefaultsService) http.Handler {
	var defaults *runtime.GlobalDefaultsService
	if len(defaultServices) > 0 {
		defaults = defaultServices[0]
	}
	return newHandler(service, defaults, nil)
}

func NewHandlerWithGovernance(service *runtime.ConversationService, defaults *runtime.GlobalDefaultsService, bridges *runtime.BridgeService) http.Handler {
	return newHandler(service, defaults, bridges)
}

func newHandler(service *runtime.ConversationService, defaults *runtime.GlobalDefaultsService, bridges *runtime.BridgeService) http.Handler {
	handler := &Handler{service: service, defaults: defaults, bridges: bridges, mux: http.NewServeMux()}
	handler.registerBrowserApp()
	handler.mux.HandleFunc("POST /v1/chat/turn", handler.chatTurn)
	handler.mux.HandleFunc("POST /v1/integrations/openclaw/turns/prepare", handler.prepareOpenClawTurn)
	handler.mux.HandleFunc("POST /v1/integrations/openclaw/turns/complete", handler.completeOpenClawTurn)
	handler.mux.HandleFunc("POST /v1/integrations/openclaw/turns/fail", handler.failOpenClawTurn)
	handler.mux.HandleFunc("POST /v1/integrations/openclaw/turns/tool-results", handler.recordOpenClawToolResult)
	handler.mux.HandleFunc("POST /v1/integrations/hermes/turns/prepare", handler.prepareHermesTurn)
	handler.mux.HandleFunc("POST /v1/integrations/hermes/turns/complete", handler.completeHermesTurn)
	handler.mux.HandleFunc("POST /v1/integrations/hermes/turns/fail", handler.failHermesTurn)
	handler.mux.HandleFunc("POST /v1/memories/confirm", handler.confirmMemory)
	handler.mux.HandleFunc("GET /v1/memories/candidates", handler.listMemoryCandidates)
	handler.mux.HandleFunc("POST /v1/memories/candidates/accept", handler.acceptMemoryCandidate)
	handler.mux.HandleFunc("POST /v1/memories/candidates/reject", handler.rejectMemoryCandidate)
	handler.mux.HandleFunc("POST /v1/memories/correct", handler.correctMemory)
	handler.mux.HandleFunc("POST /v1/memories/forget", handler.forgetMemory)
	handler.mux.HandleFunc("GET /v1/conversations/inspect", handler.inspectConversation)
	handler.mux.HandleFunc("GET /v1/defaults", handler.inspectDefaults)
	handler.mux.HandleFunc("POST /v1/defaults/set", handler.setDefault)
	handler.mux.HandleFunc("POST /v1/defaults/correct", handler.correctDefault)
	handler.mux.HandleFunc("POST /v1/defaults/forget", handler.forgetDefault)
	handler.mux.HandleFunc("POST /v1/bridges/promote", handler.promoteBridge)
	handler.mux.HandleFunc("POST /v1/bridges/link", handler.linkBridge)
	handler.mux.HandleFunc("POST /v1/bridges/export", handler.exportBridge)
	handler.mux.HandleFunc("POST /v1/bridges/adopt", handler.adoptBridge)
	handler.mux.HandleFunc("POST /v1/bridges/rebind", handler.rebindBridge)
	handler.mux.HandleFunc("POST /v1/bridges/reverse", handler.reverseBridge)
	handler.mux.HandleFunc("GET /v1/bridges/{bridge_id}", handler.inspectBridge)
	return handler
}

func (h *Handler) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	h.mux.ServeHTTP(response, request)
}

type conversationInput struct {
	OperationID string `json:"operation_id"`
	Channel     string `json:"channel"`
	ThreadID    string `json:"thread_id"`
}

type chatTurnInput struct {
	conversationInput
	Message string `json:"message"`
}

type openClawTurnInput struct {
	OperationID string `json:"operation_id"`
	SessionKey  string `json:"session_key"`
}

type prepareOpenClawTurnInput struct {
	openClawTurnInput
	Message string `json:"message"`
}

type completeOpenClawTurnInput struct {
	openClawTurnInput
	Answer string `json:"answer"`
	Model  string `json:"model"`
}

type failOpenClawTurnInput struct {
	openClawTurnInput
	FailureCode    string `json:"failure_code"`
	FailureMessage string `json:"failure_message"`
}

type recordOpenClawToolResultInput struct {
	openClawTurnInput
	RunID      string `json:"run_id"`
	ToolName   string `json:"tool_name"`
	ToolCallID string `json:"tool_call_id"`
	Content    string `json:"content"`
}

type confirmMemoryInput struct {
	conversationInput
	ObservationID string `json:"observation_id"`
}

type reviewMemoryCandidateInput struct {
	conversationInput
	CandidateMemoryID string `json:"candidate_memory_id"`
}

type correctMemoryInput struct {
	conversationInput
	MemoryID string `json:"memory_id"`
	Content  string `json:"content"`
}

type forgetMemoryInput struct {
	conversationInput
	MemoryID string `json:"memory_id"`
}

type setDefaultInput struct {
	OperationID string `json:"operation_id"`
	Key         string `json:"key"`
	Content     string `json:"content"`
}

type correctDefaultInput struct {
	OperationID string `json:"operation_id"`
	MemoryID    string `json:"memory_id"`
	Content     string `json:"content"`
}

type forgetDefaultInput struct {
	OperationID string `json:"operation_id"`
	MemoryID    string `json:"memory_id"`
}

type promoteBridgeInput struct {
	OperationID    string   `json:"operation_id"`
	SourceChannel  string   `json:"source_channel"`
	SourceThreadID string   `json:"source_thread_id"`
	TargetRepoRoot string   `json:"target_repo_root"`
	MemoryIDs      []string `json:"memory_ids"`
}

type linkBridgeInput struct {
	OperationID     string `json:"operation_id"`
	PrimaryChannel  string `json:"primary_channel"`
	PrimaryThreadID string `json:"primary_thread_id"`
	LinkedChannel   string `json:"linked_channel"`
	LinkedThreadID  string `json:"linked_thread_id"`
}

type exportBridgeInput struct {
	OperationID   string   `json:"operation_id"`
	RepoRoot      string   `json:"repo_root"`
	MemoryIDs     []string `json:"memory_ids"`
	Title         string   `json:"title"`
	TargetProfile string   `json:"target_profile"`
}

type adoptBridgeInput struct {
	OperationID      string `json:"operation_id"`
	ExistingRepoRoot string `json:"existing_repo_root"`
	NewRepoRoot      string `json:"new_repo_root"`
}

type rebindBridgeInput struct {
	OperationID string `json:"operation_id"`
	OldRepoRoot string `json:"old_repo_root"`
	NewRepoRoot string `json:"new_repo_root"`
}

type reverseBridgeInput struct {
	OperationID string `json:"operation_id"`
	BridgeID    string `json:"bridge_id"`
}

func (h *Handler) chatTurn(response http.ResponseWriter, request *http.Request) {
	var input chatTurnInput
	if !decodeRequestJSON(response, request, &input) {
		return
	}
	receipt, err := h.service.Chat(request.Context(), runtime.ChatTurnRequest{
		OperationID: input.OperationID,
		Anchor:      runtime.ConversationAnchor{Channel: input.Channel, ThreadID: input.ThreadID},
		Message:     input.Message,
	})
	if err != nil {
		writeServiceError(response, err)
		return
	}
	status := http.StatusOK
	if receipt.Status == runtime.ChatTurnFailed {
		status = http.StatusBadGateway
	}
	writeJSON(response, status, receipt)
}

func (h *Handler) prepareOpenClawTurn(response http.ResponseWriter, request *http.Request) {
	h.prepareExternalTurn(response, request, "openclaw")
}

func (h *Handler) prepareHermesTurn(response http.ResponseWriter, request *http.Request) {
	h.prepareExternalTurn(response, request, "hermes")
}

func (h *Handler) prepareExternalTurn(response http.ResponseWriter, request *http.Request, channel string) {
	var input prepareOpenClawTurnInput
	if !decodeRequestJSON(response, request, &input) {
		return
	}
	receipt, err := h.service.PrepareExternalTurn(request.Context(), runtime.ExternalConversationTurnRequest{
		OperationID: input.OperationID,
		Anchor:      runtime.ConversationAnchor{Channel: channel, ThreadID: input.SessionKey},
		Message:     input.Message,
	})
	if err != nil {
		writeServiceError(response, err)
		return
	}
	status := http.StatusOK
	if receipt.Status == runtime.ChatTurnFailed {
		status = http.StatusBadGateway
	}
	writeJSON(response, status, receipt)
}

func (h *Handler) completeOpenClawTurn(response http.ResponseWriter, request *http.Request) {
	h.completeExternalTurn(response, request, "openclaw")
}

func (h *Handler) completeHermesTurn(response http.ResponseWriter, request *http.Request) {
	h.completeExternalTurn(response, request, "hermes")
}

func (h *Handler) completeExternalTurn(response http.ResponseWriter, request *http.Request, channel string) {
	var input completeOpenClawTurnInput
	if !decodeRequestJSON(response, request, &input) {
		return
	}
	receipt, err := h.service.CompleteExternalTurn(request.Context(), runtime.CompleteExternalConversationTurnRequest{
		OperationID: input.OperationID,
		Anchor:      runtime.ConversationAnchor{Channel: channel, ThreadID: input.SessionKey},
		Answer:      input.Answer,
		Model:       input.Model,
	})
	if err != nil {
		writeServiceError(response, err)
		return
	}
	writeJSON(response, http.StatusOK, receipt)
}

func (h *Handler) failOpenClawTurn(response http.ResponseWriter, request *http.Request) {
	h.failExternalTurn(response, request, "openclaw")
}

func (h *Handler) recordOpenClawToolResult(response http.ResponseWriter, request *http.Request) {
	var input recordOpenClawToolResultInput
	if !decodeRequestJSON(response, request, &input) {
		return
	}
	receipt, err := h.service.RecordToolResult(request.Context(), runtime.RecordConversationToolResultRequest{
		OperationID: input.OperationID,
		Anchor:      runtime.ConversationAnchor{Channel: "openclaw", ThreadID: input.SessionKey},
		RunID:       input.RunID,
		ToolName:    input.ToolName,
		ToolCallID:  input.ToolCallID,
		Content:     input.Content,
	})
	if err != nil {
		writeServiceError(response, err)
		return
	}
	writeJSON(response, http.StatusOK, receipt)
}

func (h *Handler) failHermesTurn(response http.ResponseWriter, request *http.Request) {
	h.failExternalTurn(response, request, "hermes")
}

func (h *Handler) failExternalTurn(response http.ResponseWriter, request *http.Request, channel string) {
	var input failOpenClawTurnInput
	if !decodeRequestJSON(response, request, &input) {
		return
	}
	receipt, err := h.service.FailExternalTurn(request.Context(), runtime.FailExternalConversationTurnRequest{
		OperationID:    input.OperationID,
		Anchor:         runtime.ConversationAnchor{Channel: channel, ThreadID: input.SessionKey},
		FailureCode:    input.FailureCode,
		FailureMessage: input.FailureMessage,
	})
	if err != nil {
		writeServiceError(response, err)
		return
	}
	writeJSON(response, http.StatusOK, receipt)
}

func (h *Handler) confirmMemory(response http.ResponseWriter, request *http.Request) {
	var input confirmMemoryInput
	if !decodeRequestJSON(response, request, &input) {
		return
	}
	receipt, err := h.service.Confirm(request.Context(), runtime.ConfirmConversationMemoryRequest{
		OperationID:   input.OperationID,
		Anchor:        runtime.ConversationAnchor{Channel: input.Channel, ThreadID: input.ThreadID},
		ObservationID: input.ObservationID,
	})
	if err != nil {
		writeServiceError(response, err)
		return
	}
	writeJSON(response, http.StatusOK, receipt)
}

func (h *Handler) listMemoryCandidates(response http.ResponseWriter, request *http.Request) {
	inbox, err := h.service.ReviewCandidates(request.Context(), runtime.ConversationAnchor{
		Channel:  request.URL.Query().Get("channel"),
		ThreadID: request.URL.Query().Get("thread_id"),
	})
	if err != nil {
		writeServiceError(response, err)
		return
	}
	writeJSON(response, http.StatusOK, inbox)
}

func (h *Handler) acceptMemoryCandidate(response http.ResponseWriter, request *http.Request) {
	h.reviewMemoryCandidate(response, request, true)
}

func (h *Handler) rejectMemoryCandidate(response http.ResponseWriter, request *http.Request) {
	h.reviewMemoryCandidate(response, request, false)
}

func (h *Handler) reviewMemoryCandidate(response http.ResponseWriter, request *http.Request, accept bool) {
	var input reviewMemoryCandidateInput
	if !decodeRequestJSON(response, request, &input) {
		return
	}
	review := runtime.ReviewConversationCandidateRequest{
		OperationID: input.OperationID,
		Anchor:      runtime.ConversationAnchor{Channel: input.Channel, ThreadID: input.ThreadID},
		MemoryID:    input.CandidateMemoryID,
	}
	var receipt runtime.GovernedObservationReceipt
	var err error
	if accept {
		receipt, err = h.service.AcceptCandidate(request.Context(), review)
	} else {
		receipt, err = h.service.RejectCandidate(request.Context(), review)
	}
	if err != nil {
		writeServiceError(response, err)
		return
	}
	writeJSON(response, http.StatusOK, receipt)
}

func (h *Handler) correctMemory(response http.ResponseWriter, request *http.Request) {
	var input correctMemoryInput
	if !decodeRequestJSON(response, request, &input) {
		return
	}
	receipt, err := h.service.Correct(request.Context(), runtime.CorrectConversationMemoryRequest{
		OperationID: input.OperationID,
		Anchor:      runtime.ConversationAnchor{Channel: input.Channel, ThreadID: input.ThreadID},
		MemoryID:    input.MemoryID,
		Content:     input.Content,
	})
	if err != nil {
		writeServiceError(response, err)
		return
	}
	writeJSON(response, http.StatusOK, receipt)
}

func (h *Handler) forgetMemory(response http.ResponseWriter, request *http.Request) {
	var input forgetMemoryInput
	if !decodeRequestJSON(response, request, &input) {
		return
	}
	receipt, err := h.service.Forget(request.Context(), runtime.ForgetConversationMemoryRequest{
		OperationID: input.OperationID,
		Anchor:      runtime.ConversationAnchor{Channel: input.Channel, ThreadID: input.ThreadID},
		MemoryID:    input.MemoryID,
	})
	if err != nil {
		writeServiceError(response, err)
		return
	}
	writeJSON(response, http.StatusOK, receipt)
}

func (h *Handler) inspectConversation(response http.ResponseWriter, request *http.Request) {
	inspection, err := h.service.Inspect(request.Context(), runtime.ConversationAnchor{
		Channel:  request.URL.Query().Get("channel"),
		ThreadID: request.URL.Query().Get("thread_id"),
	})
	if err != nil {
		writeServiceError(response, err)
		return
	}
	writeJSON(response, http.StatusOK, inspection)
}

func (h *Handler) inspectDefaults(response http.ResponseWriter, request *http.Request) {
	if !h.requireDefaults(response) {
		return
	}
	inspection, err := h.defaults.Inspect(request.Context())
	if err != nil {
		writeServiceError(response, err)
		return
	}
	writeJSON(response, http.StatusOK, inspection)
}

func (h *Handler) setDefault(response http.ResponseWriter, request *http.Request) {
	if !h.requireDefaults(response) {
		return
	}
	var input setDefaultInput
	if !decodeRequestJSON(response, request, &input) {
		return
	}
	receipt, err := h.defaults.Set(request.Context(), runtime.SetGlobalDefaultRequest{
		OperationID: input.OperationID,
		Key:         input.Key,
		Content:     input.Content,
	})
	if err != nil {
		writeServiceError(response, err)
		return
	}
	writeJSON(response, http.StatusOK, receipt)
}

func (h *Handler) correctDefault(response http.ResponseWriter, request *http.Request) {
	if !h.requireDefaults(response) {
		return
	}
	var input correctDefaultInput
	if !decodeRequestJSON(response, request, &input) {
		return
	}
	receipt, err := h.defaults.Correct(request.Context(), runtime.CorrectGlobalDefaultRequest{
		OperationID: input.OperationID,
		MemoryID:    input.MemoryID,
		Content:     input.Content,
	})
	if err != nil {
		writeServiceError(response, err)
		return
	}
	writeJSON(response, http.StatusOK, receipt)
}

func (h *Handler) forgetDefault(response http.ResponseWriter, request *http.Request) {
	if !h.requireDefaults(response) {
		return
	}
	var input forgetDefaultInput
	if !decodeRequestJSON(response, request, &input) {
		return
	}
	receipt, err := h.defaults.Forget(request.Context(), runtime.ForgetGlobalDefaultRequest{
		OperationID: input.OperationID,
		MemoryID:    input.MemoryID,
	})
	if err != nil {
		writeServiceError(response, err)
		return
	}
	writeJSON(response, http.StatusOK, receipt)
}

func (h *Handler) requireDefaults(response http.ResponseWriter) bool {
	if h.defaults != nil {
		return true
	}
	writeError(response, http.StatusInternalServerError, "internal_error", "request could not be completed")
	return false
}

func (h *Handler) promoteBridge(response http.ResponseWriter, request *http.Request) {
	if !h.requireBridges(response) {
		return
	}
	var input promoteBridgeInput
	if !decodeRequestJSON(response, request, &input) {
		return
	}
	receipt, err := h.bridges.PromoteConversationToWorkspace(request.Context(), runtime.PromoteConversationToWorkspaceRequest{
		OperationID: input.OperationID,
		Source: runtime.ConversationAnchor{
			Channel:  input.SourceChannel,
			ThreadID: input.SourceThreadID,
		},
		TargetRepoRoot: input.TargetRepoRoot,
		MemoryIDs:      input.MemoryIDs,
	})
	if err != nil {
		writeServiceError(response, err)
		return
	}
	writeJSON(response, http.StatusOK, receipt)
}

func (h *Handler) linkBridge(response http.ResponseWriter, request *http.Request) {
	if !h.requireBridges(response) {
		return
	}
	var input linkBridgeInput
	if !decodeRequestJSON(response, request, &input) {
		return
	}
	receipt, err := h.bridges.LinkConversations(request.Context(), runtime.LinkConversationsRequest{
		OperationID: input.OperationID,
		Primary: runtime.ConversationAnchor{
			Channel:  input.PrimaryChannel,
			ThreadID: input.PrimaryThreadID,
		},
		Linked: runtime.ConversationAnchor{
			Channel:  input.LinkedChannel,
			ThreadID: input.LinkedThreadID,
		},
	})
	if err != nil {
		writeServiceError(response, err)
		return
	}
	writeJSON(response, http.StatusOK, receipt)
}

func (h *Handler) exportBridge(response http.ResponseWriter, request *http.Request) {
	if !h.requireBridges(response) {
		return
	}
	var input exportBridgeInput
	if !decodeRequestJSON(response, request, &input) {
		return
	}
	receipt, err := h.bridges.ExportWorkspace(request.Context(), runtime.ExportWorkspaceRequest{
		OperationID:   input.OperationID,
		RepoRoot:      input.RepoRoot,
		MemoryIDs:     input.MemoryIDs,
		Title:         input.Title,
		TargetProfile: input.TargetProfile,
	})
	if err != nil {
		writeServiceError(response, err)
		return
	}
	writeJSON(response, http.StatusOK, receipt)
}

func (h *Handler) adoptBridge(response http.ResponseWriter, request *http.Request) {
	if !h.requireBridges(response) {
		return
	}
	var input adoptBridgeInput
	if !decodeRequestJSON(response, request, &input) {
		return
	}
	receipt, err := h.bridges.AdoptWorkspaceAnchor(request.Context(), runtime.AdoptWorkspaceAnchorRequest{
		OperationID:      input.OperationID,
		ExistingRepoRoot: input.ExistingRepoRoot,
		NewRepoRoot:      input.NewRepoRoot,
	})
	if err != nil {
		writeServiceError(response, err)
		return
	}
	writeJSON(response, http.StatusOK, receipt)
}

func (h *Handler) rebindBridge(response http.ResponseWriter, request *http.Request) {
	if !h.requireBridges(response) {
		return
	}
	var input rebindBridgeInput
	if !decodeRequestJSON(response, request, &input) {
		return
	}
	receipt, err := h.bridges.RebindWorkspace(request.Context(), runtime.RebindWorkspaceRequest{
		OperationID: input.OperationID,
		OldRepoRoot: input.OldRepoRoot,
		NewRepoRoot: input.NewRepoRoot,
	})
	if err != nil {
		writeServiceError(response, err)
		return
	}
	writeJSON(response, http.StatusOK, receipt)
}

func (h *Handler) reverseBridge(response http.ResponseWriter, request *http.Request) {
	if !h.requireBridges(response) {
		return
	}
	var input reverseBridgeInput
	if !decodeRequestJSON(response, request, &input) {
		return
	}
	receipt, err := h.bridges.Reverse(request.Context(), runtime.ReverseBridgeRequest{
		OperationID: input.OperationID,
		BridgeID:    input.BridgeID,
	})
	if err != nil {
		writeServiceError(response, err)
		return
	}
	writeJSON(response, http.StatusOK, receipt)
}

func (h *Handler) inspectBridge(response http.ResponseWriter, request *http.Request) {
	if !h.requireBridges(response) {
		return
	}
	receipt, err := h.bridges.Inspect(request.Context(), request.PathValue("bridge_id"))
	if err != nil {
		writeServiceError(response, err)
		return
	}
	writeJSON(response, http.StatusOK, receipt)
}

func (h *Handler) requireBridges(response http.ResponseWriter) bool {
	if h.bridges != nil {
		return true
	}
	writeError(response, http.StatusInternalServerError, "internal_error", "request could not be completed")
	return false
}

func decodeRequestJSON(response http.ResponseWriter, request *http.Request, target any) bool {
	if mediaType := strings.TrimSpace(strings.Split(request.Header.Get("Content-Type"), ";")[0]); mediaType != "application/json" {
		writeError(response, http.StatusUnsupportedMediaType, "unsupported_media_type", "Content-Type must be application/json")
		return false
	}
	request.Body = http.MaxBytesReader(response, request.Body, maxRequestBodyBytes)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeError(response, http.StatusRequestEntityTooLarge, "request_too_large", "request body is too large")
			return false
		}
		writeError(response, http.StatusBadRequest, "invalid_json", "request body must contain one valid JSON object")
		return false
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeError(response, http.StatusBadRequest, "invalid_json", "request body must contain exactly one JSON object")
		return false
	}
	return true
}

func writeServiceError(response http.ResponseWriter, err error) {
	message := err.Error()
	switch {
	case strings.Contains(message, "does not exist"):
		writeError(response, http.StatusNotFound, "not_found", "resource does not exist")
	case isSafeClientError(message):
		writeError(response, http.StatusBadRequest, "invalid_request", message)
	default:
		writeError(response, http.StatusInternalServerError, "internal_error", "request could not be completed")
	}
}

func isSafeClientError(message string) bool {
	for _, fragment := range []string{
		" is required",
		" is too long",
		" does not belong",
		" cannot become memory",
		"must be an active fact",
		"already bound",
		"already has an active value",
		"key must use",
		"requires confirmation",
		"already confirmed",
		"already completed",
		"already failed",
		"already linked",
		"has no prepared delivery",
		"must be active",
		"must be different",
		"duplicate item",
		"is unsupported",
		"contains sensitive data",
		"too many tool results",
		"total content is too long",
		"does not match the prepared operation",
	} {
		if strings.Contains(message, fragment) {
			return true
		}
	}
	return false
}

func writeError(response http.ResponseWriter, status int, code, message string) {
	writeJSON(response, status, map[string]any{
		"error": map[string]string{
			"code":    code,
			"message": message,
		},
	})
}

func writeJSON(response http.ResponseWriter, status int, value any) {
	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(status)
	if err := json.NewEncoder(response).Encode(value); err != nil {
		_, _ = fmt.Fprintln(response, `{"error":{"code":"encoding_error","message":"response could not be encoded"}}`)
	}
}
