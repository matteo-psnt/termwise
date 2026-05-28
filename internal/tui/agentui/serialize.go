package agentui

import "github.com/matteo-psnt/termwise/internal/history"

// serializeThread converts a []ThreadEntry to []history.SerializedEntry.
func serializeThread(entries []ThreadEntry) []history.SerializedEntry {
	out := make([]history.SerializedEntry, 0, len(entries))
	for _, e := range entries {
		out = append(out, serializeEntry(e))
	}
	return out
}

func serializeEntry(e ThreadEntry) history.SerializedEntry {
	switch v := e.(type) {
	case UserEntry:
		return history.SerializedEntry{Type: history.EntryTypeUser, Content: v.Content}
	case AssistantEntry:
		return history.SerializedEntry{Type: history.EntryTypeAssistant, Content: v.Content}
	case ToolCallEntry:
		return history.SerializedEntry{Type: history.EntryTypeToolCall, Name: v.Name, Detail: v.Detail}
	case ToolResultEntry:
		return history.SerializedEntry{Type: history.EntryTypeToolResult, Content: v.Content, IsError: v.IsError}
	case CommandEntry:
		return history.SerializedEntry{Type: history.EntryTypeCommand, Content: v.Content}
	case ErrorEntry:
		return history.SerializedEntry{Type: history.EntryTypeError, Content: v.Content}
	default:
		return history.SerializedEntry{Type: history.EntryTypeError, Content: "[unknown entry type]"}
	}
}

// deserializeThread converts []history.SerializedEntry back to []ThreadEntry.
func deserializeThread(entries []history.SerializedEntry) []ThreadEntry {
	out := make([]ThreadEntry, 0, len(entries))
	for _, e := range entries {
		if te := deserializeEntry(e); te != nil {
			out = append(out, te)
		}
	}
	return out
}

func deserializeEntry(e history.SerializedEntry) ThreadEntry {
	switch e.Type {
	case history.EntryTypeUser:
		return UserEntry{Content: e.Content}
	case history.EntryTypeAssistant:
		return AssistantEntry{Content: e.Content}
	case history.EntryTypeToolCall:
		return ToolCallEntry{Name: e.Name, Detail: e.Detail}
	case history.EntryTypeToolResult:
		return ToolResultEntry{Content: e.Content, IsError: e.IsError}
	case history.EntryTypeCommand:
		return CommandEntry{Content: e.Content}
	case history.EntryTypeError:
		return ErrorEntry{Content: e.Content}
	default:
		return nil
	}
}
