package history

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"
)

const (
	promptHistoryFile = "prompt-history.jsonl"
	maxPromptEntries  = 1000
)

type promptEntry struct {
	Text string    `json:"text"`
	Ts   time.Time `json:"ts"`
}

// PromptHistory manages the persistent prompt history file.
// Entries are stored newest-first in memory; the file is append-only on disk.
type PromptHistory struct {
	mu      sync.Mutex
	path    string
	entries []string // newest first
}

// NewPromptHistory creates a PromptHistory backed by the given directory.
// The history file is created lazily on first write.
func NewPromptHistory(configDir string) *PromptHistory {
	return &PromptHistory{
		path: filepath.Join(configDir, promptHistoryFile),
	}
}

// Load reads the history file into memory, replacing any entries already
// held. Calling it again re-reads the file rather than doing nothing. Only
// the newest maxPromptEntries are kept, and once the file holds more than
// twice that it is rewritten to just those, so it cannot grow without bound.
func (h *PromptHistory) Load() error {
	h.mu.Lock()
	defer h.mu.Unlock()

	// ReadFile rather than bufio.Scanner: Scanner fails on any line over
	// 64KB, and a single long pasted prompt would then break history.
	data, err := os.ReadFile(h.path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}

	var entries []string // oldest first
	var lines [][]byte   // the raw line behind each entry
	for line := range bytes.SplitSeq(data, []byte{'\n'}) {
		var e promptEntry
		if json.Unmarshal(line, &e) != nil {
			continue
		}
		entries = append(entries, e.Text)
		lines = append(lines, line)
	}

	if len(entries) > maxPromptEntries {
		drop := len(entries) - maxPromptEntries
		entries = entries[drop:]
		if len(lines) > 2*maxPromptEntries {
			// Best-effort: a failed rewrite leaves the full file in place.
			_ = rewriteLines(h.path, lines[drop:])
		}
	}

	// File is oldest-first; reverse to newest-first.
	slices.Reverse(entries)
	h.entries = entries
	return nil
}

// rewriteLines atomically replaces path with lines, one per line, via a temp
// file and rename, so a crash mid-write cannot truncate the history.
func rewriteLines(path string, lines [][]byte) error {
	// Rename replaces whatever is at path, so write through a symlink (a
	// history file kept in a dotfiles repo) to its target rather than swap
	// the link for a plain file.
	if target, err := filepath.EvalSymlinks(path); err == nil {
		path = target
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) //nolint:errcheck // gone after a successful rename
	w := bufio.NewWriter(tmp)
	for _, line := range lines {
		_, _ = w.Write(line)
		_ = w.WriteByte('\n')
	}
	if err := w.Flush(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// Push adds a prompt to the history. Consecutive duplicate entries are
// deduplicated. The file is appended to immediately.
func (h *PromptHistory) Push(text string) error {
	if text == "" {
		return nil
	}
	h.mu.Lock()
	defer h.mu.Unlock()

	// Deduplicate against the most recent entry.
	if len(h.entries) > 0 && h.entries[0] == text {
		return nil
	}

	// Prepend in-memory.
	h.entries = append([]string{text}, h.entries...)
	if len(h.entries) > maxPromptEntries {
		h.entries = h.entries[:maxPromptEntries]
	}

	return appendJSONL(h.path, promptEntry{Text: text, Ts: time.Now()})
}

// Entries returns all prompts in newest-first order.
func (h *PromptHistory) Entries() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]string, len(h.entries))
	copy(out, h.entries)
	return out
}
