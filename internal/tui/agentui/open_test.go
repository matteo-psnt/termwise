package agentui

import (
	"testing"

	"github.com/matteo-psnt/termwise/internal/history"
	"github.com/matteo-psnt/termwise/internal/provider"
	"github.com/matteo-psnt/termwise/internal/theme"
)

// savedSessionModel builds a model in the given mode with one exchange and
// saves it under id, the way quitting the TUI does.
func savedSessionModel(t *testing.T, store *history.SessionStore, id string, mode sessionMode) {
	t.Helper()
	m := newModel(modelConfig{
		providerName: "anthropic", modelID: "claude-sonnet-4-6", mode: mode,
		themeName: theme.DefaultName, initialCursorRow: -1,
		sessionStore: store, sessionID: id,
	})
	m.messages = []provider.Message{{Role: "user", Content: "hi"}}
	m.saveSession()
}

func TestResumeNeverCrossesModes(t *testing.T) {
	store := history.NewSessionStore(t.TempDir())
	savedSessionModel(t, store, "agent-id", modeAgent)
	savedSessionModel(t, store, "explain-id", modeExplain)

	if loadResumableSession(store, "agent-id", modeExplain) != nil {
		t.Error("explain resumed an agent session")
	}
	if loadResumableSession(store, "explain-id", modeAgent) != nil {
		t.Error("agent resumed an explain session")
	}
	if loadResumableSession(store, "agent-id", modeAgent) == nil {
		t.Error("agent did not resume its own session")
	}
	if loadResumableSession(store, "explain-id", modeExplain) == nil {
		t.Error("explain did not resume its own session")
	}
}

func TestResumeTreatsModelessSessionAsAgent(t *testing.T) {
	store := history.NewSessionStore(t.TempDir())
	if err := store.Save(history.Session{ID: "old", Messages: []provider.Message{{Role: "user", Content: "hi"}}}); err != nil {
		t.Fatal(err)
	}
	if loadResumableSession(store, "old", modeExplain) != nil {
		t.Error("explain resumed a session saved without a mode")
	}
	if loadResumableSession(store, "old", modeAgent) == nil {
		t.Error("agent did not resume a session saved without a mode")
	}
}
