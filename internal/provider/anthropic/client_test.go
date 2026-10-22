package anthropic

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	sdk "github.com/anthropics/anthropic-sdk-go"

	"github.com/matteo-psnt/termwise/internal/provider"
)

func TestListModelsFollowsPages(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("after_id") == "" {
			_, _ = fmt.Fprint(w, `{"data":[{"id":"m1","display_name":"M1","type":"model","created_at":"2025-01-01T00:00:00Z"}],"has_more":true,"first_id":"m1","last_id":"m1"}`)
			return
		}
		_, _ = fmt.Fprint(w, `{"data":[{"id":"m2","display_name":"M2","type":"model","created_at":"2025-01-01T00:00:00Z"}],"has_more":false,"first_id":"m2","last_id":"m2"}`)
	}))
	defer srv.Close()

	c, err := New(provider.Config{APIKey: "test", BaseURL: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	models, err := c.ListModels(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 2 || models[0].ID != "m1" || models[1].ID != "m2" {
		t.Fatalf("got %+v, want m1 and m2 from both pages", models)
	}
}

func TestRequestTimeoutCoversThinkingBudget(t *testing.T) {
	// A non-streaming request has to finish within the timeout, so it must
	// allow at least what the SDK expects for the largest max_tokens we send.
	p := toParams("claude-x", "", nil, nil, "high")
	want, err := sdk.CalculateNonStreamingTimeout(int(p.MaxTokens), p.Model, nil)
	if err != nil {
		t.Fatal(err)
	}
	if provider.DefaultRequestTimeout < want {
		t.Fatalf("DefaultRequestTimeout = %v, want at least %v for max_tokens %d", provider.DefaultRequestTimeout, want, p.MaxTokens)
	}
}
