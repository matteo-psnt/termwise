package envcontext

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The git probes run while the user may be using git in the same repo, so
// they must not take the index lock git status otherwise grabs.
func TestGitProbesDisableOptionalLocks(t *testing.T) {
	bin := t.TempDir()
	record := filepath.Join(t.TempDir(), "env")
	script := "#!/bin/sh\necho \"$GIT_OPTIONAL_LOCKS\" > " + record + "\n"
	if err := os.WriteFile(filepath.Join(bin, "git"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	t.Setenv("GIT_OPTIONAL_LOCKS", "")

	if _, err := gitOut(context.Background(), t.TempDir(), "status", "--porcelain"); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(record)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(got)) != "0" {
		t.Errorf("git ran with GIT_OPTIONAL_LOCKS=%q, want 0", strings.TrimSpace(string(got)))
	}
}
