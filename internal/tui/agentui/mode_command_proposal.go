package agentui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// commandProposalMode is active when the agent has proposed a final shell
// command and is waiting for the user to accept or dismiss it.
type commandProposalMode struct{}

var commandProposalKeys = append(keymap{
	{keys: []string{"enter", "1"}, label: "↵", desc: "accept command", run: Model.quit},
	{keys: []string{"esc", "2"}, label: "esc", desc: "dismiss", run: Model.dismissCommandProposal},
	{keys: []string{"3", "e"}, label: "e", desc: "edit command", run: Model.editCommandProposal},
}, scrollKeys...)

func (commandProposalMode) handleKey(m Model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	newM, cmd, _ := commandProposalKeys.handle(m, msg)
	return newM, cmd
}

func (commandProposalMode) renderInputRow(m Model, vpW int) string {
	return lipgloss.JoinVertical(lipgloss.Left,
		m.renderCommandBlock("suggested command", "$ "+m.shellCommand, vpW),
		commandProposalKeys.hints(m),
	)
}

func (commandProposalMode) helpBindings(_ Model) []binding {
	return commandProposalKeys.help()
}
