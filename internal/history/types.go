package history

import (
	"time"

	"github.com/matteo-psnt/termwise/internal/provider"
)

// Session is the full persistent state of one agent conversation.
type Session struct {
	ID           string             `json:"id"`
	ProviderName string             `json:"provider_name"`
	ModelID      string             `json:"model_id"`
	Messages     []provider.Message `json:"messages"`
	Thread       []SerializedEntry  `json:"thread"`
	SavedAt      time.Time          `json:"saved_at"`
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
