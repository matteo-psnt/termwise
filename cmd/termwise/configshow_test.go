package main

import (
	"encoding/json"
	"testing"
)

// With no file the runtime runs from an API key in the environment, so
// `show --json` has to print that config: a script parsing stdout would
// otherwise get nothing on a successful exit.
func TestShowJSONDescribesEnvOnlyConfig(t *testing.T) {
	isolateConfigEnv(t)
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant")
	if err := configShowCmd.Flags().Set("json", "true"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = configShowCmd.Flags().Set("json", "false") })

	var runErr error
	out := captureStdout(t, func() { runErr = configShowCmd.RunE(configShowCmd, nil) })
	if runErr != nil {
		t.Fatal(runErr)
	}
	var got showJSON
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, out)
	}
	if got.Source != sourceEnvironment || got.SelectedProvider != "anthropic" {
		t.Errorf("source = %q, selected_provider = %q; want environment, anthropic", got.Source, got.SelectedProvider)
	}
	if auth := got.Providers["anthropic"].Auth; auth.Source != "ANTHROPIC_API_KEY" {
		t.Errorf("auth source = %q, want ANTHROPIC_API_KEY", auth.Source)
	}
}
