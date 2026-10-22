package agentui

import (
	"os/exec"
	"regexp"
	"runtime"
	"strings"

	"charm.land/lipgloss/v2"
)

// urlRegex matches bare http(s) URLs, stopping at whitespace, ANSI escapes,
// or common boundary punctuation.
var urlRegex = regexp.MustCompile(`https?://[^\s\x1b"'<>()\[\]]+`)

// urlTrailingPunct is trimmed from the end of a matched URL.
const urlTrailingPunct = ".,;:!?"

// ansiEscape matches ANSI CSI sequences for stripping styling before column math.
var ansiEscape = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

// linkPosition describes one URL occurrence within rendered viewport content.
type linkPosition struct {
	URL    string
	Line   int // 0-indexed line in the full content (not viewport-relative)
	StartX int // 0-indexed visible column where the URL starts
	EndX   int // exclusive visible column
}

// findLinks scans the rendered content (joined with \n) and returns every
// URL occurrence's location, indexed by content line and visible column.
func findLinks(content string) []linkPosition {
	if content == "" {
		return nil
	}
	var out []linkPosition
	lineIdx := 0
	start := 0
	for i := 0; i <= len(content); i++ {
		if i == len(content) || content[i] == '\n' {
			line := content[start:i]
			plain := stripANSI(line)
			for _, idx := range urlRegex.FindAllStringIndex(plain, -1) {
				// Sentence punctuation after a URL ("see https://x.dev.") is
				// not part of it; opening it with the dot attached 404s.
				url := strings.TrimRight(plain[idx[0]:idx[1]], urlTrailingPunct)
				out = append(out, linkPosition{
					URL:    url,
					Line:   lineIdx,
					StartX: lipgloss.Width(plain[:idx[0]]),
					EndX:   lipgloss.Width(plain[:idx[0]+len(url)]),
				})
			}
			lineIdx++
			start = i + 1
		}
	}
	return out
}

// openURL opens url in the system default browser. Best-effort; errors are
// swallowed by the caller since opening a URL is a side-effect-only action.
func openURL(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		return nil
	}
	return cmd.Start()
}
