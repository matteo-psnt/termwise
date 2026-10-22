package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/matteo-psnt/termwise/internal/config"
)

// `config model list` and `config test` have to talk to the provider the
// runtime would use, or they report "no provider selected" for a setup that
// `tw` runs fine.
func TestEffectiveProviderConfigMatchesRuntime(t *testing.T) {
	cfgPath := isolateConfigEnv(t)

	t.Setenv("ANTHROPIC_API_KEY", "sk-ant")
	name, pc, err := effectiveProviderConfig("")
	if err != nil || name != "anthropic" || pc.EnvVar != "ANTHROPIC_API_KEY" {
		t.Errorf("env-only: got %q, %+v, %v; want anthropic via $ANTHROPIC_API_KEY", name, pc, err)
	}

	writeTestConfig(t, cfgPath, `selected_provider = "anthropic"

[providers.anthropic]
model = "claude-haiku-4-5-20251001"
`)
	t.Setenv(config.EnvProvider, "ollama")
	t.Setenv(config.EnvModel, "qwen3")
	name, pc, err = effectiveProviderConfig("")
	if err != nil || name != "ollama" || pc.Model != "qwen3" {
		t.Errorf("with overrides: got %q, %+v, %v; want ollama with qwen3", name, pc, err)
	}
}

// ResolveAuth falls back to the provider's default env var when the
// configured method fails. `config test` must say so, or a broken api_key_cmd
// passes as connected.
func TestConfigTestReportsAuthFallback(t *testing.T) {
	cfgPath := isolateConfigEnv(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"gpt-4o","object":"model"}]}`))
	}))
	defer srv.Close()
	t.Setenv("OPENAI_API_KEY", "sk-oai")
	writeTestConfig(t, cfgPath, `selected_provider = "openai"

[providers.openai]
auth        = "cmd"
api_key_cmd = "exit 1"
model       = "gpt-4o"
base_url    = "`+srv.URL+`"
`)

	var runErr error
	stderr := captureFile(t, &os.Stderr, func() {
		captureStdout(t, func() { runErr = configTestCmd.RunE(configTestCmd, nil) })
	})
	if runErr != nil {
		t.Fatal(runErr)
	}
	if !strings.Contains(stderr, "configured auth failed") || !strings.Contains(stderr, "$OPENAI_API_KEY") {
		t.Errorf("stderr = %q, want a warning that $OPENAI_API_KEY was used instead", stderr)
	}
}

// A status line is erased with an escape code, which only a terminal
// understands; in a log it would leave the message and the raw code behind.
func TestStatusLineSilentWithoutTerminal(t *testing.T) {
	var b strings.Builder
	statusLine(&b, false, "Validating model...")()
	if b.Len() != 0 {
		t.Errorf("status line wrote %q to a non-terminal", b.String())
	}
	statusLine(&b, true, "x")()
	if b.String() != "x\r\033[K" {
		t.Errorf("status line on a terminal = %q", b.String())
	}
}
