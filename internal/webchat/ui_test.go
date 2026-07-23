package webchat

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"vermory/internal/provider"
)

func TestBrowserAppServesSameOriginAssetsWithRestrictiveHeaders(t *testing.T) {
	handler, _ := testHandler(t, provider.Mock{Output: "unused"})
	tests := []struct {
		path        string
		contentType string
		contains    []string
	}{
		{
			path:        "/",
			contentType: "text/html; charset=utf-8",
			contains:    []string{"<title>Vermory</title>", `rel="icon"`, `src="/assets/app.js"`, `href="/assets/app.css"`, `id="transcript"`, `id="memory-content"`, `id="auth-gate"`, `id="api-token"`, `id="sign-out"`},
		},
		{
			path:        "/assets/app.css",
			contentType: "text/css; charset=utf-8",
			contains:    []string{".workspace", "[data-mobile-view=", "@media (max-width: 820px)"},
		},
		{
			path:        "/assets/app.js",
			contentType: "text/javascript; charset=utf-8",
			contains:    []string{storageKeyForTest, "operationID", "persistState();", `fetch("/v1/browser/runtime"`, `requestJSON("/v1/chat/turn"`, `data-action="save-correction"`},
		},
		{
			path:        "/v1/browser/runtime",
			contentType: "application/json",
			contains:    []string{`{"mode":"local"}`},
		},
	}

	for _, test := range tests {
		t.Run(test.path, func(t *testing.T) {
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, test.path, nil))
			if response.Code != http.StatusOK {
				t.Fatalf("GET %s returned %d: %s", test.path, response.Code, response.Body.String())
			}
			if got := response.Header().Get("Content-Type"); got != test.contentType {
				t.Fatalf("GET %s Content-Type = %q, want %q", test.path, got, test.contentType)
			}
			for _, header := range []string{"Content-Security-Policy", "Referrer-Policy", "X-Content-Type-Options"} {
				if response.Header().Get(header) == "" {
					t.Fatalf("GET %s omitted %s", test.path, header)
				}
			}
			for _, expected := range test.contains {
				if !strings.Contains(response.Body.String(), expected) {
					t.Errorf("GET %s omitted %q", test.path, expected)
				}
			}
		})
	}
}

func TestBrowserAppDoesNotUseInlineExecutableContentOrCrossOriginDependencies(t *testing.T) {
	handler, _ := testHandler(t, provider.Mock{Output: "unused"})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", nil))
	body := response.Body.String()
	for _, forbidden := range []string{"<script>", "onclick=", "onload=", "http://", "https://"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("browser app contains forbidden inline or cross-origin content %q", forbidden)
		}
	}
	policy := response.Header().Get("Content-Security-Policy")
	for _, directive := range []string{"default-src 'self'", "connect-src 'self'", "script-src 'self'", "style-src 'self'", "object-src 'none'", "frame-ancestors 'none'"} {
		if !strings.Contains(policy, directive) {
			t.Errorf("CSP omitted %q: %s", directive, policy)
		}
	}
}

func TestBrowserAppAssetsRejectUnsupportedMethods(t *testing.T) {
	handler, _ := testHandler(t, provider.Mock{Output: "unused"})
	for _, path := range []string{"/", "/assets/app.css", "/assets/app.js", "/v1/browser/runtime"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, path, nil))
		if response.Code != http.StatusMethodNotAllowed {
			t.Errorf("POST %s returned %d, want %d", path, response.Code, http.StatusMethodNotAllowed)
		}
	}
}

func TestBrowserAppUsesDatasetNamesThatMatchDataAttributes(t *testing.T) {
	script := string(browserAppJS)
	for _, expected := range []string{"dataset.threadId", "dataset.messageId", "dataset.observationId", "dataset.memoryId", "dataset.candidateId"} {
		if !strings.Contains(script, expected) {
			t.Errorf("browser app omitted canonical dataset property %q", expected)
		}
	}
	for _, forbidden := range []string{"dataset.threadID", "dataset.messageID", "dataset.observationID", "dataset.memoryID", "dataset.candidateID"} {
		if strings.Contains(script, forbidden) {
			t.Errorf("browser app uses dataset property %q, which does not map to a *-id attribute", forbidden)
		}
	}
}

func TestBrowserAppScopesAsyncSendUpdatesToOriginatingThread(t *testing.T) {
	script := string(browserAppJS)
	if !strings.Contains(script, "const threadID = thread.id;") {
		t.Fatal("browser app does not capture the originating thread before sending")
	}
	if count := strings.Count(script, "activeThread().id === threadID"); count != 2 {
		t.Fatalf("browser app has %d originating-thread guards, want 2", count)
	}
}

func TestBrowserAppRejectsStaleRefreshesAfterThreadSwitch(t *testing.T) {
	script := string(browserAppJS)
	for _, expected := range []string{
		"let refreshSequence = 0;",
		"const refreshID = ++refreshSequence;",
		"state.activeThreadID === threadID && refreshID === refreshSequence",
		"conversationQuery(threadID)",
	} {
		if !strings.Contains(script, expected) {
			t.Errorf("browser app omitted stale-refresh guard %q", expected)
		}
	}
	if count := strings.Count(script, "if (!refreshStillCurrent()) return;"); count != 2 {
		t.Fatalf("browser app has %d stale-refresh exits, want 2", count)
	}
}

