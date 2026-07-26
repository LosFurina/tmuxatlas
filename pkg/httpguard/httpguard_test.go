package httpguard

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestJSONBodyRejectsInvalidInputsBeforeMutation(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		body        string
		limit       int64
		wantStatus  int
	}{
		{name: "wrong media type", contentType: "text/plain", body: `{}`, limit: 32, wantStatus: http.StatusUnsupportedMediaType},
		{name: "oversized", contentType: "application/json", body: `{"value":"too long"}`, limit: 8, wantStatus: http.StatusRequestEntityTooLarge},
		{name: "trailing value", contentType: "application/json", body: `{} {}`, limit: 32, wantStatus: http.StatusBadRequest},
		{name: "malformed", contentType: "application/json", body: `{`, limit: 32, wantStatus: http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mutated := false
			handler := JSONBody(tt.limit)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				mutated = true
			}))
			req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", tt.contentType)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if mutated {
				t.Fatal("mutation ran after rejected request")
			}
		})
	}
}

func TestJSONBodyAcceptsSingleJSONValue(t *testing.T) {
	called := false
	handler := JSONBody(32)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	}))
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"ok":true}`))
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent || !called {
		t.Fatalf("status = %d, called = %v", rec.Code, called)
	}
}
