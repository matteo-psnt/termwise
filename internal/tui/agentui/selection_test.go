package agentui

import (
	"strings"
	"testing"
)

// oscLine is "link tail" with "link" wrapped in an OSC 8 hyperlink, the way
// glamour may render a markdown link.
const oscLine = "\x1b]8;;https://example.com/path\x1b\\link\x1b]8;;\x1b\\ tail"

// The OSC parameter is invisible. Counting its bytes as columns moved the
// highlight off the selected text.
func TestApplyInverseSkipsOSCSequences(t *testing.T) {
	out := applyInverse(oscLine, 0, 4)
	if !strings.Contains(out, "\x1b[7mlink") {
		t.Errorf("inverse does not start at the selected text: %q", out)
	}
	if !strings.Contains(out, "\x1b[27m tail") {
		t.Errorf("inverse does not end after the selected text: %q", out)
	}
	if !strings.Contains(out, "\x1b]8;;https://example.com/path\x1b\\") {
		t.Errorf("hyperlink escape was split: %q", out)
	}
}

func TestExtractSelectionSkipsOSCSequences(t *testing.T) {
	got := extractSelection("first\n"+oscLine, selectionState{startLine: 1, startCol: 2, endLine: 1, endCol: 7})
	if got != "nk ta" {
		t.Errorf("extractSelection = %q, want %q", got, "nk ta")
	}
}

// A family emoji is several runes joined into one two-cell grapheme; stepping
// by rune made the columns disagree with lipgloss.Width.
func TestExtractSelectionStepsByGrapheme(t *testing.T) {
	line := "a👨‍👩‍👧b"
	got := extractSelection(line, selectionState{startLine: 0, startCol: 3, endLine: 0, endCol: 4})
	if got != "b" {
		t.Errorf("extractSelection = %q, want %q", got, "b")
	}
}
