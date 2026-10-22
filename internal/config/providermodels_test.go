package config

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// A connectivity check that answers from the model cache would report a
// server as reachable for days after it went away.
func TestCheckProviderConnectivityDoesNotAnswerFromCache(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"qwen3","object":"model"}]}`))
	}))
	pc := ProviderConfig{Model: "qwen3", BaseURL: srv.URL}

	if err := CheckProviderConnectivity(context.Background(), "ollama", pc); err != nil {
		t.Fatalf("first check with the server up: %v", err)
	}
	srv.Close()
	if err := CheckProviderConnectivity(context.Background(), "ollama", pc); err == nil {
		t.Error("check passed after the server went away: it answered from the cache")
	}
	// The picker path still gets the cached list.
	if ms, err := ListProviderModels(context.Background(), "ollama", pc); err != nil || len(ms) != 1 {
		t.Errorf("ListProviderModels = %v, %v; want the cached list", ms, err)
	}
}
