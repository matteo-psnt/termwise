package configui

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/zalando/go-keyring"

	"github.com/matteo-psnt/termwise/internal/config"
	_ "github.com/matteo-psnt/termwise/internal/provider/builtin"
)

func TestBuildProviderConfigFromAuthInput(t *testing.T) {
	tests := []struct {
		name             string
		provider, method string
		input, fallback  string
		want             config.ProviderConfig
	}{
		{"env typed", "openai", "env", "MY_KEY", "OPENAI_API_KEY",
			config.ProviderConfig{Auth: "env", EnvVar: "MY_KEY"}},
		{"env falls back", "openai", "env", "", "OPENAI_API_KEY",
			config.ProviderConfig{Auth: "env", EnvVar: "OPENAI_API_KEY"}},
		{"cmd", "openai", "cmd", "pass show k", "",
			config.ProviderConfig{Auth: "cmd", APIKeyCmd: "pass show k"}},
		{"keychain never stores the raw key", "openai", "keychain", "sk-secret", "",
			config.ProviderConfig{Auth: "keychain", KeychainEntry: config.DefaultKeychainEntry("openai")}},
		{"typed value trimmed", "openai", "cmd", "  pass show k \n", "",
			config.ProviderConfig{Auth: "cmd", APIKeyCmd: "pass show k"}},
		{"blank value falls back", "openai", "env", "   ", "OPENAI_API_KEY",
			config.ProviderConfig{Auth: "env", EnvVar: "OPENAI_API_KEY"}},
		{"no-auth default URL omitted", "ollama", "env", "", "http://localhost:11434",
			config.ProviderConfig{Auth: "env"}},
		{"no-auth custom URL kept", "ollama", "env", "http://box:11434", "http://localhost:11434",
			config.ProviderConfig{Auth: "env", BaseURL: "http://box:11434"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildProviderConfigFromAuthInput(tt.provider, tt.method, tt.input, tt.fallback)
			if got != tt.want {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

// fakeOpenAI serves an OpenAI-style model list to requests bearing goodKey
// and rejects every other key.
func fakeOpenAI(t *testing.T, goodKey string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Header.Get("Authorization") != "Bearer "+goodKey {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":{"message":"Incorrect API key","type":"invalid_request_error","code":"invalid_api_key"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"gpt-5","object":"model","created":0,"owned_by":"openai"}]}`))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// mockKeychain swaps the system keychain for an in-memory one holding
// openai's key, and returns a reader for that key.
func mockKeychain(t *testing.T, openaiKey string) func() string {
	t.Helper()
	keyring.MockInit()
	entry := config.DefaultKeychainEntry("openai")
	if err := keyring.Set("termwise", entry, openaiKey); err != nil {
		t.Fatal(err)
	}
	return func() string {
		v, _ := keyring.Get("termwise", entry)
		return v
	}
}

func TestKeychainKeyStoredOnlyOnceVerified(t *testing.T) {
	stored := mockKeychain(t, "sk-old")
	srv := fakeOpenAI(t, "sk-good")
	pc := config.ProviderConfig{Auth: "keychain", KeychainEntry: config.DefaultKeychainEntry("openai"), BaseURL: srv.URL}

	if _, err := listModelsFor(context.Background(), "openai", pc, "sk-bad"); err == nil {
		t.Fatal("bad key verified")
	}
	if got := stored(); got != "sk-old" {
		t.Fatalf("keychain = %q after a rejected key, want the old key kept", got)
	}

	list, err := listModelsFor(context.Background(), "openai", pc, "sk-good")
	if err != nil || len(list) != 1 {
		t.Fatalf("good key: %v, %v", list, err)
	}
	if got := stored(); got != "sk-good" {
		t.Errorf("keychain = %q, want the verified key", got)
	}
}

func TestAuthEditorBadKeyKeepsKeychain(t *testing.T) {
	stored := mockKeychain(t, "sk-old")
	srv := fakeOpenAI(t, "sk-good")
	pc := config.ProviderConfig{Auth: "keychain", KeychainEntry: config.DefaultKeychainEntry("openai"), BaseURL: srv.URL}
	m := newAuthEditor("openai", pc, false, false, newStylesForTheme(true, ""))

	m = pressAuth(m, keyEnter) // keychain, the current method
	m.input.SetValue("sk-bad")
	m = pressAuth(m, keyEnter)
	if m.step != authStepVerify {
		t.Fatalf("step = %v, want verify", m.step)
	}
	m, _ = m.Update(m.verifyCmd()())
	if m.done || m.inputErr == "" {
		t.Errorf("done = %v, inputErr = %q; want the rejection shown", m.done, m.inputErr)
	}
	if got := stored(); got != "sk-old" {
		t.Errorf("keychain = %q after a rejected key, want the old key kept", got)
	}
}

func TestKeychainKeyTrimmedAndNeverBlank(t *testing.T) {
	stored := mockKeychain(t, "sk-old")
	srv := fakeOpenAI(t, "sk-good")
	pc := config.ProviderConfig{Auth: "keychain", KeychainEntry: config.DefaultKeychainEntry("openai"), BaseURL: srv.URL}

	if _, err := listModelsFor(context.Background(), "openai", pc, "  \n"); err == nil {
		t.Error("blank key accepted")
	}
	if got := stored(); got != "sk-old" {
		t.Fatalf("keychain = %q after a blank key, want the old key kept", got)
	}
	if _, err := listModelsFor(context.Background(), "openai", pc, " sk-good\n"); err != nil {
		t.Fatalf("pasted key with whitespace: %v", err)
	}
	if got := stored(); got != "sk-good" {
		t.Errorf("keychain = %q, want the trimmed key", got)
	}
}
