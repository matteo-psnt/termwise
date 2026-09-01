package agentui

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	agenttools "github.com/matteo-psnt/termwise/internal/agent/tools"
	"github.com/matteo-psnt/termwise/internal/config"
	"github.com/matteo-psnt/termwise/internal/history"
	"github.com/matteo-psnt/termwise/internal/provider"
	"github.com/matteo-psnt/termwise/internal/systemprompt"
	"github.com/matteo-psnt/termwise/internal/theme"
	"github.com/matteo-psnt/termwise/internal/tty"
)

type openProgramConfig struct {
	providerName  string
	provider      provider.AgentClient
	modelID       string
	cfgPath       string
	stdin         string
	stdinPiped    bool
	initialDraft  string
	initialPrompt string
	sessionID     string
	forceResume   bool
	programInput  *os.File
	programOutput *os.File
}

// Open launches the agent TUI without an initial prompt.
func Open(
	providerName string,
	provider provider.AgentClient,
	modelID string,
	cfgPath string,
	sessionID string,
	forceResume bool,
) error {
	stdin, stdinPiped, err := readProgramStdin()
	if err != nil {
		return err
	}
	_, err = openProgram(openProgramConfig{
		providerName: providerName,
		provider:     provider,
		modelID:      modelID,
		cfgPath:      cfgPath,
		stdin:        stdin,
		stdinPiped:   stdinPiped,
		sessionID:    sessionID,
		forceResume:  forceResume,
	})
	return err
}

// OpenWithPrompt launches the agent TUI and submits an initial prompt.
func OpenWithPrompt(
	providerName string,
	provider provider.AgentClient,
	modelID string,
	cfgPath string,
	initialPrompt string,
	sessionID string,
	forceResume bool,
) error {
	stdin, stdinPiped, err := readProgramStdin()
	if err != nil {
		return err
	}
	_, err = openProgram(openProgramConfig{
		providerName:  providerName,
		provider:      provider,
		modelID:       modelID,
		cfgPath:       cfgPath,
		stdin:         stdin,
		stdinPiped:    stdinPiped,
		initialPrompt: initialPrompt,
		sessionID:     sessionID,
		forceResume:   forceResume,
	})
	return err
}

// OpenShellWidget launches the widget TUI and returns the final shell handoff command.
func OpenShellWidget(
	providerName string,
	provider provider.AgentClient,
	modelID string,
	cfgPath string,
	sessionID string,
	forceResume bool,
) (string, error) {
	initialDraft, _, err := readProgramStdin()
	if err != nil {
		return "", err
	}
	ttyRW, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return "", fmt.Errorf("opening /dev/tty: %w", err)
	}
	defer func() { _ = ttyRW.Close() }()

	return openProgram(openProgramConfig{
		providerName:  providerName,
		provider:      provider,
		modelID:       modelID,
		cfgPath:       cfgPath,
		initialDraft:  initialDraft,
		sessionID:     sessionID,
		forceResume:   forceResume,
		programInput:  ttyRW,
		programOutput: ttyRW,
	})
}

func readProgramStdin() (string, bool, error) {
	stdinPiped := !tty.IsTerminal(os.Stdin)
	if stdinPiped {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return "", false, fmt.Errorf("reading stdin: %w", err)
		}
		return strings.TrimRight(string(data), "\n"), true, nil
	}
	return "", false, nil
}

func openProgram(cfg openProgramConfig) (string, error) {

	// Determine program input. When stdin is piped we need to reopen the TTY
	// so bubbletea can receive keyboard events. Mouse capture (for viewport
	// wheel scrolling and selection) is requested per-frame via View.MouseMode
	// in v2, not as a program option.
	var programOpts []tea.ProgramOption
	outputFile := os.Stdout
	// Queried before NewProgram so the DSR reply doesn't end up in bubbletea's input queue.
	inputFile := os.Stdin

	if cfg.programInput != nil {
		inputFile = cfg.programInput
		programOpts = append(programOpts, tea.WithInput(cfg.programInput))
	}
	if cfg.programOutput != nil {
		outputFile = cfg.programOutput
		programOpts = append(programOpts, tea.WithOutput(cfg.programOutput))
	} else if cfg.stdinPiped {
		ttyFile, err := os.Open("/dev/tty")
		if err != nil {
			return "", fmt.Errorf("opening /dev/tty for keyboard input: %w", err)
		}
		defer func() { _ = ttyFile.Close() }()
		inputFile = ttyFile
		programOpts = append(programOpts, tea.WithInput(ttyFile))
	}

	// -1 = unknown; the model falls back to bottom-anchoring.
	initialCursorRow := -1
	if row, err := tty.QueryCursorRow(inputFile, outputFile); err == nil {
		initialCursorRow = row
	}

	// Detect the terminal background once, before the program starts, so the
	// light/dark palette and glamour theme are chosen correctly. Querying here
	// (like the cursor-row query above) keeps the OSC 11 reply out of
	// bubbletea's input queue.
	hasDarkBg := lipgloss.HasDarkBackground(inputFile, outputFile)

	themeName := theme.DefaultName
	var llmJudge bool
	var closeKey string
	var autoResume bool
	suggestions := true
	var effort string
	var configDir string
	if cfg.cfgPath != "" {
		cfgFile, exists, err := config.LoadConfig(cfg.cfgPath)
		if err == nil && exists {
			themeName = theme.Normalize(cfgFile.Settings.Theme)
			llmJudge = config.ResolveBoolSetting(cfgFile, "llm_judge")
			closeKey = cfgFile.Settings.Keybinding
			autoResume = config.ResolveBoolSetting(cfgFile, "auto_resume")
			suggestions = config.ResolveBoolSetting(cfgFile, "suggestions")
			effort = cfgFile.Providers[cfg.providerName].Effort
		}
		configDir = filepath.Dir(cfg.cfgPath)
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
	if cfg.sessionID != "" && sessionStore != nil && (autoResume || cfg.forceResume) {
		if s, ok, err := sessionStore.Load(cfg.sessionID); err == nil && ok {
			initialSession = &s
		}
	}

	system := systemprompt.Agent(agenttools.Defs)
	m := newModel(cfg.providerName, cfg.provider, cfg.modelID, system, cfg.stdin, hasDarkBg, llmJudge, suggestions, effort, cfg.cfgPath, cfg.initialDraft, cfg.initialPrompt, themeName, closeKey,
		promptHistory, sessionStore, cfg.sessionID, initialSession, initialCursorRow)

	// Surface any config warnings as a SystemEntry so they remain visible (and
	// terminal-selectable) inside the TUI — stderr text is wiped by the alt-screen.
	if cfg.cfgPath != "" {
		if warnings := config.CheckConfigWarnings(cfg.cfgPath); len(warnings) > 0 {
			m.thread = append(m.thread, SystemEntry{
				Content: "Config warnings:\n" + strings.Join(warnings, "\n"),
			})
		}
	}

	p := tea.NewProgram(m, programOpts...)
	finalModel, err := p.Run()
	if err != nil {
		return "", fmt.Errorf("TUI error: %w", err)
	}
	if model, ok := finalModel.(Model); ok {
		return model.exitShellCommand(), nil
	}
	return "", nil
}
