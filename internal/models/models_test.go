package models

import "testing"

// The catalog is embedded at compile time, so load() panics on a malformed
// file rather than leaving every lookup silently empty. That only helps if
// something parses it during a test run — this is that something.
func TestEmbeddedCatalogParses(t *testing.T) {
	id := DefaultModel("anthropic")
	if id == "" {
		t.Fatal("no default model for anthropic — models.yaml parsed empty")
	}
	md := Find("anthropic", id)
	if md == nil {
		t.Fatalf("default model %q is not in the anthropic catalog", id)
	}
	if md.Context <= 0 {
		t.Errorf("%s has context %d; the TUI falls back to 32k when this is unset", id, md.Context)
	}
}
