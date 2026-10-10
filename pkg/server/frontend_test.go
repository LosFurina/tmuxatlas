package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func testFrontendHandler() http.Handler {
	return frontendHandler(fstest.MapFS{
		"index.html": {
			Data: []byte("<!doctype html><title>TmuxAtlas test shell</title>"),
		},
		"manifest.json": {
			Data: []byte(`{"name":"TmuxAtlas"}`),
		},
		"sw.js": {
			Data: []byte("self.addEventListener('push', () => {})"),
		},
		"icon-192.png": {
			Data: []byte("\x89PNG\r\n\x1a\n"),
		},
	})
}

func TestFrontendHandlerServesPWAResources(t *testing.T) {
	t.Parallel()

	tests := []struct {
		path               string
		contentType        string
		cacheControl       string
		serviceWorkerScope string
		body               string
	}{
		{
			path:         "/",
			cacheControl: "no-cache",
			body:         "<title>TmuxAtlas test shell</title>",
		},
		{
			path:         "/manifest.json",
			contentType:  "application/manifest+json",
			cacheControl: "no-cache",
			body:         `{"name":"TmuxAtlas"}`,
		},
		{
			path:               "/sw.js",
			contentType:        "text/javascript",
			cacheControl:       "no-cache",
			serviceWorkerScope: "/",
			body:               "self.addEventListener('push', () => {})",
		},
		{
			path:        "/icon-192.png",
			contentType: "image/png",
		},
	}

	handler := testFrontendHandler()
	for _, test := range tests {
		t.Run(test.path, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, test.path, nil)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)

			if response.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
			}
			if contentType := response.Header().Get("Content-Type"); !strings.HasPrefix(contentType, test.contentType) {
				t.Fatalf("Content-Type = %q, want prefix %q", contentType, test.contentType)
			}
			if cacheControl := response.Header().Get("Cache-Control"); cacheControl != test.cacheControl {
				t.Fatalf("Cache-Control = %q, want %q", cacheControl, test.cacheControl)
			}
			if scope := response.Header().Get("Service-Worker-Allowed"); scope != test.serviceWorkerScope {
				t.Fatalf("Service-Worker-Allowed = %q, want %q", scope, test.serviceWorkerScope)
			}
			if test.body != "" && !strings.Contains(response.Body.String(), test.body) {
				t.Fatalf("body = %q, want substring %q", response.Body.String(), test.body)
			}
		})
	}
}

func TestFrontendHandlerReturnsNotFoundForMissingAssets(t *testing.T) {
	t.Parallel()

	handler := testFrontendHandler()
	for _, requestPath := range []string{
		"/missing.js",
		"/missing.json",
		"/missing.png",
		"/missing.css",
		"/assets/missing.svg",
	} {
		t.Run(requestPath, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, requestPath, nil)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)

			if response.Code != http.StatusNotFound {
				t.Fatalf("status = %d, want %d", response.Code, http.StatusNotFound)
			}
			body, err := io.ReadAll(response.Result().Body)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(body), "TmuxAtlas test shell") {
				t.Fatal("missing asset returned the SPA document")
			}
		})
	}
}

func TestFrontendHandlerServesSPARoutes(t *testing.T) {
	t.Parallel()

	handler := testFrontendHandler()
	for _, requestPath := range []string{
		"/",
		"/settings",
		"/setup",
		"/session/bash",
		"/session/project.v2",
		"/session/peer-id/project.v2",
	} {
		t.Run(requestPath, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, requestPath, nil)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)

			if response.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
			}
			if !strings.Contains(response.Body.String(), "TmuxAtlas test shell") {
				t.Fatalf("body did not contain SPA document: %q", response.Body.String())
			}
		})
	}
}
