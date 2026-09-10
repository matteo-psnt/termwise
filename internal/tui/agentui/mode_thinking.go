package agentui

import (
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/matteo-psnt/termwise/internal/agent/tools"
)

// thinkingMode covers stateThinking and stateJudging — waiting on the model
// or on the LLM safety judge. Only scroll keys and Esc-to-interrupt are accepted.
type thinkingMode struct{}

var thinkingKeys = append(keymap{
	{keys: []string{"esc"}, label: "esc", desc: "interrupt", run: Model.interruptTurn},
}, scrollKeys...)

func (thinkingMode) handleKey(m Model, msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	newM, cmd, _ := thinkingKeys.handle(m, msg)
	return newM, cmd
}

// interruptTurn cancels the in-flight model turn, adapting interruptActiveTurn
// to the keymap action signature.
func (m Model) interruptTurn() (tea.Model, tea.Cmd) {
	m.interruptActiveTurn()
	return m, nil
}

func (thinkingMode) renderInputRow(m Model, vpW int) string {
	spin := m.renderer.styles.Spinner.Render(m.spin.View())

	label := "thinking..."
	switch {
	case m.state == stateJudging:
		label = "checking safety of " + tools.BashCommand(m.pending.toolCall)
	case m.activity != "":
		label = m.activity
	}

	elapsed := m.renderer.styles.ActionHints.Render(formatElapsed(m.turnStartedAt))
	// Keep the elapsed counter from pushing the row past the viewport width.
	room := max(vpW-lipgloss.Width(spin)-lipgloss.Width(elapsed)-4, 8)
	if lipgloss.Width(label) > room {
		label = truncateLabel(label, room)
	}
	return " " + spin + " " + label + "  " + elapsed
}

// formatElapsed reports how long the current turn has been running. It stays
// empty for the first couple of seconds so quick turns do not flash a counter.
func formatElapsed(start time.Time) string {
	if start.IsZero() {
		return ""
	}
	d := time.Since(start)
	if d < 2*time.Second {
		return ""
	}
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	return fmt.Sprintf("%dm%02ds", int(d.Minutes()), int(d.Seconds())%60)
}

func truncateLabel(s string, width int) string {
	r := []rune(s)
	if width < 2 || len(r) <= width {
		return s
	}
	return string(r[:width-1]) + "…"
}

func (thinkingMode) helpBindings(_ Model) []binding {
	return thinkingKeys.help()
}
