package agentui

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/matteo-psnt/termwise/internal/config"
	"github.com/matteo-psnt/termwise/internal/history"
	"github.com/matteo-psnt/termwise/internal/provider"
	"github.com/matteo-psnt/termwise/internal/theme"

	// Registers the providers applyModel builds clients for.
	_ "github.com/matteo-psnt/termwise/internal/provider/builtin"
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

// resumeModel opens a model the way openProgram does when resuming a session
// saved on anthropic/claude-opus-4-6, with openai/gpt-5 currently selected.
func resumeModel(t *testing.T, cfg config.FileConfig) Model {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := config.SaveConfig(path, cfg); err != nil {
		t.Fatal(err)
	}
	return newModel(modelConfig{
		providerName: "openai", modelID: "gpt-5",
		themeName: theme.DefaultName, initialCursorRow: -1, cfgPath: path,
		initialSession: &history.Session{
			ProviderName: "anthropic", ModelID: "claude-opus-4-6",
			Messages: []provider.Message{{Role: "user", Content: "hi"}},
		},
	})
}

func TestResumeRestoresSessionModel(t *testing.T) {
	t.Setenv("TW_TEST_ANTHROPIC_KEY", "sk-test")
	m := resumeModel(t, config.FileConfig{Providers: map[string]config.ProviderConfig{
		"anthropic": {EnvVar: "TW_TEST_ANTHROPIC_KEY", Effort: "high"},
	}})
	if m.providerName != "anthropic" || m.modelID != "claude-opus-4-6" {
		t.Fatalf("active model = %s/%s, want the session's anthropic/claude-opus-4-6", m.providerName, m.modelID)
	}
	if m.provider == nil {
		t.Fatal("provider client was not rebuilt for the session's provider")
	}
	if m.effort != "high" {
		t.Errorf("effort = %q, want the restored provider's %q", m.effort, "high")
	}
	for _, e := range m.thread {
		if _, ok := e.(SystemEntry); ok {
			t.Errorf("unexpected notice after a successful restore: %v", e)
		}
	}
}

func TestResumeKeepsCurrentModelWhenSessionProviderUnavailable(t *testing.T) {
	m := resumeModel(t, config.FileConfig{})
	if m.providerName != "openai" || m.modelID != "gpt-5" {
		t.Fatalf("active model = %s/%s, want openai/gpt-5 kept", m.providerName, m.modelID)
	}
	var notices []string
	for _, e := range m.thread {
		if s, ok := e.(SystemEntry); ok {
			notices = append(notices, s.Content)
		}
	}
	if len(notices) != 1 || !strings.Contains(notices[0], "anthropic/claude-opus-4-6") || !strings.Contains(notices[0], "openai/gpt-5") {
		t.Fatalf("notices = %q, want one naming both models", notices)
	}
}

// Piped stdin rides along with the next message even when a session is
// resumed, so the thread must show it then too.
func TestResumeStillShowsPipedStdin(t *testing.T) {
	m := newModel(modelConfig{
		providerName: "anthropic", modelID: "claude-sonnet-4-6",
		themeName: theme.DefaultName, initialCursorRow: -1,
		stdin: "ls -la\n",
		initialSession: &history.Session{
			ProviderName: "anthropic", ModelID: "claude-sonnet-4-6",
			Messages: []provider.Message{{Role: "user", Content: "hi"}},
			Thread:   []history.SerializedEntry{{Type: history.EntryTypeUser, Content: "hi"}},
		},
	})
	if len(m.thread) != 2 {
		t.Fatalf("thread = %#v, want the resumed entry and the stdin marker", m.thread)
	}
	if u, ok := m.thread[1].(UserEntry); !ok || !strings.HasPrefix(u.Content, "[stdin:") {
		t.Errorf("last entry = %#v, want the stdin marker", m.thread[1])
	}
}
