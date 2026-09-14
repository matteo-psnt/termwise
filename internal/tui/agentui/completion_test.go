package agentui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestCompletionStateClamp(t *testing.T) {
	tests := []struct {
		cursor, n, want int
	}{
		{cursor: 3, n: 2, want: 1}, // list shrank under the cursor
		{cursor: 0, n: 0, want: 0}, // empty list must not go negative
		{cursor: -1, n: 5, want: 0},
		{cursor: 2, n: 5, want: 2},
	}
	for _, tc := range tests {
		c := completionState{cursor: tc.cursor}
		c.clamp(tc.n)
		if c.cursor != tc.want {
			t.Errorf("clamp(cursor=%d, n=%d) = %d, want %d", tc.cursor, tc.n, c.cursor, tc.want)
		}
	}
}

func TestCompletionStateNavigate(t *testing.T) {
	const n = 3
	tests := []struct {
		name    string
		key     tea.KeyPressMsg
		start   int
		want    int
		handled bool
	}{
		{name: "down", key: tea.KeyPressMsg{Code: tea.KeyDown}, start: 0, want: 1, handled: true},
		{name: "up", key: tea.KeyPressMsg{Code: tea.KeyUp}, start: 1, want: 0, handled: true},
		{name: "down stops at the end", key: tea.KeyPressMsg{Code: tea.KeyDown}, start: 2, want: 2, handled: true},
		{name: "up stops at the top", key: tea.KeyPressMsg{Code: tea.KeyUp}, start: 0, want: 0, handled: true},
		{name: "ctrl+n", key: tea.KeyPressMsg{Code: 'n', Mod: tea.ModCtrl}, start: 0, want: 1, handled: true},
		{name: "ctrl+p", key: tea.KeyPressMsg{Code: 'p', Mod: tea.ModCtrl}, start: 1, want: 0, handled: true},
		{name: "a letter is not navigation", key: tea.KeyPressMsg{Code: 'x'}, start: 1, want: 1, handled: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := completionState{cursor: tc.start}
			if got := c.navigate(tc.key, n); got != tc.handled {
				t.Errorf("navigate handled = %v, want %v", got, tc.handled)
			}
			if c.cursor != tc.want {
				t.Errorf("cursor = %d, want %d", c.cursor, tc.want)
			}
		})
	}
}