func TestBrowserAppKeepsAuthenticatedCredentialEphemeral(t *testing.T) {
	script := string(browserAppJS)
	for _, expected := range []string{
		`let apiCredential = "";`,
		`fetch("/v1/browser/runtime"`,
		`Authorization: `,
		`Bearer ${apiCredential}`,
		`requestJSON("/v1/session"`,
		`apiCredential = "";`,
	} {
		if !strings.Contains(script, expected) {
			t.Errorf("browser app omitted ephemeral authentication behavior %q", expected)
		}
	}
	for _, forbidden := range []string{
		"sessionStorage",
		"localStorage.setItem(credential",
		"localStorage.setItem(token",
		"api_token=",
		"access_token=",
		"?token=",
	} {
		if strings.Contains(script, forbidden) {
			t.Errorf("browser app contains credential persistence or URL transport %q", forbidden)
		}
	}
}

const storageKeyForTest = "vermory.webchat.v1"

func TestW32BrowserContractManifestRemainsFrozen(t *testing.T) {
	payload, err := os.ReadFile("../../runtime/cases/W32-browser-webchat-lifecycle/case.json")
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Version       int      `json:"version"`
		ID            string   `json:"id"`
		Surface       string   `json:"surface"`
		HardGateCount int      `json:"hard_gate_count"`
		HardGates     []string `json:"hard_gates"`
		NonClaims     []string `json:"non_claims"`
	}
	if err := json.Unmarshal(payload, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Version != 1 || manifest.ID != "W32-browser-webchat-lifecycle" || manifest.Surface != "real_chrome_same_origin_web_chat" {
		t.Fatalf("unexpected W32 identity: %#v", manifest)
	}
	if manifest.HardGateCount != 20 || len(manifest.HardGates) != manifest.HardGateCount {
		t.Fatalf("unexpected W32 hard gates: count=%d gates=%d", manifest.HardGateCount, len(manifest.HardGates))
	}
	if len(manifest.NonClaims) != 3 {
		t.Fatalf("unexpected W32 non-claim count: %d", len(manifest.NonClaims))
	}
}

func TestW35AuthenticatedRemoteBrowserContractManifestRemainsFrozen(t *testing.T) {
	payload, err := os.ReadFile("../../runtime/cases/W35-authenticated-remote-webchat/case.json")
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Version       int      `json:"version"`
		ID            string   `json:"id"`
		Surface       string   `json:"surface"`
		HardGateCount int      `json:"hard_gate_count"`
		HardGates     []string `json:"hard_gates"`
		NonClaims     []string `json:"non_claims"`
	}
	if err := json.Unmarshal(payload, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Version != 1 || manifest.ID != "W35-authenticated-remote-webchat" || manifest.Surface != "real_remote_chrome_https_multi_tenant_web_chat" {
		t.Fatalf("unexpected W35 identity: %#v", manifest)
	}
	if manifest.HardGateCount != 26 || len(manifest.HardGates) != manifest.HardGateCount {
		t.Fatalf("unexpected W35 hard gates: count=%d gates=%d", manifest.HardGateCount, len(manifest.HardGates))
	}
	if len(manifest.NonClaims) != 4 {
		t.Fatalf("unexpected W35 non-claim count: %d", len(manifest.NonClaims))
	}
}

func TestW38OfficialOpenClawWebChatContractManifestRemainsFrozen(t *testing.T) {
	payload, err := os.ReadFile("../../runtime/cases/W38-official-openclaw-webchat/case.json")
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Version              int      `json:"version"`
		ID                   string   `json:"id"`
		Surface              string   `json:"surface"`
		OpenClawVersion      string   `json:"openclaw_version"`
		ConversationProvider string   `json:"conversation_provider"`
		HardGateCount        int      `json:"hard_gate_count"`
		HardGates            []string `json:"hard_gates"`
		NonClaims            []string `json:"non_claims"`
	}
	if err := json.Unmarshal(payload, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Version != 1 || manifest.ID != "W38-official-openclaw-webchat" ||
		manifest.Surface != "real_chrome_official_openclaw_control_ui_gateway_websocket" {
		t.Fatalf("unexpected W38 identity: %#v", manifest)
	}
	if manifest.OpenClawVersion != "2026.6.11" || manifest.ConversationProvider != "real_grok_cli_stateless" {
		t.Fatalf("unexpected W38 client boundary: %#v", manifest)
	}
	if manifest.HardGateCount != 28 || len(manifest.HardGates) != manifest.HardGateCount {
		t.Fatalf("unexpected W38 hard gates: count=%d gates=%d", manifest.HardGateCount, len(manifest.HardGates))
	}
	if len(manifest.NonClaims) != 6 {
		t.Fatalf("unexpected W38 non-claim count: %d", len(manifest.NonClaims))
	}
}
