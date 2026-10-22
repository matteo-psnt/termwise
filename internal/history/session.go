package history

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/matteo-psnt/termwise/internal/provider"
)

const (
	sessionsDir   = "sessions"
	sessionMaxAge = 30 * 24 * time.Hour
)

// Session modes. The TUI runs either the default agent or explain mode, and
// the two have different system prompts and tool sets, so a session saved in
// one must never be resumed in the other.
const (
	SessionModeAgent   = "agent"
	SessionModeExplain = "explain"
)

// Session is the full persistent state of one agent conversation.
type Session struct {
	ID string `json:"id"`
	// Mode is SessionModeAgent or SessionModeExplain. Files written before the
	// field existed have it empty; read it through SessionMode.
	Mode         string             `json:"mode,omitempty"`
	ProviderName string             `json:"provider_name"`
	ModelID      string             `json:"model_id"`
	Messages     []provider.Message `json:"messages"`
	Thread       []SerializedEntry  `json:"thread"`
	SavedAt      time.Time          `json:"saved_at"`
}

// SessionMode returns the mode the session was saved in. Sessions saved before
// the mode was recorded predate explain's own session slot only by a few
// releases and were overwhelmingly agent sessions, so empty means agent.
func (s Session) SessionMode() string {
	if s.Mode == "" {
		return SessionModeAgent
	}
	return s.Mode
}

// SerializedEntry is a JSON-serializable form of a ThreadEntry.
// The Type field discriminates between concrete types.
type SerializedEntry struct {
	Type    string `json:"type"`
	Content string `json:"content,omitempty"`
	Name    string `json:"name,omitempty"`
	Detail  string `json:"detail,omitempty"`
	IsError bool   `json:"is_error,omitempty"`
}

const (
	EntryTypeUser       = "user"
	EntryTypeAssistant  = "assistant"
	EntryTypeToolCall   = "tool_call"
	EntryTypeToolResult = "tool_result"
	EntryTypeCommand    = "command"
	EntryTypeError      = "error"
)

// SessionStore saves and loads sessions by ID.
// Sessions are stored as individual JSON files under <configDir>/sessions/.
type SessionStore struct {
	dir string
}

// NewSessionStore creates a SessionStore backed by the given config directory.
func NewSessionStore(configDir string) *SessionStore {
	return &SessionStore{dir: filepath.Join(configDir, sessionsDir)}
}

// Save writes the session to disk, creating the sessions directory if needed.
func (s *SessionStore) Save(session Session) error {
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return err
	}
	session.SavedAt = time.Now()
	data, err := json.Marshal(session)
	if err != nil {
		return err
	}
	return os.WriteFile(s.filePath(session.ID), data, 0o600)
}

// Load retrieves a session by ID. Returns (session, true, nil) if found,
// (Session{}, false, nil) if not found, or an error on I/O failure.
func (s *SessionStore) Load(id string) (Session, bool, error) {
	data, err := os.ReadFile(s.filePath(id))
	if os.IsNotExist(err) {
		return Session{}, false, nil
	}
	if err != nil {
		return Session{}, false, err
	}
	var session Session
	if err := json.Unmarshal(data, &session); err != nil {
		return Session{}, false, err
	}
	return session, true, nil
}

// Delete removes the session file for the given ID.
func (s *SessionStore) Delete(id string) error {
	err := os.Remove(s.filePath(id))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// PruneOld removes session files older than sessionMaxAge.
func (s *SessionStore) PruneOld() {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return
	}
	cutoff := time.Now().Add(-sessionMaxAge)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			_ = os.Remove(filepath.Join(s.dir, e.Name())) // Best-effort cleanup of stale session files.
		}
	}
}

// filePath returns the path for a session file given a raw session ID string.
// The ID is hashed to produce a safe filename.
func (s *SessionStore) filePath(id string) string {
	h := sha256.Sum256([]byte(id))
	return filepath.Join(s.dir, hex.EncodeToString(h[:8])+".json")
}
