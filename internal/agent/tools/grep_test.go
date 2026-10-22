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

func TestGrepContentRelativizesContextLines(t *testing.T) {
	// Names with '-' and ':' must not be mistaken for rg's separators.
	for _, name := range []string{"sub/f-2.txt", "d-1:x/f-2:3.txt"} {
		t.Run(name, func(t *testing.T) {
			grepDir(t, map[string]string{name: "before\nfoo\nafter\n"})
			out, isErr := grep(context.Background(), map[string]any{
				"pattern": "foo", "output_mode": "content", "-C": float64(1),
			})
			if isErr {
				t.Fatal(out)
			}
			want := name + "-1-before\n" + name + ":2:foo\n" + name + "-3-after"
			if out != want {
				t.Fatalf("got\n%s\nwant\n%s", out, want)
			}
		})
	}
}

func TestGrepContentSingleFileWithoutLineNumbers(t *testing.T) {
	grepDir(t, map[string]string{"a.txt": "x\nfoo\n"})
	out, _ := grep(context.Background(), map[string]any{
		"pattern": "foo", "path": "a.txt", "output_mode": "content", "-n": false, "-B": float64(1),
	})
	if want := "a.txt-x\na.txt:foo"; out != want {
		t.Fatalf("got %q, want %q", out, want)
	}
}

func TestSearchToolsNameTheMissingDefaultPath(t *testing.T) {
	dir := grepDir(t, nil)
	if err := os.Remove(dir); err != nil {
		t.Fatal(err)
	}
	for name, fn := range map[string]func(context.Context, map[string]any) (string, bool){"grep": grep, "glob": glob} {
		out, isErr := fn(context.Background(), map[string]any{"pattern": "x"})
		// Getwd may or may not still report the removed directory; either
		// way the message must say what was missing.
		if !isErr || strings.HasSuffix(out, ": ") {
			t.Errorf("%s: got %q", name, out)
		}
	}
}
