package agentui

import tea "github.com/charmbracelet/bubbletea"

// binding is one key + description pair shown in the help overlay.
type binding struct {
	key  string
	desc string
}

// mode owns key handling, input-row rendering, and the help binding list for
// one TUI screen. Modes are exclusive: exactly one is active at a time,
// selected via Model.state.
type mode interface {
	handleKey(m Model, msg tea.KeyMsg) (tea.Model, tea.Cmd)
	renderInputRow(m Model, vpW int) string
	helpBindings(m Model) []binding
}

// Compile-time assertions that each concrete mode satisfies the interface.
var (
	_ mode = idleMode{}
	_ mode = thinkingMode{}
	_ mode = approvalMode{}
	_ mode = commandProposalMode{}
	_ mode = commandProposalEditMode{}
	_ mode = askPickerMode{}
	_ mode = slashPickerMode{}
	_ mode = configEditorMode{}
	_ mode = histSearchMode{}
)
