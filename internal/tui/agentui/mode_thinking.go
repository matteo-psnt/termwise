package agentui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/matteo-psnt/termwise/internal/agent/tools"
)

// thinkingMode covers stateThinking and stateJudging — waiting on the model
// or on the LLM safety judge. Only scroll keys and Esc-to-interrupt are accepted.
type thinkingMode struct{}

var thinkingKeys = append(keymap{
	{keys: []string{"esc"}, label: "esc", desc: "interrupt", run: Model.interruptTurn},
}, scrollKeys...)

func (thinkingMode) handleKey(m Model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	newM, cmd, _ := thinkingKeys.handle(m, msg)
	return newM, cmd
}

// interruptTurn cancels the in-flight model turn, adapting interruptActiveTurn
// to the keymap action signature.
func (m Model) interruptTurn() (tea.Model, tea.Cmd) {
	m.interruptActiveTurn()
	return m, nil
}

func (thinkingMode) renderInputRow(m Model, _ int) string {
	if m.state == stateJudging {
		cmd := tools.BashCommand(m.pending.toolCall)
		return " " + m.renderer.styles.Spinner.Render(m.spin.View()) + " " + cmd
	}
	return " " + m.renderer.styles.Spinner.Render(m.spin.View()) + " thinking..."
}

func (thinkingMode) helpBindings(_ Model) []binding {
	return thinkingKeys.help()
}
