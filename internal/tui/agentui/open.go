package agentui

import (
	"fmt"
	"io"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/matteo-psnt/termwise/internal/config"
	"github.com/matteo-psnt/termwise/internal/provider"
	"github.com/matteo-psnt/termwise/internal/systemprompt"
	"github.com/matteo-psnt/termwise/internal/theme"
	"github.com/matteo-psnt/termwise/internal/tty"
)

// Open launches the agent TUI. It handles stdin pre-loading and TTY setup.
func Open(providerName string, provider provider.AgentClient, modelID string, cfgPath string, prefill string) error {
	// Read stdin if piped.
	stdinPiped := !tty.IsTerminal(os.Stdin)
	var stdin string
	if stdinPiped {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return fmt.Errorf("reading stdin: %w", err)
		}
		stdin = strings.TrimRight(string(data), "\n")
	}

	// Determine program input. When stdin is piped we need to reopen the TTY
	// so bubbletea can receive keyboard events.
	var programOpts []tea.ProgramOption

	if stdinPiped {
		ttyFile, err := os.Open("/dev/tty")
		if err != nil {
			return fmt.Errorf("opening /dev/tty for keyboard input: %w", err)
		}
		defer ttyFile.Close()
		programOpts = append(programOpts, tea.WithInput(ttyFile))
	}

	// bubbletea's init() already pre-queried lipgloss.HasDarkBackground() on
	// the global renderer before the program started. Read that cached value
	// and pin it on our custom renderer so AdaptiveColor never sends a second
	// OSC 11 query (which would leave unread bytes in the tty buffer).
	r := lipgloss.NewRenderer(os.Stdout)
	r.SetHasDarkBackground(lipgloss.HasDarkBackground())

	themeName := theme.DefaultName
	var llmJudge bool
	var closeKey string
	if cfgPath != "" {
		cfg, exists, err := config.LoadConfig(cfgPath)
		if err == nil && exists {
			themeName = theme.Normalize(cfg.UI.Theme)
			llmJudge = cfg.Policies.Bash.LLMJudge
			closeKey = cfg.Shell.Keybinding
		}
	}

	system := systemprompt.Agent()
	m := newModel(providerName, provider, modelID, system, stdin, r, llmJudge, prefill, themeName, closeKey)

	p := tea.NewProgram(m, programOpts...)
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("TUI error: %w", err)
	}
	return nil
}
