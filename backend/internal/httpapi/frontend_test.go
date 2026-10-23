package httpapi

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestWithFrontend(t *testing.T) {
	directory := t.TempDir()
	if err := os.Mkdir(filepath.Join(directory, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "index.html"), []byte("frontend"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "assets", "app.js"), []byte("asset"), 0o644); err != nil {
		t.Fatal(err)
	}

	api := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})
	handler, err := WithFrontend(api, directory)
	if err != nil {
		t.Fatalf("WithFrontend() error = %v", err)
	}

	tests := []struct {
		name       string
		method     string
		path       string
		wantStatus int
		wantBody   string
	}{
		{name: "root", method: http.MethodGet, path: "/", wantStatus: http.StatusOK, wantBody: "frontend"},
		{name: "client route", method: http.MethodGet, path: "/pos", wantStatus: http.StatusOK, wantBody: "frontend"},
		{name: "asset", method: http.MethodGet, path: "/assets/app.js", wantStatus: http.StatusOK, wantBody: "asset"},
		{name: "missing asset", method: http.MethodGet, path: "/assets/missing.js", wantStatus: http.StatusNotFound, wantBody: "404 page not found\n"},
		{name: "api", method: http.MethodGet, path: "/api/v1/me", wantStatus: http.StatusTeapot},
		{name: "health", method: http.MethodGet, path: "/healthz", wantStatus: http.StatusTeapot},
		{name: "unknown mutation", method: http.MethodPost, path: "/pos", wantStatus: http.StatusTeapot},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(test.method, test.path, nil)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d", response.Code, test.wantStatus)
			}
			if test.wantBody != "" && response.Body.String() != test.wantBody {
				t.Fatalf("body = %q, want %q", response.Body.String(), test.wantBody)
			}
		})
	}
}

func TestWithFrontendRejectsMissingBuild(t *testing.T) {
	if _, err := WithFrontend(http.NotFoundHandler(), t.TempDir()); err == nil {
		t.Fatal("WithFrontend() expected error")
	}
}
