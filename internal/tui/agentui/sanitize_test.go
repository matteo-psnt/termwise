package agentui

import (
	"strings"
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
		if got := visibleControls(in); got != want {
			t.Errorf("visibleControls(%q) = %q, want %q", in, got, want)
		}
	}
}

// Text from outside termwise must not reach the terminal with live escape
// sequences: a tool result that cats a file with escape codes, a proposed
// command, model output.
func TestThreadAndCommandBoxNeutraliseEscapes(t *testing.T) {
	m := pickerModel("")
	r := m.renderer
	const hostile = "ok\x1b]52;c;ZXZpbA==\x07\x1b[2J"
	for name, out := range map[string]string{
		"tool result": ToolResultEntry{Content: hostile}.render(r),
		"command":     CommandEntry{Content: hostile}.render(r),
		"tool call":   ToolCallEntry{Name: "bash", Detail: hostile}.render(r),
		"assistant":   AssistantEntry{Content: hostile}.render(r),
		"command box": m.renderCommandBlock("cmd", hostile, 60),
	} {
		if strings.Contains(out, "\x1b]52") || strings.Contains(out, "\x1b[2J") || strings.Contains(out, "\x07") {
			t.Errorf("%s passed a raw escape sequence through: %q", name, out)
		}
	}
}
