package http

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFetchFromAPI(t *testing.T) {
	t.Parallel()
	pemKey := "-----BEGIN PUBLIC KEY-----\nMFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAEtest\n-----END PUBLIC KEY-----\n"

	t.Run("successful fetch", func(t *testing.T) {
		t.Parallel()
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(pemKey)) //nolint:errcheck
		}))
		defer server.Close()

		body, err := FetchFromAPI(http.DefaultClient, server.URL)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if string(body) != pemKey {
			t.Errorf("got %q, want %q", string(body), pemKey)
		}
	})

	t.Run("non-200 status", func(t *testing.T) {
		t.Parallel()
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("internal error")) //nolint:errcheck
		}))
		defer server.Close()

		_, err := FetchFromAPI(http.DefaultClient, server.URL)
		if err == nil {
			t.Error("expected error for non-200 status")
		}
		if !strings.Contains(err.Error(), "returned status 500") {
			t.Errorf("expected status 500 in error, got: %v", err)
		}
	})

	t.Run("connection error", func(t *testing.T) {
		t.Parallel()
		_, err := FetchFromAPI(http.DefaultClient, "http://localhost:1")
		if err == nil {
			t.Error("expected error for connection failure")
		}
	})

	t.Run("response body limited to 1MiB", func(t *testing.T) {
		t.Parallel()
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write(make([]byte, 2<<20)) //nolint:errcheck
		}))
		defer server.Close()

		body, err := FetchFromAPI(http.DefaultClient, server.URL)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(body) > 1<<20 {
			t.Errorf("expected body capped at 1MiB, got %d bytes", len(body))
		}
	})
}
