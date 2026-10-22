package tty

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadPipedFromPipe(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close() }()
	go func() {
		_, _ = w.WriteString("line one\nline two\n\n")
		_ = w.Close()
	}()

	got, piped, err := ReadPiped(r)
	if err != nil {
		t.Fatalf("ReadPiped: %v", err)
	}
	if !piped {
		t.Fatal("a pipe should be reported as piped")
	}
	if want := "line one\nline two"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestReadPipedFromFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "input")
	// Leading whitespace and inner blank lines are kept; only trailing
	// newlines are trimmed.
	if err := os.WriteFile(path, []byte("  indented\n\nlast \n"), 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()

	got, piped, err := ReadPiped(f)
	if err != nil || !piped {
		t.Fatalf("ReadPiped = (%q, %v, %v)", got, piped, err)
	}
	if want := "  indented\n\nlast "; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestReadPipedEmpty(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close() }()
	_ = w.Close()

	got, piped, err := ReadPiped(r)
	if err != nil || !piped || got != "" {
		t.Fatalf("ReadPiped = (%q, %v, %v), want (\"\", true, nil)", got, piped, err)
	}
}
