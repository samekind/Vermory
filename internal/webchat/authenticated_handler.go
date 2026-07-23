package webchat

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"unicode"

	"vermory/internal/authn"
	"vermory/internal/provider"
	"vermory/internal/runtime"
)

type authenticatedHandler struct {
	store         *runtime.Store
	provider      provider.Provider
	model         string
	authenticator authn.Authenticator
	retriever     runtime.MemoryRetriever
	browser       http.Handler
}

type routeAccess int

const (
	routeUnknown routeAccess = iota
	routeClient
	routeOperator
)

func NewAuthenticatedHandler(store *runtime.Store, llm provider.Provider, model string, authenticator authn.Authenticator) http.Handler {
	return &authenticatedHandler{store: store, provider: llm, model: model, authenticator: authenticator, browser: newBrowserAppHandler("authenticated")}
}

func NewAuthenticatedHandlerWithRetriever(store *runtime.Store, llm provider.Provider, model string, authenticator authn.Authenticator, retriever runtime.MemoryRetriever) http.Handler {
	return &authenticatedHandler{store: store, provider: llm, model: model, authenticator: authenticator, retriever: retriever, browser: newBrowserAppHandler("authenticated")}
}

func (handler *authenticatedHandler) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	if authenticatedBrowserPath(request.URL.Path) {
		handler.browser.ServeHTTP(response, request)
		return
	}
	raw, ok := bearerCredential(request)
	if !ok || handler.authenticator == nil || handler.store == nil {
		writeError(response, http.StatusUnauthorized, "unauthorized", "authentication is required")
		return
	}
	principal, err := handler.authenticator.Authenticate(request.Context(), raw)
	if err != nil {
		if errors.Is(err, authn.ErrAuthenticationFailed) || errors.Is(err, authn.ErrInvalidToken) {
			writeError(response, http.StatusUnauthorized, "unauthorized", "authentication failed")
			return
		}
		writeError(response, http.StatusServiceUnavailable, "authentication_unavailable", "authentication is unavailable")
		return
	}
	if strings.TrimSpace(principal.TenantID) == "" || !principal.Role.Valid() {
		writeError(response, http.StatusUnauthorized, "unauthorized", "authentication failed")
		return
	}
	access := authenticatedRouteAccess(request.Method, request.URL.Path)
	if access == routeUnknown {
		writeError(response, http.StatusNotFound, "not_found", "resource does not exist")
		return
	}
	if access == routeOperator && principal.Role == authn.RoleClient {
		writeError(response, http.StatusForbidden, "forbidden", "this identity cannot perform that operation")
		return
	}
	if request.Method == http.MethodGet && request.URL.Path == "/v1/session" {
		response.Header().Set("Cache-Control", "no-store")
		writeJSON(response, http.StatusOK, map[string]string{
			"role":          principal.Role.String(),
			"storage_scope": browserStorageScope(principal.TenantID),
		})
		return
	}

	service := runtime.NewConversationService(handler.store, principal.TenantID, handler.provider, handler.model, runtime.ConversationServiceConfig{Retriever: handler.retriever})
	defaults := runtime.NewGlobalDefaultsService(handler.store, principal.TenantID)
	bridges := runtime.NewBridgeService(handler.store, principal.TenantID)
	buffered := newBufferedResponse()
	NewHandlerWithGovernance(service, defaults, bridges).ServeHTTP(buffered, request)
	buffered.flushSafe(response)
}

func bearerCredential(request *http.Request) (string, bool) {
	values := request.Header.Values("Authorization")
	if len(values) != 1 {
		return "", false
	}
	value := values[0]
	if value != strings.TrimSpace(value) || strings.Contains(value, ",") {
		return "", false
	}
	scheme, credential, found := strings.Cut(value, " ")
	if !found || !strings.EqualFold(scheme, "Bearer") || credential == "" {
		return "", false
	}
	if strings.IndexFunc(credential, func(value rune) bool {
		return unicode.IsSpace(value) || unicode.IsControl(value)
	}) >= 0 {
		return "", false
	}
	return credential, true
}

