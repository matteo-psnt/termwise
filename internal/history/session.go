package history

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

const (
	sessionsDir   = "sessions"
	sessionMaxAge = 30 * 24 * time.Hour
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

// PruneOld removes session files older than 30 days.
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
			os.Remove(filepath.Join(s.dir, e.Name()))
		}
	}
}

// filePath returns the path for a session file given a raw session ID string.
// The ID is hashed to produce a safe filename.
func (s *SessionStore) filePath(id string) string {
	h := sha256.Sum256([]byte(id))
	return filepath.Join(s.dir, hex.EncodeToString(h[:8])+".json")
}
