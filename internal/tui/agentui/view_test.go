package agentui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

// Every row of the box must be exactly the requested width, or the border
// breaks. Long pipelines are the normal case for a proposed command.
func TestCommandBlockRowsAreAllTheSameWidth(t *testing.T) {
	m := Model{}
	m.renderer = testRenderer()
	cases := map[string]string{
		"short":     "$ ls -la",
		"long pipe": "$ find . -type f -exec du -h {} + | sort -hr | head -20 | awk '{print $2}' | xargs wc -l",
		"multiline": "$ git add -A \\\n  && git commit -m 'a message'",
		"unbroken":  "$ curl https://example.com/" + strings.Repeat("a", 120),
		"empty":     "",
	}
	for name, content := range cases {
		out := m.renderCommandBlock("suggested command", content, 60)
		for i, line := range strings.Split(out, "\n") {
			if w := lipgloss.Width(line); w != 60 {
				t.Errorf("%s: line %d width = %d, want 60\n%q", name, i, w, line)
			}
		}
	}
}

func TestCommandBlockKeepsTheWholeCommand(t *testing.T) {
	m := Model{}
	m.renderer = testRenderer()
	cmd := "find . -type f -exec du -h {} + | sort -hr | head -20 | awk '{print $2}'"
	out := m.renderCommandBlock("cmd", "$ "+cmd, 60)

	// Strip borders and rejoin; every token must survive the wrap.
	var body []string
	for _, line := range strings.Split(out, "\n")[1:] {
		if strings.HasPrefix(strings.TrimSpace(stripANSI(line)), "╰") {
			break
		}
		body = append(body, strings.TrimSpace(strings.Trim(stripANSI(line), "│")))
	}
	joined := strings.Join(body, " ")
	for _, token := range strings.Fields(cmd) {
		if !strings.Contains(joined, token) {
			t.Errorf("token %q lost in wrapping\ngot: %s", token, joined)
		}
	}
}

func TestCommandBlockWrapsRatherThanOverflowing(t *testing.T) {
	m := Model{}
	m.renderer = testRenderer()
	long := "$ " + strings.Repeat("echo hello | ", 12)
	out := m.renderCommandBlock("cmd", long, 60)
	if n := len(strings.Split(out, "\n")); n < 4 {
		t.Errorf("long command produced %d lines; expected it to wrap onto several", n)
	}
}

func TestWrapToWidthHardBreaksUnbrokenTokens(t *testing.T) {
	segs := wrapToWidth(strings.Repeat("x", 50), 10)
	if len(segs) < 5 {
		t.Fatalf("got %d segments for a 50-char token at width 10", len(segs))
	}
	for i, s := range segs {
		if lipgloss.Width(s) > 10 {
			t.Errorf("segment %d is %d wide, want <= 10: %q", i, lipgloss.Width(s), s)
		}
	}
}

func TestWrapToWidthLeavesShortLinesAlone(t *testing.T) {
	if got := wrapToWidth("ls -la", 40); len(got) != 1 || got[0] != "ls -la" {
		t.Errorf("wrapToWidth() = %q", got)
	}
}
