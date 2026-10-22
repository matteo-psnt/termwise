package agentui

import (
	"reflect"
	"testing"

	"github.com/matteo-psnt/termwise/internal/history"
)

func TestSerializeThreadSkipsSystemEntries(t *testing.T) {
	thread := []ThreadEntry{
		SystemEntry{Content: "Config warnings:\nbad key"},
		UserEntry{Content: "hi"},
		SystemEntry{Content: "Theme set to dark."},
		AssistantEntry{Content: "hello"},
		ToolCallEntry{Name: "bash", Detail: "ls"},
		ToolResultEntry{Content: "a\nb", IsError: true},
		CommandEntry{Content: "ls -la"},
		ErrorEntry{Content: "boom"},
	}
	want := []ThreadEntry{
		UserEntry{Content: "hi"},
		AssistantEntry{Content: "hello"},
		ToolCallEntry{Name: "bash", Detail: "ls"},
		ToolResultEntry{Content: "a\nb", IsError: true},
		CommandEntry{Content: "ls -la"},
		ErrorEntry{Content: "boom"},
	}
	got := deserializeThread(serializeThread(thread))
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("round trip:\n got %#v\nwant %#v", got, want)
	}
}

// Sessions saved before system entries were skipped hold a placeholder error
// for each of them; they must still load, without the placeholders.
func TestDeserializeThreadDropsLegacyUnknownPlaceholder(t *testing.T) {
	old := []history.SerializedEntry{
		{Type: history.EntryTypeError, Content: legacyUnknownEntry},
		{Type: history.EntryTypeUser, Content: "hi"},
		{Type: history.EntryTypeError, Content: "real error"},
	}
	want := []ThreadEntry{UserEntry{Content: "hi"}, ErrorEntry{Content: "real error"}}
	if got := deserializeThread(old); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}
