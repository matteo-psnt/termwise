package tools

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// grepDir makes a temp working directory holding files and chdirs into it.
func grepDir(t *testing.T, files map[string]string) string {
	t.Helper()
	if _, err := exec.LookPath("rg"); err != nil {
		t.Skip("rg not installed")
	}
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for name, content := range files {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	t.Chdir(dir)
	return dir
}

func TestGrepCountSingleFile(t *testing.T) {
	grepDir(t, map[string]string{"a.txt": "foo\nbar\nfoo\n"})
	out, isErr := grep(context.Background(), map[string]any{
		"pattern": "foo", "path": "a.txt", "output_mode": "count",
	})
	if isErr {
		t.Fatal(out)
	}
	if !strings.HasPrefix(out, "a.txt:2\n") || !strings.Contains(out, "Found 2 total occurrences across 1 file.") {
		t.Fatalf("got %q", out)
	}
}

func TestGrepCountPathWithColon(t *testing.T) {
	grepDir(t, map[string]string{"a:1.txt": "foo\n", "b.txt": "foo\nfoo\n"})
	out, _ := grep(context.Background(), map[string]any{"pattern": "foo", "output_mode": "count"})
	if !strings.Contains(out, "a:1.txt:1") || !strings.Contains(out, "Found 3 total occurrences across 2 files.") {
		t.Fatalf("got %q", out)
	}
}
