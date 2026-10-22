package configui

import (
	"context"
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/matteo-psnt/termwise/internal/config"
)

func (m *editorModel) resetConnectivity() {
	m.connChecked = false
	m.connOK = false
}

func (m editorModel) checkConnectivityCmd() tea.Cmd {
	providerName := m.cfg.SelectedProvider
	pc, ok := m.cfg.Providers[providerName]
	if !ok {
		return func() tea.Msg {
			return editorConnMsg{err: fmt.Errorf("provider not configured")}
		}
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := config.CheckProviderConnectivity(ctx, providerName, pc); err != nil {
			return editorConnMsg{err: err}
		}
		return editorConnMsg{}
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
