package httpapi

import (
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path"
	"strings"
)

// WithFrontend serves a built single-page application alongside the API. API
// and health routes always keep their normal HTTP semantics; client-side
// routes fall back to index.html.
func WithFrontend(api http.Handler, directory string) (http.Handler, error) {
	root := os.DirFS(directory)
	if _, err := fs.Stat(root, "index.html"); err != nil {
		return nil, fmt.Errorf("open frontend index: %w", err)
	}

	files := http.FileServer(http.FS(root))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isBackendPath(r.URL.Path) || (r.Method != http.MethodGet && r.Method != http.MethodHead) {
			api.ServeHTTP(w, r)
			return
		}

		name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if name == "." || name == "" {
			name = "index.html"
		}
		info, err := fs.Stat(root, name)
		if err == nil && !info.IsDir() {
			setFrontendHeaders(w, name)
			files.ServeHTTP(w, r)
			return
		}
		if path.Ext(name) != "" {
			http.NotFound(w, r)
			return
		}

		request := r.Clone(r.Context())
		request.URL.Path = "/"
		setFrontendHeaders(w, "index.html")
		files.ServeHTTP(w, request)
	}), nil
}

func isBackendPath(requestPath string) bool {
	return requestPath == "/api" || strings.HasPrefix(requestPath, "/api/") ||
		requestPath == "/healthz" || requestPath == "/readyz"
}

func setFrontendHeaders(w http.ResponseWriter, name string) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Referrer-Policy", "same-origin")
	if name == "index.html" || name == "service-worker.js" || name == "manifest.webmanifest" {
		w.Header().Set("Cache-Control", "no-cache")
	}
}
