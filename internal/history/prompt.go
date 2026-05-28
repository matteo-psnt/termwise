package history

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
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

// Load reads the history file into memory. Safe to call multiple times;
// subsequent calls are no-ops if already loaded.
func (h *PromptHistory) Load() error {
	h.mu.Lock()
	defer h.mu.Unlock()

	f, err := os.Open(h.path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	defer f.Close()

	var entries []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var e promptEntry
		if err := json.Unmarshal(sc.Bytes(), &e); err != nil {
			continue
		}
		entries = append(entries, e.Text)
	}

	// File is oldest-first; reverse to newest-first.
	for i, j := 0, len(entries)-1; i < j; i, j = i+1, j-1 {
		entries[i], entries[j] = entries[j], entries[i]
	}
	h.entries = entries
	return sc.Err()
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

	// Append to file.
	if err := os.MkdirAll(filepath.Dir(h.path), 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(h.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()

	line, err := json.Marshal(promptEntry{Text: text, Ts: time.Now()})
	if err != nil {
		return err
	}
	_, err = f.Write(append(line, '\n'))
	return err
}

// Entries returns all prompts in newest-first order.
func (h *PromptHistory) Entries() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]string, len(h.entries))
	copy(out, h.entries)
	return out
}
