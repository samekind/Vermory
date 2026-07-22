package webchat

import (
	_ "embed"
	"net/http"
)

//go:embed ui/index.html
var browserAppHTML []byte

//go:embed ui/app.css
var browserAppCSS []byte

//go:embed ui/app.js
var browserAppJS []byte

func (h *Handler) registerBrowserApp() {
	h.mux.HandleFunc("GET /{$}", serveBrowserAsset("text/html; charset=utf-8", browserAppHTML))
	h.mux.HandleFunc("GET /assets/app.css", serveBrowserAsset("text/css; charset=utf-8", browserAppCSS))
	h.mux.HandleFunc("GET /assets/app.js", serveBrowserAsset("text/javascript; charset=utf-8", browserAppJS))
}

func serveBrowserAsset(contentType string, body []byte) http.HandlerFunc {
	return func(response http.ResponseWriter, _ *http.Request) {
		response.Header().Set("Cache-Control", "no-cache")
		response.Header().Set("Content-Security-Policy", "default-src 'self'; connect-src 'self'; img-src 'self' data:; script-src 'self'; style-src 'self'; object-src 'none'; base-uri 'none'; form-action 'self'; frame-ancestors 'none'")
		response.Header().Set("Content-Type", contentType)
		response.Header().Set("Referrer-Policy", "no-referrer")
		response.Header().Set("X-Content-Type-Options", "nosniff")
		response.WriteHeader(http.StatusOK)
		_, _ = response.Write(body)
	}
}
