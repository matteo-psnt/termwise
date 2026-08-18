package agentui

import tea "github.com/charmbracelet/bubbletea"

// commandProposalMode is active when the agent has proposed a final shell
// command and is waiting for the user to accept or dismiss it.
type commandProposalMode struct{}

func (commandProposalMode) handleKey(m Model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEnter:
		return m.acceptCommandProposal()
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
		return m.acceptCommandProposal()
	case "2":
		return m.dismissCommandProposal()
	}
	return m, nil
}
