package configui

import (
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/matteo-psnt/termwise/internal/config"
	"github.com/matteo-psnt/termwise/internal/theme"
)

// Open launches the config TUI.
// If no config exists it runs the first-run wizard and saves the result.
// If a config exists it runs the editor.
func Open(cfgPath string, cfg config.FileConfig, exists bool) error {
	hasDarkBg := lipgloss.HasDarkBackground(os.Stdin, os.Stdout)

	if !exists {
		return runWizard(cfgPath, hasDarkBg)
	}
	return runEditor(cfgPath, cfg, hasDarkBg)
}

func runWizard(cfgPath string, hasDarkBg bool) error {
	m := newWizardModel(hasDarkBg, theme.DefaultName)
	p := tea.NewProgram(m)
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

func runEditor(cfgPath string, cfg config.FileConfig, hasDarkBg bool) error {
	m := newEditorModel(cfgPath, cfg, hasDarkBg)
	p := tea.NewProgram(m)
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
