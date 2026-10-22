package tty

import (
	"testing"
)

func TestVisibleControls(t *testing.T) {
	tests := map[string]string{
		"plain\ttext\nline":  "plain\ttext\nline",
		"a\r\nb":             "a\nb",
		"\x1b[2Jcleared":     "␛[2Jcleared",
		"\x1b]52;c;aGk=\x07": "␛]52;c;aGk=␇",
		"over\rwrite":        "over␍write",
		"del\x7f":            "del␡",
		"c1\u009b31m":        "c1�31m",
		"émoji 😀":            "émoji 😀",
	}
	for in, want := range tests {
		if got := VisibleControls(in); got != want {
			t.Errorf("VisibleControls(%q) = %q, want %q", in, got, want)
		}
	}
}
