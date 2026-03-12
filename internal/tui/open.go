package tui

import (
	"fmt"
	"io"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/matteo-psnt/termwise/internal/ai"
	"github.com/matteo-psnt/termwise/internal/systemprompt"
	"github.com/matteo-psnt/termwise/internal/tty"
)

// Open launches the agent TUI. It handles stdin pre-loading and TTY setup.
func Open(providerName string, provider ai.AgentProvider, modelID string) error {
	// Read stdin if piped.
	var stdin string
	if !tty.IsTerminal(os.Stdin) {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return fmt.Errorf("reading stdin: %w", err)
		}
		stdin = strings.TrimRight(string(data), "\n")
	}

	// Determine program input. When stdin is piped we need to reopen the TTY
	// so bubbletea can receive keyboard events.
	var programOpts []tea.ProgramOption
	programOpts = append(programOpts, tea.WithAltScreen())

	if !tty.IsTerminal(os.Stdin) {
		ttyFile, err := os.Open("/dev/tty")
		if err != nil {
			return fmt.Errorf("opening /dev/tty for keyboard input: %w", err)
		}
		defer ttyFile.Close()
		programOpts = append(programOpts, tea.WithInput(ttyFile))
	}

	// Build lipgloss renderer from stdout so it detects colour support correctly.
	r := lipgloss.NewRenderer(os.Stdout)

	system := systemprompt.Agent()
	m := newModel(providerName, provider, modelID, system, stdin, r)

	p := tea.NewProgram(m, programOpts...)
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("TUI error: %w", err)
	}
	return nil
}
