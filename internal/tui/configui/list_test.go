package configui

import "testing"

func TestMoveListCursor(t *testing.T) {
	tests := []struct {
		key       string
		cursor, n int
		want      int
		ok        bool
	}{
		{"down", 0, 3, 1, true},
		{"j", 1, 3, 2, true},
		{"down", 2, 3, 2, true}, // stops at the end, no wraparound
		{"up", 2, 3, 1, true},
		{"k", 0, 3, 0, true}, // stops at the top
		{"down", 0, 0, 0, true},
		{"enter", 1, 3, 1, false},
	}
	for _, tt := range tests {
		got, ok := moveListCursor(tt.key, tt.cursor, tt.n)
		if got != tt.want || ok != tt.ok {
			t.Errorf("moveListCursor(%q, %d, %d) = %d, %v; want %d, %v", tt.key, tt.cursor, tt.n, got, ok, tt.want, tt.ok)
		}
	}
}
