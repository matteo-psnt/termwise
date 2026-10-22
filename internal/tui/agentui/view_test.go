package agentui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/matteo-psnt/termwise/internal/theme"
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

func TestFormatTokens(t *testing.T) {
	for n, want := range map[int]string{
		0:          "0",
		999:        "999",
		1000:       "1,000",
		1001:       "1,001",
		999_999:    "999,999",
		1_000_000:  "1,000,000",
		1_234_567:  "1,234,567",
		12_000_050: "12,000,050",
	} {
		if got := formatTokens(n); got != want {
			t.Errorf("formatTokens(%d) = %q, want %q", n, got, want)
		}
	}
}

// layoutModel is a sized, ready Model built the way Open builds one.
func layoutModel(t *testing.T) Model {
	t.Helper()
	m := newModel(modelConfig{providerName: "anthropic", modelID: "claude-sonnet-4-6", themeName: theme.DefaultName, initialCursorRow: -1})
	next, _ := m.handleWindowSize(tea.WindowSizeMsg{Width: 80, Height: 40})
	return next.(Model)
}

// totalViewHeight drives the anchor and mouse-row math, so it must agree with
// what renderedView draws, including states whose input row is a textarea
// that has grown to several lines, and the help overlay.
func TestTotalViewHeightMatchesRenderedView(t *testing.T) {
	multi := "first line\nsecond line\nthird line"
	cases := map[string]func(Model) Model{
		"idle multi-line": func(m Model) Model { m.input.SetValue(multi); return m },
		"proposal edit multi-line": func(m Model) Model {
			m.state = stateCommandProposalEdit
			m.input.SetValue(multi)
			return m
		},
		"hist search multi-line": func(m Model) Model {
			m.state = stateHistSearch
			m.input.SetValue(multi)
			return m
		},
		"help overlay": func(m Model) Model { m.showHelp = true; return m },
	}
	for name, setup := range cases {
		m := setup(layoutModel(t))
		if got, want := m.totalViewHeight(), lipgloss.Height(m.renderedView()); got != want {
			t.Errorf("%s: totalViewHeight = %d, rendered %d rows", name, got, want)
		}
	}
}
