package agentui

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	agenttools "github.com/matteo-psnt/termwise/internal/agent/tools"
	"github.com/matteo-psnt/termwise/internal/config"
	"github.com/matteo-psnt/termwise/internal/history"
	"github.com/matteo-psnt/termwise/internal/provider"
	"github.com/matteo-psnt/termwise/internal/systemprompt"
	"github.com/matteo-psnt/termwise/internal/theme"
	"github.com/matteo-psnt/termwise/internal/tty"
)

// Open launches the agent TUI. It handles stdin pre-loading and TTY setup.
func Open(
	providerName string,
	provider provider.AgentClient,
	modelID string,
	cfgPath string,
	initialDraft string,
	initialPrompt string,
	sessionID string,
	forceResume bool,
) error {
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
		defer func() { _ = ttyFile.Close() }()
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
	var autoResume bool
	var configDir string
	if cfgPath != "" {
		cfg, exists, err := config.LoadConfig(cfgPath)
		if err == nil && exists {
			themeName = theme.Normalize(cfg.Settings.Theme)
			llmJudge = cfg.Settings.LLMJudge
			closeKey = cfg.Settings.Keybinding
			autoResume = cfg.Settings.AutoResume
		}
		configDir = filepath.Dir(cfgPath)
	}

	// Set up persistent history and session stores.
	var promptHistory *history.PromptHistory
	var sessionStore *history.SessionStore
	if configDir != "" {
		promptHistory = history.NewPromptHistory(configDir)
		promptHistory.Load() //nolint:errcheck // Empty or unreadable history should not block the TUI.

		sessionStore = history.NewSessionStore(configDir)
		sessionStore.PruneOld()
	}

	// Load prior session if requested.
	var initialSession *history.Session
	if sessionID != "" && sessionStore != nil && (autoResume || forceResume) {
		if s, ok, err := sessionStore.Load(sessionID); err == nil && ok {
			initialSession = &s
		}
	}

	system := systemprompt.Agent(agenttools.Defs)
	m := newModel(providerName, provider, modelID, system, stdin, r, llmJudge, initialDraft, initialPrompt, themeName, closeKey,
		promptHistory, sessionStore, sessionID, initialSession)

	programOpts = append(programOpts, tea.WithMouseCellMotion())
	p := tea.NewProgram(m, programOpts...)
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("TUI error: %w", err)
	}
	return nil
}
