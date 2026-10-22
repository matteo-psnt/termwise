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

func TestListWindow(t *testing.T) {
	tests := []struct {
		n, cursor, size, start, end int
	}{
		{3, 0, 10, 0, 3},
		{50, 0, 10, 0, 10},
		{50, 4, 10, 0, 10},
		{50, 20, 10, 15, 25},
		{50, 49, 10, 40, 50},
	}
	for _, tt := range tests {
		start, end := listWindow(tt.n, tt.cursor, tt.size)
		if start != tt.start || end != tt.end {
			t.Errorf("listWindow(%d, %d, %d) = [%d, %d), want [%d, %d)", tt.n, tt.cursor, tt.size, start, end, tt.start, tt.end)
		}
		if tt.cursor < start || tt.cursor >= end {
			t.Errorf("listWindow(%d, %d, %d): cursor outside window", tt.n, tt.cursor, tt.size)
		}
	}
}
