package agentui

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/matteo-psnt/termwise/internal/agent/tools"
	"github.com/matteo-psnt/termwise/internal/allowlist"
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

	rows := []string{m.renderCommandBlock("run command?", cmd, vpW)}
	// Say why this one stopped, so approving is a judgement rather than a reflex.
	if reason := allowlist.ApprovalReason(cmd); reason != "" {
		rows = append(rows, m.renderer.styles.ActionHints.Render("   "+reason))
	}
	rows = append(rows, approvalKeys.hints(m))

	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}

func (approvalMode) helpBindings(_ Model) []binding {
	return approvalKeys.help()
}
