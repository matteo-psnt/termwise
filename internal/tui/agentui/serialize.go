package agentui

import "github.com/matteo-psnt/termwise/internal/history"

// legacyUnknownEntry is what serializeEntry used to write for any entry type it
// didn't know, which in practice meant every SystemEntry. Sessions saved before
// system entries were skipped still carry it; dropping it on load stops them
// resuming with a column of "Error: [unknown entry type]".
const legacyUnknownEntry = "[unknown entry type]"

// serializeThread converts a []ThreadEntry to []history.SerializedEntry.
func serializeThread(entries []ThreadEntry) []history.SerializedEntry {
	out := make([]history.SerializedEntry, 0, len(entries))
	for _, e := range entries {
		if se, ok := serializeEntry(e); ok {
			out = append(out, se)
		}
	}
	return out
}

// serializeEntry reports false for entries that are not persisted.
//
// SystemEntry is deliberately one of them: it is termwise talking about the
// session, not part of it. Config warnings and the env-only notice are
// re-derived and appended on every open, so persisting them would show each one
// twice after a resume (and keep showing warnings already fixed); /help output
// and "Theme set to …" are acknowledgements that mean nothing on a later run.
func serializeEntry(e ThreadEntry) (history.SerializedEntry, bool) {
	switch v := e.(type) {
	case UserEntry:
		return history.SerializedEntry{Type: history.EntryTypeUser, Content: v.Content}, true
	case AssistantEntry:
		return history.SerializedEntry{Type: history.EntryTypeAssistant, Content: v.Content}, true
	case ToolCallEntry:
		return history.SerializedEntry{Type: history.EntryTypeToolCall, Name: v.Name, Detail: v.Detail}, true
	case ToolResultEntry:
		return history.SerializedEntry{Type: history.EntryTypeToolResult, Content: v.Content, IsError: v.IsError}, true
	case CommandEntry:
		return history.SerializedEntry{Type: history.EntryTypeCommand, Content: v.Content}, true
	case ErrorEntry:
		return history.SerializedEntry{Type: history.EntryTypeError, Content: v.Content}, true
	default:
		return history.SerializedEntry{}, false
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
		if e.Content == legacyUnknownEntry {
			return nil
		}
		return ErrorEntry{Content: e.Content}
	default:
		return nil
	}
}
