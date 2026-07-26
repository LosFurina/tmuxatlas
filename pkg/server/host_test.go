package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHostMiddlewareIgnoresForwardedHost(t *testing.T) {
	middleware, err := hostMiddleware("https://tmuxatlas.example")
	if err != nil {
		t.Fatal(err)
	}
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	request := httptest.NewRequest(http.MethodGet, "https://evil.example/", nil)
	request.Host = "evil.example"
	request.Header.Set("X-Forwarded-Host", "tmuxatlas.example")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", recorder.Code)
	}
}

func TestHostMiddlewareAcceptsConfiguredDefaultPort(t *testing.T) {
	middleware, _ := hostMiddleware("https://tmuxatlas.example")
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	request := httptest.NewRequest(http.MethodGet, "https://tmuxatlas.example/", nil)
	request.Host = "tmuxatlas.example:443"
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", recorder.Code)
	}
}