func authenticatedRouteAccess(method, path string) routeAccess {
	clientRoutes := map[string]struct{}{
		"GET /v1/session":                                   {},
		"POST /v1/chat/turn":                                {},
		"GET /v1/conversations/inspect":                     {},
		"POST /v1/integrations/openclaw/turns/prepare":      {},
		"POST /v1/integrations/openclaw/turns/complete":     {},
		"POST /v1/integrations/openclaw/turns/fail":         {},
		"POST /v1/integrations/openclaw/turns/tool-results": {},
		"POST /v1/integrations/hermes/turns/prepare":        {},
		"POST /v1/integrations/hermes/turns/complete":       {},
		"POST /v1/integrations/hermes/turns/fail":           {},
		"POST /v1/client-operations/prepare":                {},
		"POST /v1/client-operations/reclaim":                {},
		"POST /v1/client-operations/heartbeat":              {},
		"POST /v1/client-operations/checkpoint":             {},
		"POST /v1/client-operations/complete":               {},
		"POST /v1/client-operations/fail":                   {},
		"POST /v1/client-operations/cancel":                 {},
	}
	operatorRoutes := map[string]struct{}{
		"POST /v1/memories/confirm":           {},
		"GET /v1/memories/candidates":         {},
		"POST /v1/memories/candidates/accept": {},
		"POST /v1/memories/candidates/reject": {},
		"POST /v1/memories/correct":           {},
		"POST /v1/memories/forget":            {},
		"GET /v1/defaults":                    {},
		"POST /v1/defaults/set":               {},
		"POST /v1/defaults/correct":           {},
		"POST /v1/defaults/forget":            {},
		"POST /v1/bridges/promote":            {},
		"POST /v1/bridges/link":               {},
		"POST /v1/bridges/export":             {},
		"POST /v1/bridges/adopt":              {},
		"POST /v1/bridges/rebind":             {},
		"POST /v1/bridges/reverse":            {},
	}
	key := method + " " + path
	if _, ok := clientRoutes[key]; ok {
		return routeClient
	}
	if _, ok := operatorRoutes[key]; ok {
		return routeOperator
	}
	if method == http.MethodGet && strings.HasPrefix(path, "/v1/bridges/") && strings.TrimPrefix(path, "/v1/bridges/") != "" && !strings.Contains(strings.TrimPrefix(path, "/v1/bridges/"), "/") {
		return routeOperator
	}
	return routeUnknown
}

func authenticatedBrowserPath(path string) bool {
	switch path {
	case "/", "/assets/app.css", "/assets/app.js", "/v1/browser/runtime":
		return true
	default:
		return false
	}
}

func browserStorageScope(tenantID string) string {
	digest := sha256.Sum256([]byte("vermory-browser-storage-v1\x00" + strings.TrimSpace(tenantID)))
	return hex.EncodeToString(digest[:16])
}

type bufferedResponse struct {
	header http.Header
	status int
	body   bytes.Buffer
}

func newBufferedResponse() *bufferedResponse {
	return &bufferedResponse{header: make(http.Header)}
}

func (response *bufferedResponse) Header() http.Header {
	return response.header
}

func (response *bufferedResponse) WriteHeader(status int) {
	if response.status == 0 {
		response.status = status
	}
}

func (response *bufferedResponse) Write(data []byte) (int, error) {
	if response.status == 0 {
		response.status = http.StatusOK
	}
	return response.body.Write(data)
}

func (response *bufferedResponse) flushSafe(target http.ResponseWriter) {
	status := response.status
	if status == 0 {
		status = http.StatusOK
	}
	if status >= http.StatusBadRequest {
		code, message := safeHTTPError(status)
		writeError(target, status, code, message)
		return
	}
	for key, values := range response.header {
		for _, value := range values {
			target.Header().Add(key, value)
		}
	}
	target.WriteHeader(status)
	_, _ = target.Write(response.body.Bytes())
}

func safeHTTPError(status int) (string, string) {
	switch status {
	case http.StatusBadRequest:
		return "invalid_request", "request is invalid"
	case http.StatusNotFound:
		return "not_found", "resource does not exist"
	case http.StatusRequestEntityTooLarge:
		return "request_too_large", "request body is too large"
	case http.StatusUnsupportedMediaType:
		return "unsupported_media_type", "Content-Type must be application/json"
	case http.StatusBadGateway:
		return "upstream_failure", "upstream execution failed"
	default:
		return "request_failed", "request could not be completed"
	}
}
