package agentui

import tea "github.com/charmbracelet/bubbletea"

// thinkingMode covers stateThinking and stateJudging — waiting on the model
// or on the LLM safety judge. Only scroll keys and Esc-to-interrupt are accepted.
type thinkingMode struct{}

func (thinkingMode) handleKey(m Model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyPgUp:
		m.vp.PageUp()
		m.userScrolled = !m.vp.AtBottom()
		return m, nil
	case tea.KeyPgDown:
		m.vp.PageDown()
		m.userScrolled = !m.vp.AtBottom()
		return m, nil
	case tea.KeyEnd:
		m.userScrolled = false
		m.vp.GotoBottom()
		return m, nil
	case tea.KeyEsc:
		m.interruptActiveTurn()
		return m, nil
	}
	return m, nil
}

func (thinkingMode) renderInputRow(m Model, _ int) string {
	if m.state == stateJudging {
		cmd, _ := m.pending.toolCall.Input["command"].(string)
		return " " + m.renderer.styles.Spinner.Render(m.spin.View()) + " " + cmd
	}
	return " " + m.renderer.styles.Spinner.Render(m.spin.View()) + " thinking..."
}

func (thinkingMode) helpBindings(_ Model) []binding {
	return []binding{
		{"esc", "interrupt"},
		{"?", "close help"},
		{"ctrl+c", "quit"},
	}
}
