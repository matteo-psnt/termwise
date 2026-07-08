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
	// so bubbletea can receive keyboard events.
	var programOpts []tea.ProgramOption
	outputFile := os.Stdout

	if cfg.programInput != nil {
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
		programOpts = append(programOpts, tea.WithInput(ttyFile))
	}

	// bubbletea's init() already pre-queried lipgloss.HasDarkBackground() on
	// the global renderer before the program started. Read that cached value
	// and pin it on our custom renderer so AdaptiveColor never sends a second
	// OSC 11 query (which would leave unread bytes in the tty buffer).
	r := lipgloss.NewRenderer(outputFile)
	r.SetHasDarkBackground(lipgloss.HasDarkBackground())

	themeName := theme.DefaultName
	var llmJudge bool
	var closeKey string
	var autoResume bool
	suggestions := true
	var configDir string
	if cfg.cfgPath != "" {
		cfgFile, exists, err := config.LoadConfig(cfg.cfgPath)
		if err == nil && exists {
			themeName = theme.Normalize(cfgFile.Settings.Theme)
			llmJudge = config.ResolveBoolSetting(cfgFile, "llm_judge")
			closeKey = cfgFile.Settings.Keybinding
			autoResume = config.ResolveBoolSetting(cfgFile, "auto_resume")
			suggestions = config.ResolveBoolSetting(cfgFile, "suggestions")
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
	m := newModel(cfg.providerName, cfg.provider, cfg.modelID, system, cfg.stdin, r, llmJudge, suggestions, cfg.initialDraft, cfg.initialPrompt, themeName, closeKey,
		promptHistory, sessionStore, cfg.sessionID, initialSession)

	programOpts = append(programOpts, tea.WithMouseCellMotion())
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
