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
	registerBrowserAppRoutes(h.mux, "local")
}

func newBrowserAppHandler(mode string) http.Handler {
	mux := http.NewServeMux()
	registerBrowserAppRoutes(mux, mode)
	return mux
}

func registerBrowserAppRoutes(mux *http.ServeMux, mode string) {
	mux.HandleFunc("GET /{$}", serveBrowserAsset("text/html; charset=utf-8", browserAppHTML))
	mux.HandleFunc("GET /assets/app.css", serveBrowserAsset("text/css; charset=utf-8", browserAppCSS))
	mux.HandleFunc("GET /assets/app.js", serveBrowserAsset("text/javascript; charset=utf-8", browserAppJS))
	mux.HandleFunc("GET /v1/browser/runtime", serveBrowserAsset("application/json", []byte("{\"mode\":\""+mode+"\"}\n")))
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
