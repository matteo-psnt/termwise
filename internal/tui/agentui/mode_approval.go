package agentui

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/matteo-psnt/termwise/internal/agent/tools"
)

// approvalMode is active when a bash command is awaiting user approval.
type approvalMode struct{}

var approvalKeys = keymap{
	{keys: []string{"enter", "1", "y"}, label: "↵", desc: "run command", run: Model.approvePendingBash},
	{keys: []string{"esc", "2", "n"}, label: "esc", desc: "skip", run: Model.denyPendingBash},
}

func (approvalMode) handleKey(m Model, msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	newM, cmd, _ := approvalKeys.handle(m, msg)
	return newM, cmd
}

func (approvalMode) renderInputRow(m Model, vpW int) string {
	cmd := tools.BashCommand(m.pending.toolCall)
	return lipgloss.JoinVertical(lipgloss.Left,
		m.renderCommandBlock("run command?", cmd, vpW),
		approvalKeys.hints(m),
	)
}

func (approvalMode) helpBindings(_ Model) []binding {
	return approvalKeys.help()
}
