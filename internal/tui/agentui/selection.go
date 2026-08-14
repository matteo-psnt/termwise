package agentui

import (
	"os/exec"
	"runtime"
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
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
func applyInverse(line string, colStart, colEnd int) string {
	var b strings.Builder
	col := 0
	inverted := false
	for i := 0; i < len(line); {
		if line[i] == '\x1b' {
			end := i + 1
			for end < len(line) && !isANSITerminator(line[end]) {
				end++
			}
			if end < len(line) {
				end++
			}
			b.WriteString(line[i:end])
			if inverted {
				b.WriteString("\x1b[7m")
			}
			i = end
			continue
		}
		r, size := utf8.DecodeRuneInString(line[i:])
		w := lipgloss.Width(string(r))
		if !inverted && col >= colStart && col < colEnd {
			b.WriteString("\x1b[7m")
			inverted = true
		}
		if inverted && col >= colEnd {
			b.WriteString("\x1b[27m")
			inverted = false
		}
		b.WriteString(line[i : i+size])
		col += w
		i += size
	}
	if inverted {
		b.WriteString("\x1b[27m")
	}
	return b.String()
}

func isANSITerminator(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}

func stripANSI(s string) string {
	return ansiEscape.ReplaceAllString(s, "")
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
	var b strings.Builder
	col := 0
	for i := 0; i < len(plain); {
		r, size := utf8.DecodeRuneInString(plain[i:])
		w := lipgloss.Width(string(r))
		if col >= colStart && col < colEnd {
			b.WriteString(plain[i : i+size])
		}
		col += w
		i += size
		if col >= colEnd {
			break
		}
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
