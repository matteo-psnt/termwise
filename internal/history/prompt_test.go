package history

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestPromptHistoryLoadsLongPrompt(t *testing.T) {
	dir := t.TempDir()
	h := NewPromptHistory(dir)
	long := strings.Repeat("x", 100*1024)
	for _, p := range []string{"first", long, "last"} {
		if err := h.Push(p); err != nil {
			t.Fatal(err)
		}
	}

	loaded := NewPromptHistory(dir)
	if err := loaded.Load(); err != nil {
		t.Fatalf("load: %v", err)
	}
	got := loaded.Entries()
	if len(got) != 3 || got[0] != "last" || got[1] != long || got[2] != "first" {
		t.Fatalf("got %d entries, want last, long, first", len(got))
	}
}

func TestPromptHistoryLoadCapsAndCompacts(t *testing.T) {
	dir := t.TempDir()
	h := NewPromptHistory(dir)
	total := 2*maxPromptEntries + 1
	for i := range total {
		if err := h.Push("p" + strconv.Itoa(i)); err != nil {
			t.Fatal(err)
		}
	}

	loaded := NewPromptHistory(dir)
	if err := loaded.Load(); err != nil {
		t.Fatal(err)
	}
	got := loaded.Entries()
	if len(got) != maxPromptEntries || got[0] != "p"+strconv.Itoa(total-1) {
		t.Fatalf("got %d entries starting %q, want %d newest", len(got), got[0], maxPromptEntries)
	}

	data, err := os.ReadFile(filepath.Join(dir, promptHistoryFile))
	if err != nil {
		t.Fatal(err)
	}
	if n := bytes.Count(data, []byte{'\n'}); n != maxPromptEntries {
		t.Fatalf("file has %d lines after load, want it compacted to %d", n, maxPromptEntries)
	}
	if matches, _ := filepath.Glob(filepath.Join(dir, "*.tmp")); len(matches) != 0 {
		t.Fatalf("temp files left behind: %v", matches)
	}

	// The compacted file loads to the same entries.
	again := NewPromptHistory(dir)
	if err := again.Load(); err != nil {
		t.Fatal(err)
	}
	if e := again.Entries(); len(e) != maxPromptEntries || e[0] != got[0] || e[len(e)-1] != got[len(got)-1] {
		t.Fatal("compacted file loaded differently")
	}
}

func TestPromptHistoryLoadKeepsFileBelowCompactionThreshold(t *testing.T) {
	dir := t.TempDir()
	h := NewPromptHistory(dir)
	for i := range maxPromptEntries + 10 {
		if err := h.Push("p" + strconv.Itoa(i)); err != nil {
			t.Fatal(err)
		}
	}
	loaded := NewPromptHistory(dir)
	if err := loaded.Load(); err != nil {
		t.Fatal(err)
	}
	if n := len(loaded.Entries()); n != maxPromptEntries {
		t.Fatalf("got %d entries, want %d", n, maxPromptEntries)
	}
	data, _ := os.ReadFile(filepath.Join(dir, promptHistoryFile))
	if n := bytes.Count(data, []byte{'\n'}); n != maxPromptEntries+10 {
		t.Fatalf("file rewritten below threshold: %d lines", n)
	}
}
