package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCORSAllowsCredentialedOrigin(t *testing.T) {
	t.Setenv("CORS_ALLOW_ORIGIN", "*")

	nextCalled := false
	handler := CORS(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		nextCalled = true
	}))
	req := httptest.NewRequest(http.MethodOptions, "/api/auth/google", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNoContent)
	}
	if got := recorder.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:3000" {
		t.Fatalf("allow-origin = %q, want requesting origin", got)
	}
	if got := recorder.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Fatalf("allow-credentials = %q, want true", got)
	}
	if recorder.Header().Get("Vary") != "Origin" {
		t.Fatalf("vary = %q, want Origin", recorder.Header().Get("Vary"))
	}
	if nextCalled {
		t.Fatal("OPTIONS request unexpectedly reached the next handler")
	}
}

func TestCORSRejectsUnconfiguredOrigin(t *testing.T) {
	t.Setenv("CORS_ALLOW_ORIGIN", "http://localhost:3000")

	handler := CORS(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	req := httptest.NewRequest(http.MethodGet, "/api/challenges", nil)
	req.Header.Set("Origin", "https://untrusted.example")
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, req)

	if got := recorder.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("allow-origin = %q, want no CORS permission", got)
	}
}
