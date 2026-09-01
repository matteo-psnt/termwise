package agentui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// commandProposalMode is active when the agent has proposed a final shell
// command and is waiting for the user to accept or dismiss it.
type commandProposalMode struct{}

func (commandProposalMode) handleKey(m Model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEnter:
		return m.quit()
	case tea.KeyEsc:
		return m.dismissCommandProposal()
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
	}
	switch msg.String() {
	case "1":
		return m.quit()
	case "2":
		return m.dismissCommandProposal()
	case "3", "e":
		m.input.SetValue(m.shellCommand)
		m.input.CursorEnd()
		m.state = stateCommandProposalEdit
		return m, nil
	}
	return m, nil
}

func (commandProposalMode) renderInputRow(m Model, vpW int) string {
	return lipgloss.JoinVertical(lipgloss.Left,
		m.renderCommandBlock("suggested command", "$ "+m.shellCommand, vpW),
		m.renderer.styles.ActionHints.Render("  [↵] accept   [esc] dismiss   [e] edit"),
	)
}

func (commandProposalMode) helpBindings(_ Model) []binding {
	return []binding{
		{"↵", "accept command"},
		{"esc", "dismiss"},
		{"e", "edit command"},
		{"?", "close help"},
		{"ctrl+c", "quit"},
	}
}
