package agentui

import tea "github.com/charmbracelet/bubbletea"

// mode owns key handling for one TUI screen. Modes are exclusive: exactly one
// is active at a time, selected via Model.state during the 3b→3d migration and
// directly afterward.
//
// During the migration, modes may delegate to the existing handle*Key methods
// on Model; once a mode owns its body it lives in its own file.
type mode interface {
	handleKey(m Model, msg tea.KeyMsg) (tea.Model, tea.Cmd)
}

// Compile-time assertions that each concrete mode satisfies the interface.
var (
	_ mode = idleMode{}
	_ mode = thinkingMode{}
	_ mode = approvalMode{}
	_ mode = approvalEditMode{}
	_ mode = commandProposalMode{}
	_ mode = askPickerMode{}
	_ mode = slashPickerMode{}
	_ mode = histSearchMode{}
)

// idleMode handles input when no modal screen is open.
type idleMode struct{}

func (idleMode) handleKey(m Model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	return m.handleIdleKey(msg)
}

// thinkingMode covers stateThinking and stateJudging — waiting on the model
// or on the LLM safety judge. The TUI only accepts scroll keys and Esc.
type thinkingMode struct{}

func (thinkingMode) handleKey(m Model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	return m.handleThinkingKey(msg)
}

// approvalMode handles input when a bash command needs user approval.
type approvalMode struct{}

func (approvalMode) handleKey(m Model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	return m.handleApprovalKey(msg)
}

// approvalEditMode handles input while the user is editing a proposed bash
// command before approving.
type approvalEditMode struct{}

func (approvalEditMode) handleKey(m Model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	return m.handleApprovalEditKey(msg)
}

// commandProposalMode handles input when the agent has proposed a final shell
// command; the user can accept or dismiss it.
type commandProposalMode struct{}

func (commandProposalMode) handleKey(m Model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	return m.handleCommandProposalKey(msg)
}

// askPickerMode handles input while the agent's `ask` tool has surfaced a
// multiple-choice question.
type askPickerMode struct{}

func (askPickerMode) handleKey(m Model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	return m.handlePickerKey(msg)
}

// slashPickerMode handles input inside a slash-command sub-picker (e.g.
// /effort, /theme, /model).
type slashPickerMode struct{}

func (slashPickerMode) handleKey(m Model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	return m.handleSlashPickerKey(msg)
}

// histSearchMode handles Ctrl+R reverse search through the prompt history.
type histSearchMode struct{}

func (histSearchMode) handleKey(m Model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	return m.handleHistSearchKey(msg)
}
