package configtui

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/matteo-psnt/termwise/internal/config"
)

// Open launches the config TUI.
// If no config exists it runs the first-run wizard and saves the result.
// If a config exists it runs the editor.
func Open(cfgPath string, cfg config.Config, exists bool) error {
	r := lipgloss.NewRenderer(os.Stdout)

	if !exists {
		return runWizard(cfgPath, r)
	}
	return runEditor(cfgPath, cfg, r)
}

func runWizard(cfgPath string, r *lipgloss.Renderer) error {
	m := newWizardModel(r)
	p := tea.NewProgram(m, tea.WithAltScreen())
	final, err := p.Run()
	if err != nil {
		return fmt.Errorf("config wizard: %w", err)
	}
	wm, ok := final.(wizardModel)
	if !ok || wm.Result == nil {
		// User quit without completing.
		return nil
	}
	if err := config.SaveConfig(cfgPath, *wm.Result); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}
	return nil
}

func runEditor(cfgPath string, cfg config.Config, r *lipgloss.Renderer) error {
	m := newEditorModel(cfgPath, cfg, r)
	p := tea.NewProgram(m, tea.WithAltScreen())
	final, err := p.Run()
	if err != nil {
		return fmt.Errorf("config editor: %w", err)
	}
	em, ok := final.(editorModel)
	if !ok {
		return nil
	}
	if em.err != nil {
		return em.err
	}
	return nil
}
