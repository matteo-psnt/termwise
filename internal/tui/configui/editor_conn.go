package configui

import (
	"context"
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/matteo-psnt/termwise/internal/config"
)

// resetConnectivity starts a new check: results of earlier ones still in
// flight no longer match connSeq and are dropped.
func (m *editorModel) resetConnectivity() {
	m.connChecked = false
	m.connOK = false
	m.connSeq++
}

func (m editorModel) checkConnectivityCmd() tea.Cmd {
	providerName := m.cfg.SelectedProvider
	seq := m.connSeq
	pc, ok := m.cfg.Providers[providerName]
	if !ok {
		return func() tea.Msg {
			return editorConnMsg{provider: providerName, seq: seq, err: fmt.Errorf("provider not configured")}
		}
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		err := config.CheckProviderConnectivity(ctx, providerName, pc)
		return editorConnMsg{provider: providerName, seq: seq, err: err}
	}
}

func (m editorModel) connIndicator() string {
	if !m.connChecked {
		return m.styles.Spinner.Render(m.spin.View())
	}
	if m.connOK {
		return m.styles.Success.Render("●")
	}
	return m.styles.Error.Render("●")
}
