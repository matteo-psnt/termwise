package agentui

import (
	"os/exec"
	"runtime"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// selectionState tracks an in-progress or completed mouse text selection.
// Coordinates are TUI-block-row indices (0 = header) and visible columns.
type selectionState struct {
	active              bool
	startLine, startCol int
	endLine, endCol     int
}

// has reports whether the selection covers at least one cell.
func (s selectionState) has() bool {
	if !s.active && s.startLine == 0 && s.endLine == 0 && s.startCol == 0 && s.endCol == 0 {
		return false
	}
	a, b := s.normalize()
	return a.line != b.line || a.col != b.col
}

type selPoint struct{ line, col int }

// normalize returns (start, end) in document order regardless of drag direction.
func (s selectionState) normalize() (selPoint, selPoint) {
	a := selPoint{s.startLine, s.startCol}
	b := selPoint{s.endLine, s.endCol}
	if b.line < a.line || (b.line == a.line && b.col < a.col) {
		return b, a
	}
	return a, b
}

// applySelectionToView applies inverse-video to every cell inside `sel` on
// the composed View output. Any row of the TUI block can be highlighted.
func applySelectionToView(view string, sel selectionState) string {
	if !sel.has() {
		return view
	}
	a, b := sel.normalize()
	lines := strings.Split(view, "\n")
	for i, line := range lines {
		if i < a.line || i > b.line {
			continue
		}
		colStart := 0
		colEnd := lipgloss.Width(stripANSI(line))
		if i == a.line {
			colStart = a.col
		}
		if i == b.line {
			colEnd = b.col
		}
		if colStart >= colEnd {
			continue
		}
		lines[i] = applyInverse(line, colStart, colEnd)
	}
	return strings.Join(lines, "\n")
}

// applyInverse wraps visible columns [colStart, colEnd) of `line` with
// inverse-video, re-arming \x1b[7m after every escape so resets like \x1b[0m
// don't silently drop the attribute mid-selection.
//
// Escapes are decoded with x/ansi rather than by scanning to the next letter:
// that only understood CSI, so an OSC 8 hyperlink (ESC ] 8 ;; url ESC \) from
// glamour ended at the first letter of the URL and the rest counted as
// visible columns, shifting the highlight off the selected text.
func applyInverse(line string, colStart, colEnd int) string {
	var b strings.Builder
	col := 0
	inverted := false
	var state byte
	for rest := line; rest != ""; {
		seq, w, n, newState := ansi.DecodeSequence(rest, state, nil)
		state = newState
		rest = rest[n:]
		if isEscapeSequence(seq) {
			b.WriteString(seq)
			if inverted {
				b.WriteString("\x1b[7m")
			}
			continue
		}
		if !inverted && col >= colStart && col < colEnd {
			b.WriteString("\x1b[7m")
			inverted = true
		}
		if inverted && col >= colEnd {
			b.WriteString("\x1b[27m")
			inverted = false
		}
		b.WriteString(seq)
		col += w
	}
	if inverted {
		b.WriteString("\x1b[27m")
	}
	return b.String()
}

// isEscapeSequence reports whether a DecodeSequence result is an escape
// sequence (CSI, OSC, DCS, …) rather than text.
func isEscapeSequence(seq string) bool {
	return seq != "" && (seq[0] == ansi.ESC || seq[0] >= 0x80 && seq[0] <= 0x9f)
}

// stripANSI removes every escape sequence, OSC included, so what remains can
// be measured in columns.
func stripANSI(s string) string {
	return ansi.Strip(s)
}

// extractSelection returns the plain (de-styled) text of the selection
// against the full content (joined by \n).
func extractSelection(content string, sel selectionState) string {
	if !sel.has() {
		return ""
	}
	a, b := sel.normalize()
	lines := strings.Split(content, "\n")
	var out []string
	for i := a.line; i <= b.line && i < len(lines); i++ {
		plain := stripANSI(lines[i])
		colStart := 0
		colEnd := lipgloss.Width(plain)
		if i == a.line {
			colStart = a.col
		}
		if i == b.line {
			colEnd = b.col
		}
		out = append(out, sliceByCol(plain, colStart, colEnd))
	}
	return strings.Join(out, "\n")
}

// sliceByCol returns the substring of plain (already ANSI-free) that occupies
// visible columns [colStart, colEnd).
func sliceByCol(plain string, colStart, colEnd int) string {
	if colStart >= colEnd {
		return ""
	}
	// Step by grapheme, not rune, so the columns agree with lipgloss.Width:
	// an emoji built from several runes is one two-cell character.
	var b strings.Builder
	col := 0
	var state byte
	for rest := plain; rest != "" && col < colEnd; {
		seq, w, n, newState := ansi.DecodeSequence(rest, state, nil)
		state = newState
		rest = rest[n:]
		if col >= colStart {
			b.WriteString(seq)
		}
		col += w
	}
	return b.String()
}

// copyToClipboard writes text to the OS clipboard via the platform-appropriate
// CLI tool. Falls back silently when no tool is available.
func copyToClipboard(text string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("pbcopy")
	case "linux":
		if _, err := exec.LookPath("wl-copy"); err == nil {
			cmd = exec.Command("wl-copy")
		} else if _, err := exec.LookPath("xclip"); err == nil {
			cmd = exec.Command("xclip", "-selection", "clipboard")
		} else {
			return nil
		}
	case "windows":
		cmd = exec.Command("clip")
	default:
		return nil
	}
	cmd.Stdin = strings.NewReader(text)
	return cmd.Run()
}
