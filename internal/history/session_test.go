package history

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSessionModeRoundTrips(t *testing.T) {
	store := NewSessionStore(t.TempDir())
	if err := store.Save(Session{ID: "1", Mode: SessionModeExplain}); err != nil {
		t.Fatal(err)
	}
	got, ok, err := store.Load("1")
	if err != nil || !ok {
		t.Fatalf("Load = %v, %v", ok, err)
	}
	if got.SessionMode() != SessionModeExplain {
		t.Errorf("SessionMode() = %q, want %q", got.SessionMode(), SessionModeExplain)
	}
}

func TestSessionModeMissingMeansAgent(t *testing.T) {
	dir := t.TempDir()
	store := NewSessionStore(dir)
	// A file written before the mode field existed.
	if err := os.MkdirAll(filepath.Join(dir, sessionsDir), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.filePath("old"), []byte(`{"id":"old","messages":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	got, ok, err := store.Load("old")
	if err != nil || !ok {
		t.Fatalf("Load = %v, %v", ok, err)
	}
	if got.SessionMode() != SessionModeAgent {
		t.Errorf("SessionMode() = %q, want %q", got.SessionMode(), SessionModeAgent)
	}
}
