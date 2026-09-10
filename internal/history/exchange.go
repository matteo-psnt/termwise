package history

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	exchangeFile = "exchanges.jsonl"
	// maxStoredResponse caps a single logged response. Long prose answers are
	// truncated rather than dropped — the point of this log is to judge output
	// quality later, and the opening lines carry most of that signal.
	maxStoredResponse = 4000
)

// Exchange is one completed turn: what the user asked and what came back.
//
// The prompt history alone records only what the user typed, which makes it
// impossible to tell afterwards whether an answer was any good. This log exists
// so prompt and tool-description changes can be evaluated against real use
// instead of recalled impressions.
type Exchange struct {
	Ts       time.Time `json:"ts"`
	Prompt   string    `json:"prompt"`
	Kind     string    `json:"kind"` // ExchangeKind*
	Response string    `json:"response"`
	Provider string    `json:"provider,omitempty"`
	Model    string    `json:"model,omitempty"`
	Tools    []string  `json:"tools,omitempty"` // tool names used, in call order
	Turns    int       `json:"turns,omitempty"` // model round-trips spent
}

// Exchange kinds. Command is the main job; text is an answer; error is a failed turn.
const (
	ExchangeKindCommand = "command"
	ExchangeKindText    = "text"
	ExchangeKindError   = "error"
)

// ExchangeLog is an append-only JSONL log of completed turns.
type ExchangeLog struct {
	mu   sync.Mutex
	path string
}

// NewExchangeLog creates a log backed by the given config directory. The file
// is created lazily on first append.
func NewExchangeLog(configDir string) *ExchangeLog {
	return &ExchangeLog{path: filepath.Join(configDir, exchangeFile)}
}

// Append writes one exchange. A nil log is a no-op so callers need no guard,
// and write failures are returned but are never worth interrupting a session
// over.
func (l *ExchangeLog) Append(e Exchange) error {
	if l == nil || e.Prompt == "" {
		return nil
	}
	if e.Ts.IsZero() {
		e.Ts = time.Now()
	}
	if len(e.Response) > maxStoredResponse {
		e.Response = e.Response[:maxStoredResponse] + "…[truncated]"
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	if err := os.MkdirAll(filepath.Dir(l.path), 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(l.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	line, err := json.Marshal(e)
	if err != nil {
		_ = f.Close()
		return err
	}
	if _, err := f.Write(append(line, '\n')); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

// Load reads every logged exchange, oldest first. Malformed lines are skipped.
func (l *ExchangeLog) Load() ([]Exchange, error) {
	data, err := os.ReadFile(l.path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []Exchange
	for _, line := range splitLines(data) {
		var e Exchange
		if json.Unmarshal(line, &e) == nil {
			out = append(out, e)
		}
	}
	return out, nil
}

func splitLines(b []byte) [][]byte {
	var out [][]byte
	start := 0
	for i, c := range b {
		if c == '\n' {
			if i > start {
				out = append(out, b[start:i])
			}
			start = i + 1
		}
	}
	if start < len(b) {
		out = append(out, b[start:])
	}
	return out
}
