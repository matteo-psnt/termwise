package agentui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

// commandProposalEditMode is active while the user is editing the model's
// proposed shell command before accepting it. Enter commits the edited value
// to m.shellCommand and accepts; Esc reverts to the proposal screen.
type commandProposalEditMode struct{}

var commandProposalEditKeys = keymap{
	{keys: []string{"enter"}, label: "↵", desc: "accept command", run: Model.acceptCommandEdit},
	{keys: []string{"esc"}, label: "esc", desc: "back", run: Model.cancelCommandEdit},
}

func (commandProposalEditMode) handleKey(m Model, msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if newM, cmd, ok := commandProposalEditKeys.handle(m, msg); ok {
		return newM, cmd
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

// acceptCommandEdit commits the edited input as the shell command and accepts.
func (m Model) acceptCommandEdit() (tea.Model, tea.Cmd) {
	edited := strings.TrimSpace(m.input.Value())
	m.input.SetValue("")
	if edited != "" {
		m.setShellCommand(edited)
	}
	return m.quit()
}

// cancelCommandEdit discards the edit and returns to the proposal screen.
func (m Model) cancelCommandEdit() (tea.Model, tea.Cmd) {
	m.input.SetValue("")
	m.state = stateCommandProposal
	return m, nil
}

func (commandProposalEditMode) renderInputRow(m Model, _ int) string {
	return m.renderer.styles.InputPrompt.Render(" ✎ $ ") + m.input.View()
}

func (commandProposalEditMode) helpBindings(_ Model) []binding {
	return commandProposalEditKeys.help()
}
